package http

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"
	urllib "net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
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
	// llmRespCache cache llm response to quick response
	llmRespCache = gutils.NewExpCache[string](context.Background(), time.Hour)
)

// ChatHandler handle api request
func ChatHandler(ctx *gin.Context) {
	toolcalls := sendAndParseChat(ctx)
	if toolcalls == nil {
		return
	}

	AbortErr(ctx, errors.New("tool calls not implemented"))
}

func sendAndParseChat(ctx *gin.Context) (toolCalls []OpenaiCompletionStreamRespToolCall) {
	logger := gmw.GetLogger(ctx)
	frontReq, openaiReq, err := convert2OpenaiRequest(ctx) //nolint:bodyclose
	if AbortErr(ctx, err) {
		return
	}

	// read cache
	if cacheKey, err := req2CacheKey(frontReq); err != nil {
		logger.Warn("marshal req for cache key", zap.Error(err))
	} else if respContent, ok := llmRespCache.Load(cacheKey); ok {
		res := &OpenaiCompletionStreamResp{
			Choices: []OpenaiCompletionStreamRespChoice{
				{
					Delta: OpenaiCompletionStreamRespDelta{
						Content: respContent,
					},
					FinishReason: "stop",
				},
			},
		}
		if data, err := json.Marshal(res); err != nil {
			logger.Warn("marshal resp", zap.Error(err))
		} else {
			data = append([]byte("data: "), data...)
			data = append(data, []byte("\n\n")...)

			if _, err = io.Copy(ctx.Writer, bytes.NewReader(data)); err != nil {
				logger.Warn("resp from cache", zap.Error(err))
			} else {
				logger.Debug("hit cache for llm response")
				return
			}
		}
	}

	// send request to openai
	logger.Debug("try send request to upstream server", zap.String("url", openaiReq.RemoteAddr))
	resp, err := httpcli.Do(openaiReq) //nolint: bodyclose
	if AbortErr(ctx, err) {
		return
	}
	defer gutils.LogErr(resp.Body.Close, logger)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		AbortErr(ctx, errors.Errorf("[%d]%s", resp.StatusCode, body))
		return
	}

	CopyHeader(ctx.Writer.Header(), resp.Header)
	isStream := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
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
	var lastResp *OpenaiCompletionStreamResp
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

		lastResp = new(OpenaiCompletionStreamResp)
		if err = json.Unmarshal(chunk, lastResp); err != nil {
			logger.Warn("unmarshal resp", zap.ByteString("chunk", chunk), zap.Error(err))
			continue
		}

		if len(lastResp.Choices) > 0 {
			if len(lastResp.Choices[0].Delta.ToolCalls) != 0 {
				return lastResp.Choices[0].Delta.ToolCalls
			}

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
	} else if lastResp != nil && gutils.IsEmpty(lastResp) {
		return // bypass empty response
	} else {
		AbortErr(ctx, errors.Errorf("unsupport resp body %q", reader.Text()))
	}

	return nil
}

func req2CacheKey(req *FrontendReq) (string, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return "", errors.Wrap(err, "marshal req")
	}

	hashed := sha1.Sum(data)
	return hex.EncodeToString(hashed[:]), nil
}

func saveLLMConservation(req *FrontendReq, respContent string) {
	logger := log.Logger.Named("save_llm")

	// save to cache
	if cacheKey, err := req2CacheKey(req); err != nil {
		logger.Warn("marshal req for cache key", zap.Error(err))
	} else {
		llmRespCache.Store(cacheKey, respContent)
	}

	// save to db
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

// VisionTokenPrice vision token price($/500000)
const VisionTokenPrice = 5000

// CountVisionImagePrice count vision image tokens
//
// https://openai.com/pricing
func CountVisionImagePrice(width int, height int, resolution VisionImageResolution) (int, error) {
	switch resolution {
	case VisionImageResolutionLow:
		return 85, nil // fixed price
	case VisionImageResolutionHigh:
		h := math.Ceil(float64(height) / 512)
		w := math.Ceil(float64(width) / 512)
		n := w * h
		total := 85 + 170*n
		return int(total) * VisionTokenPrice, nil
	default:
		return 0, errors.Errorf("unsupport resolution %q", resolution)
	}
}

func imageSize(cnt []byte) (width, height int, err error) {
	contentType := http.DetectContentType(cnt)
	switch contentType {
	case "image/jpeg", "image/jpg":
		img, err := jpeg.Decode(bytes.NewReader(cnt))
		if err != nil {
			return 0, 0, errors.Wrap(err, "decode jpeg")
		}

		bounds := img.Bounds()
		return bounds.Dx(), bounds.Dy(), nil
	case "image/png":
		img, err := png.Decode(bytes.NewReader(cnt))
		if err != nil {
			return 0, 0, errors.Wrap(err, "decode png")
		}

		bounds := img.Bounds()
		return bounds.Dx(), bounds.Dy(), nil
	default:
		return 0, 0, errors.Errorf("unsupport image content type %q", contentType)
	}
}

var (
	// hdResolutionMarker enable hd resolution for gpt-4-vision only
	// if user has permission and mention "hd" in prompt
	hdResolutionMarker = regexp.MustCompile(`\bhd\b`)
)

func convert2OpenaiRequest(ctx *gin.Context) (frontendReq *FrontendReq, openaiReq *http.Request, err error) {
	// logger := gmw.GetLogger(ctx).With(zap.String("method", ctx.Request.Method))
	path := strings.TrimPrefix(ctx.Request.URL.Path, "/chat")
	user, err := getUserByAuthHeader(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "get user")
	}

	// no need to check quota for chat, because the chat api (one-api) will check it
	// if err := checkUserExternalBilling(ctx.Request.Context(), user, 0); err != nil {
	// 	return nil, nil, errors.Wrapf(err, "check quota for user %q", user.UserName)
	// }

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
		case "gpt-4-1106-preview",
			"gpt-4-0613",
			"gpt-4-32k",
			"gpt-4-32k-0613",
			"gpt-3.5-turbo",
			"gpt-3.5-turbo-16k",
			"gpt-3.5-turbo-0613",
			"gpt-3.5-turbo-16k-0613",
			"gemini-pro":
			newUrl = fmt.Sprintf("%s/%s", user.APIBase, "v1/chat/completions")

			req := new(OpenaiChatReq[string])
			if err := copier.Copy(req, frontendReq); err != nil {
				return nil, nil, errors.Wrap(err, "copy to chat req")
			}

			openaiReq = req
		case "gpt-4-vision-preview":
			newUrl = fmt.Sprintf("%s/%s", user.APIBase, "v1/chat/completions")
			lastMessage := frontendReq.Messages[len(frontendReq.Messages)-1]
			if len(lastMessage.Files) == 0 { // gpt-vision
				return nil, nil, errors.New("no image")
			}

			req := new(OpenaiChatReq[[]OpenaiVisionMessageContent])
			if err := copier.Copy(req, frontendReq); err != nil {
				return nil, nil, errors.Wrap(err, "copy to chat req")
			}

			req.Messages = []OpenaiReqMessage[[]OpenaiVisionMessageContent]{
				{
					Role: OpenaiMessageRoleUser,
					Content: []OpenaiVisionMessageContent{
						{
							Type: OpenaiVisionMessageContentTypeText,
							Text: lastMessage.Content,
						},
					},
				},
			}

			totalFileSize := 0
			for i, f := range lastMessage.Files {
				resolution := VisionImageResolutionLow
				// if user has permission and image size is large than 1MB,
				// use high resolution
				if (user.BYOK || user.NoLimitExpensiveModels) && hdResolutionMarker.MatchString(lastMessage.Content) {
					resolution = VisionImageResolutionHigh
				}

				req.Messages[0].Content = append(req.Messages[0].Content, OpenaiVisionMessageContent{
					Type: OpenaiVisionMessageContentTypeImageUrl,
					ImageUrl: OpenaiVisionMessageContentImageUrl{
						URL:    "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(f.Content),
						Detail: resolution,
					},
				})

				if i >= 1 {
					break // only support 2 images for cost saving
				}

				totalFileSize += len(f.Content)
				if totalFileSize > 10*1024*1024 {
					return nil, nil, errors.Errorf("total file size should less than 10MB, got %d", totalFileSize)
				}
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
		body = io.NopCloser(bytes.NewReader(payload))
	}

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

	return frontendReq, req, nil
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

	ctx, cancel := context.WithTimeout(gctx.Request.Context(), 25*time.Second)
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

	if args.user.IsFree {
		reqData["max_chunks"] = 1500
	}

	postBody, err := json.Marshal(reqData)
	if err != nil {
		return "", errors.Wrap(err, "marshal post body")
	}

	queryChunkURL := fmt.Sprintf("%s/gptchat/query/chunks", config.Config.RamjetURL)

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

	if len(userReq.Messages) == 0 {
		return nil, errors.New("no messages")
	}

	trimMessages(userReq)
	maxTokens := uint(MaxTokens())
	if maxTokens != 0 && userReq.MaxTokens > maxTokens {
		return nil, errors.Errorf("max_tokens should less than %d", maxTokens)
	}

	stopch := make(chan struct{})
	defer close(stopch)
	go func() {
		for {
			select {
			case <-stopch:
				return
			case <-gctx.Request.Context().Done():
				return
			default:
			}

			if _, err := io.Copy(gctx.Writer, bytes.NewReader([]byte("data: [HEARTBEAT]\n\n"))); err != nil {
				log.Logger.Warn("failed write heartbeat msg to sse", zap.Error(err))
				return
			}

			gctx.Writer.Flush()
			time.Sleep(time.Second)
		}
	}()

	if config.Config.RamjetURL != "" {
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
