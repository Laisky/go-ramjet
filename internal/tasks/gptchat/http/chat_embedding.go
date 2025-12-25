package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	urllib "net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	gutils "github.com/Laisky/go-utils/v5"
	"github.com/Laisky/go-utils/v5/json"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/html"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	gptTasks "github.com/Laisky/go-ramjet/internal/tasks/gptchat/tasks"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/utils"
	"github.com/Laisky/go-ramjet/library/log"
)

var (
	urlContentCache = gutils.NewExpCache[[]byte](context.Background(), time.Hour)
	urlRegexp       = regexp.MustCompile(`https:\/\/(?:www\.)?[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[a-zA-Z0-9()]{1,6}\b(?:[-a-zA-Z0-9()@:%_\+.~#?&//=]*)`)
)

// FetchURLContent fetch url content
func FetchURLContent(gctx *gin.Context, url string) (content []byte, err error) {
	// check cache
	content, ok := urlContentCache.Load(url)
	if ok {
		log.Logger.Debug("hit cache for query mentioned url", zap.String("url", url))
		return content, nil
	}

	ctx, cancel := context.WithTimeout(gmw.Ctx(gctx), 25*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "new request %q", url)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537")
	resp, err := httpcli.Do(req) // nolint:bodyclose
	if err != nil {
		return nil, errors.Wrapf(err, "do request %q", url)
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger)

	contentType := resp.Header.Get("Content-Type")
	switch {
	case strings.Contains(contentType, "text/html") ||
		strings.Contains(contentType, "application/xhtml+xml"):
		content, err = gptTasks.FetchDynamicURLContent(ctx, url)
	default:
		content, err = fetchStaticURLContent(ctx, url)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "fetch url %q", url)
	}

	// update cache
	urlContentCache.Store(url, content)

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

var webSearchQueryPrompt = gutils.Dedent(`
	Do not directly answer the user's question, but rather analyze the
	user's question in the role of a decision-making system scheduler.
	Consider what additional information is needed to better answer the user's question.
	you can use following python functions to fetch the information from web,
	you can call each function multiple times if necessary.

	* def search_web(query: str) -> str

	Just return the function calls in valid python syntax, don’t answer the user’s question directly!

	>>> following is user prompt:
	`)

var functionCallsRegexp = regexp.MustCompile(
	`search_web\(\\?['"](?P<searchQuery>[^'"\)]*?)\\?['"]\)`)

func (r *FrontendReq) embeddingGoogleSearch(gctx *gin.Context, user *config.UserConfig) {
	logger := gmw.GetLogger(gctx)
	logger.Debug("embedding google search")

	if len(r.Messages) == 0 {
		return
	}

	var lastUserPrompt *string
	for i := len(r.Messages) - 1; i >= 0; i-- {
		if r.Messages[i].Role != OpenaiMessageRoleUser {
			continue
		}

		lastUserPrompt = &r.Messages[i].Content
		break
	}

	if lastUserPrompt == nil { // no user prompt
		return
	}

	functionCalling, err := OneshotChat(gmw.Ctx(gctx),
		user, defaultChatModel, "",
		webSearchQueryPrompt+"\n\n"+*lastUserPrompt)
	if err != nil {
		logger.Error("google search query", zap.Error(err))
		return
	}

	// parse function calls
	matches := functionCallsRegexp.FindAllStringSubmatch(functionCalling, -1)
	if len(matches) == 0 {
		logger.Debug("no function calls found in response",
			zap.String("response", functionCalling))
		return
	}

	var pool errgroup.Group
	var mu sync.Mutex
	var additionalText []string
	for i, match := range matches {
		if i > 5 {
			logger.Debug("too many function calls, skip",
				zap.String("match", match[0]))
			break
		}

		match := match
		if len(match) != 2 {
			logger.Debug("invalid function call match",
				zap.String("match", match[0]))
			continue
		}

		searchQuery := match[1]
		if len(searchQuery) == 0 {
			logger.Debug("empty search query")
			continue
		}

		pool.Go(func() error {
			extra, err := webSearch(gmw.Ctx(gctx), searchQuery, user)
			if err != nil {
				return errors.Wrapf(err, "web search %q", searchQuery)
			}

			if len([]rune(extra)) > 20000 {
				extra, err = queryChunks(gctx, queryChunksArgs{
					user:    user,
					query:   searchQuery,
					ext:     ".txt",
					model:   r.Model,
					content: []byte(extra),
				})
				if err != nil {
					log.Logger.Warn("query chunks for search result", zap.Error(err))
				}
			}

			// trim extra content
			limit := 4000 // for paid user
			if user.IsFree {
				limit = user.LimitPromptTokenLength / 5
			}

			extra = strings.TrimSpace(utils.TrimByTokens("", extra, limit))

			mu.Lock()
			defer mu.Unlock()

			if len(extra) != 0 {
				additionalText = append(additionalText, extra)
			}

			additionalText = append(additionalText, extra)
			return nil
		})
	}

	if err := pool.Wait(); err != nil {
		logger.Error("query mentioned urls", zap.Error(err))
	}

	if len(additionalText) != 0 {
		*lastUserPrompt += "\n\nfollowing are auxiliary content just for your reference:\n\n" +
			strings.Join(additionalText, "\n")
	}
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
		break
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
			content, err := FetchURLContent(gctx, url)
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

// queryChunksArgs args for queryChunks
type queryChunksArgs struct {
	// user who send the request
	user *config.UserConfig
	// query is the user query
	query string
	// ext is the file extension of content, like .txt, .md, .html
	ext string
	// model is the name of LLM model to use
	model string
	// content is the content to query
	content []byte
}

func queryChunks(gctx *gin.Context, args queryChunksArgs) (result string, err error) {
	log.Logger.Debug("query ramjet to search chunks",
		zap.String("ext", args.ext))

	reqData := map[string]any{
		"content":    base64.StdEncoding.EncodeToString(args.content),
		"query":      args.query,
		"ext":        args.ext,
		"model":      args.model,
		"max_chunks": 10000,
	}

	if args.user.IsFree {
		reqData["max_chunks"] = 500
	}

	postBody, err := json.Marshal(reqData)
	if err != nil {
		return "", errors.Wrap(err, "marshal post body")
	}

	queryChunkURL := fmt.Sprintf("%s/gptchat/query/chunks", config.Config.RamjetURL)

	queryCtx, queryCancel := context.WithTimeout(gmw.Ctx(gctx), 180*time.Second)
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
