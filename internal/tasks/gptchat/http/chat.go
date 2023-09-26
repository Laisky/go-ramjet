package http

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	urllib "net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/go-utils/v4/json"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/copier"
	"golang.org/x/net/html"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/db"
	"github.com/Laisky/go-ramjet/library/log"
)

var (
	dataReg = regexp.MustCompile(`data: (\{.*\})`)
)

var ramjetURL = strings.TrimSuffix(os.Getenv("GPTCHAT_CHUNK_SEARCH_URL"), "/")

// APIHandler handle api request
func APIHandler(ctx *gin.Context) {
	defer ctx.Request.Body.Close() // nolint: errcheck,gosec
	// logger := log.Logger.Named("chat")

	frontReq, resp, err := send2openai(ctx) //nolint:bodyclose
	if AbortErr(ctx, err) {
		return
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger)

	CopyHeader(ctx.Writer.Header(), resp.Header)
	isStream := resp.Header.Get("Content-Type") == "text/event-stream"
	bodyReader := resp.Body

	reader := bufio.NewScanner(bodyReader)
	reader.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF {
			return 0, nil, io.EOF
		}

		if i := bytes.Index(data, []byte("\n\n")); i >= 0 {
			return i + 1, data[0:i], nil
		}

		return 0, nil, nil
	})

	var respContent string
	var lastResp *OpenaiCOmpletionStreamResp
	for reader.Scan() {
		line := reader.Bytes()
		// logger.Debug("got response line", zap.ByteString("line", line))

		var chunk []byte
		if matched := dataReg.FindAllSubmatch(line, -1); len(matched) != 0 {
			chunk = matched[0][1]
		}

		_, err = io.Copy(ctx.Writer, bytes.NewReader(append(line, []byte("\n\n")...)))
		if AbortErr(ctx, err) {
			return
		}

		lastResp = new(OpenaiCOmpletionStreamResp)
		if err = json.Unmarshal(chunk, lastResp); err != nil {
			//nolint: lll
			// TODO completion's stream response is not support
			//
			// 2023-03-16T08:02:37Z	DEBUG	go-ramjet.chat	http/chat.go:68	got response line	{"line": "\ndata: {\"id\": \"cmpl-6ucrBZjC3aU8Nu4izkaSywzdVb8h1\", \"object\": \"text_completion\", \"created\": 1678953753, \"choices\": [{\"text\": \"\\n\", \"index\": 0, \"logprobs\": null, \"finish_reason\": null}], \"model\": \"text-davinci-003\"}"}
			// 2023-03-16T08:02:37Z	DEBUG	go-ramjet.chat	http/chat.go:68	got response line	{"line": "\ndata: {\"id\": \"cmpl-6ucrBZjC3aU8Nu4izkaSywzdVb8h1\", \"object\": \"text_completion\", \"created\": 1678953753, \"choices\": [{\"text\": \"});\", \"index\": 0, \"logprobs\": null, \"finish_reason\": null}], \"model\": \"text-davinci-003\"}"}

			continue
		}

		if len(lastResp.Choices) > 0 {
			respContent += lastResp.Choices[0].Delta.Content
		}

		// check if resp is end
		if !isStream ||
			len(lastResp.Choices) == 0 ||
			lastResp.Choices[0].FinishReason != "" {
			go saveLLMConservation(frontReq, respContent)
			return
		}
	}

	go saveLLMConservation(frontReq, respContent)
	// scanner quit unexpected, write last line
	if lastResp != nil &&
		len(lastResp.Choices) != 0 &&
		lastResp.Choices[0].FinishReason == "" {
		lastResp.Choices[0].FinishReason = "stop"
		lastResp.Choices[0].Delta.Content = " [TRUNCATED BY SERVER]"
		payload, err := json.MarshalToString(lastResp)
		if AbortErr(ctx, err) {
			return
		}

		_, err = io.Copy(ctx.Writer, strings.NewReader("\ndata: "+payload))
		if AbortErr(ctx, err) {
			return
		}
	} else {
		AbortErr(ctx, errors.Errorf("unsupport resp body %q", reader.Text()))
	}
}

func saveLLMConservation(req *FrontendReq, respContent string) {
	logger := log.Logger.Named("save_llm")
	openaidb, err := db.GetOpenaiDB()
	if err != nil {
		logger.Error("get openai db", zap.Error(err))
		return
	}

	docu := &db.OpenaiConservation{
		Model:      req.Model,
		MaxTokens:  req.MaxTokens,
		Completion: respContent,
	}
	for _, msg := range req.Messages {
		docu.Prompt = append(docu.Prompt, db.OpenaiMessage{
			Role:    msg.Role.String(),
			Content: msg.Content,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	ret, err := openaidb.GetCol("conservations").InsertOne(ctx, docu)
	if err != nil {
		logger.Error("insert conservation", zap.Error(err))
		return
	}

	logger.Debug("save conservation", zap.Any("id", ret.InsertedID))
}

func send2openai(ctx *gin.Context) (frontendReq *FrontendReq, resp *http.Response, err error) {
	path := strings.TrimPrefix(ctx.Request.URL.Path, "/chat")
	user, err := getUserByAuthHeader(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "get user")
	}

	if err := checkUserTotalQuota(ctx.Request.Context(), user, 0); err != nil {
		return nil, nil, errors.Wrapf(err, "check quota for user %q", user.UserName)
	}

	newUrl := fmt.Sprintf("%s%s",
		user.APIBase,
		path,
	)

	if ctx.Request.URL.RawQuery != "" {
		newUrl += "?" + ctx.Request.URL.RawQuery
	}

	body := ctx.Request.Body
	if gutils.Contains([]string{http.MethodPost, http.MethodPut}, ctx.Request.Method) {
		frontendReq, err = bodyChecker(ctx, user, ctx.Request.Body)
		if err != nil {
			return nil, nil, errors.Wrap(err, "request is illegal")
		}

		if err := user.IsModelAllowed(frontendReq.Model); err != nil {
			return nil, nil, errors.Wrapf(err, "check is model allowed for user %q", user.UserName)
		}

		var openaiReq any
		switch frontendReq.Model {
		case "gpt-3.5-turbo",
			"gpt-3.5-turbo-16k",
			"gpt-3.5-turbo-0613",
			"gpt-3.5-turbo-16k-0613",
			"gpt-4",
			"gpt-4-0613",
			"gpt-4-32k",
			"gpt-4-32k-0613":
			newUrl = fmt.Sprintf("%s/%s", user.APIBase, "v1/chat/completions")
			req := new(OpenaiChatReq)
			if err := copier.Copy(req, frontendReq); err != nil {
				return nil, nil, errors.Wrap(err, "copy to chat req")
			}

			openaiReq = req
		case "text-davinci-003":
			newUrl = fmt.Sprintf("%s/%s", user.APIBase, "v1/completions")
			openaiReq = new(OpenaiCompletionReq)
			if err := copier.Copy(openaiReq, frontendReq); err != nil {
				return nil, nil, errors.Wrap(err, "copy to completion req")
			}
		default:
			return nil, nil, errors.Errorf("unsupport chat model %q", frontendReq.Model)
		}

		payload, err := json.Marshal(openaiReq)
		if err != nil {
			return nil, nil, errors.Wrap(err, "marshal new body")
		}
		// log.Logger.Debug("prepare request", zap.ByteString("req", payload))
		body = io.NopCloser(bytes.NewReader(payload))
	}

	log.Logger.Debug("send request to openai api",
		zap.String("url", newUrl), zap.String("method", ctx.Request.Method))
	req, err := http.NewRequestWithContext(ctx.Request.Context(),
		ctx.Request.Method, newUrl, body)
	if err != nil {
		return nil, nil, errors.Wrap(err, "new request")
	}
	CopyHeader(req.Header, ctx.Request.Header)
	req.Header.Set("authorization", "Bearer "+user.OpenaiToken)

	// if set header "Accept-Encoding" manually,
	// golang's http client will not auto decompress response body
	req.Header.Del("Accept-Encoding")

	log.Logger.Debug("proxy request", zap.String("url", newUrl))
	resp, err = httpcli.Do(req)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "do request %q", newUrl)
	}

	if resp.StatusCode != http.StatusOK {
		defer gutils.LogErr(resp.Body.Close, log.Logger) // nolint
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, errors.Errorf("[%d]%s", resp.StatusCode, string(body))
	}

	// do not close resp.Body
	return frontendReq, resp, nil
}

func (r *FrontendReq) fillDefault() {
	r.MaxTokens = gutils.OptionalVal(&r.MaxTokens, uint(MaxTokens()))
	r.Temperature = gutils.OptionalVal(&r.Temperature, 1)
	r.TopP = gutils.OptionalVal(&r.TopP, 1)
	r.N = gutils.OptionalVal(&r.N, 1)
	r.Model = gutils.OptionalVal(&r.Model, ChatModel())
	// r.BestOf = gutils.OptionalVal(&r.BestOf, 1)
}

// UserQueryType user query type
type UserQueryType string

const (
	// UserQueryTypeSearch search by embeddings chunks
	UserQueryTypeSearch UserQueryType = "search"
	// UserQueryTypeScan scan by map-reduce
	UserQueryTypeScan UserQueryType = "scan"
)

var (
	urlContentCache = gutils.NewExpCache[[]byte](context.Background(), 24*time.Hour)
	urlRegexp       = regexp.MustCompile(`https?://[^\s]+`)
)

// fetchURLContent fetch url content
func fetchURLContent(gctx *gin.Context, url string) (content []byte, err error) {
	content, ok := urlContentCache.Load(url)
	if ok {
		log.Logger.Debug("hit cache for query mentioned url", zap.String("url", url))
		return content, nil
	}

	ctx, cancel := context.WithTimeout(gctx.Request.Context(), 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "new request %q", url)
	}
	req.Header.Set("User-Agent", "go-ramjet-bot")
	resp, err := httpcli.Do(req) // nolint:bodyclose
	if err != nil {
		return nil, errors.Wrapf(err, "do request %q", url)
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger)

	contentType := resp.Header.Get("Content-Type")
	switch {
	case strings.Contains(contentType, "text/html") ||
		strings.Contains(contentType, "application/xhtml+xml"):
		content, err = fetchDynamicURLContent(ctx, url)
	default:
		content, err = fetchStaticURLContent(ctx, url)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "fetch url %q", url)
	}

	urlContentCache.Store(url, content) // save cache
	return content, nil
}

// findHTMLBody find html body recursively
func findHTMLBody(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.Data == "body" {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if body := findHTMLBody(c); body != nil {
			return body
		}
	}
	return nil
}

// extractHTMLBody extract body from html
func extractHTMLBody(content []byte) (bodyContent []byte, err error) {
	parsedHTML, err := html.Parse(bytes.NewReader(content))
	if err != nil {
		return nil, errors.Wrap(err, "parse html")
	}

	body := findHTMLBody(parsedHTML)
	if body == nil {
		return nil, errors.New("no body found")
	}

	var out bytes.Buffer
	if err := html.Render(&out, body); err != nil {
		return nil, errors.Wrap(err, "render html")
	}

	return out.Bytes(), nil
}

// embeddingUrlContent if user has mentioned some url in message,
// try to fetch and embed content of url into the tail of message.
func (r *FrontendReq) embeddingUrlContent(gctx *gin.Context, user *config.UserConfig) {
	if len(r.Messages) == 0 {
		return
	}

	var lastUserPrompt *string
	for i := len(r.Messages) - 1; i >= 0; i-- {
		if r.Messages[i].Role != OpenaiMessageRoleUser {
			continue
		}

		lastUserPrompt = &r.Messages[i].Content
	}

	if lastUserPrompt == nil { // no user prompt
		return
	}

	urls := urlRegexp.FindAllString(*lastUserPrompt, -1)
	if len(urls) == 0 { // user do not mention any url
		return
	}

	var (
		pool        errgroup.Group
		mu          sync.Mutex
		auxiliaries []string
	)
	for _, url := range urls {
		url := url
		pool.Go(func() (err error) {
			content, err := fetchURLContent(gctx, url)
			if err != nil {
				return errors.Wrap(err, "fetch url content")
			}

			parsedURL, err := urllib.Parse(url)
			if err != nil {
				return errors.Wrap(err, "parse url")
			}

			ext := strings.ToLower(filepath.Ext(parsedURL.Path))
			if !gutils.Contains([]string{".txt", ".md", ".doc", ".docx", ".ppt", ".pptx", ".pdf"}, ext) {
				ext = ".html" // default
			}

			auxiliary, err := queryChunks(gctx, queryChunksArgs{
				user:    user,
				query:   *lastUserPrompt,
				ext:     ext,
				model:   r.Model,
				content: content,
			})
			if err != nil {
				return errors.Wrap(err, "query chunks")
			}

			mu.Lock()
			auxiliaries = append(auxiliaries, auxiliary)
			mu.Unlock()

			return nil
		})
	}

	if err := pool.Wait(); err != nil {
		log.Logger.Error("query mentioned urls", zap.Error(err))
		*lastUserPrompt += "\n\n(some url content is not available)"
	}

	if len(auxiliaries) == 0 {
		return
	}

	*lastUserPrompt += "\n\nfollowing are auxiliary content just for your reference:\n\n" +
		strings.Join(auxiliaries, "\n")
}

type queryChunksResponse struct {
	Results  string `json:"results"`
	Cached   bool   `json:"cached"`
	CacheKey string `json:"cache_key"`
	Operator string `json:"operator"`
}

type queryChunksArgs struct {
	user    *config.UserConfig
	query   string
	ext     string
	model   string
	content []byte
}

func queryChunks(gctx *gin.Context, args queryChunksArgs) (result string, err error) {
	log.Logger.Debug("query ramjet to search chunks",
		zap.String("ext", args.ext))

	reqData := map[string]any{
		"content":    args.content,
		"query":      args.query,
		"ext":        args.ext,
		"model":      args.model,
		"max_chunks": 200,
	}

	if args.user.IsPaid {
		reqData["max_chunks"] = 1500
	}

	postBody, err := json.Marshal(reqData)
	if err != nil {
		return "", errors.Wrap(err, "marshal post body")
	}

	queryChunkURL := fmt.Sprintf("%s/gptchat/query/chunks", ramjetURL)

	queryCtx, queryCancel := context.WithTimeout(gctx.Request.Context(), 180*time.Second)
	defer queryCancel()
	req, err := http.NewRequestWithContext(queryCtx, http.MethodPost, queryChunkURL, bytes.NewReader(postBody))
	if err != nil {
		return "", errors.Wrapf(err, "new request %q", queryChunkURL)
	}
	req.Header.Set("Authorization", "Bearer "+args.user.OpenaiToken)

	if err := setUserAuth(gctx, req); err != nil {
		return "", errors.Wrap(err, "set user auth")
	}

	resp, err := httpcli.Do(req) // nolint:bodyclose
	if err != nil {
		return "", errors.Wrapf(err, "do request %q", queryChunkURL)
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger)

	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("[%d]%s", resp.StatusCode, queryChunkURL)
	}

	args.content, err = io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "read response body")
	}

	respData := new(queryChunksResponse)
	if err = json.Unmarshal(args.content, respData); err != nil {
		return "", errors.Wrap(err, "unmarshal response body")
	}

	log.Logger.Debug("got ramjet parsed chunks",
		// zap.String("result", respData.Results),
		zap.Bool("cached", respData.Cached),
		zap.String("cache_key", respData.CacheKey),
		zap.String("operator", respData.Operator),
	)
	return respData.Results, nil
}

func bodyChecker(gctx *gin.Context, user *config.UserConfig, body io.ReadCloser) (userReq *FrontendReq, err error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return nil, errors.Wrap(err, "read request body")
	}

	userReq = new(FrontendReq)
	if err = json.Unmarshal(payload, userReq); err != nil {
		return nil, errors.Wrap(err, "parse request")
	}
	userReq.fillDefault()

	trimMessages(userReq)
	maxTokens := uint(MaxTokens())
	if maxTokens != 0 && userReq.MaxTokens > maxTokens {
		return nil, errors.Errorf("max_tokens should less than %d", maxTokens)
	}

	if ramjetURL != "" {
		userReq.embeddingUrlContent(gctx, user)
	}

	return userReq, err
}

func trimMessages(data *FrontendReq) {
	maxMessages := MaxMessages()
	maxTokens := MaxTokens()

	if maxMessages != 0 && len(data.Messages) > maxMessages {
		data.Messages = data.Messages[len(data.Messages)-maxMessages:]
	}

	if maxTokens != 0 {
		for i := range data.Messages {
			cnt := data.Messages[i].Content
			if len(cnt) > maxTokens {
				cnt = cnt[len(cnt)-maxTokens:]
				data.Messages[i].Content = cnt
			}
		}
	}
}
