package http

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/go-utils/v6/json"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/web"
)

// func convert2OpenaiRequest(ctx *gin.Context) (frontendReq *FrontendReq, openaiReq *http.Request, err error) {
// 	logger := gmw.GetLogger(ctx)
// 	var quotaReservation *TokenReservation
// 	defer func() {
// 		if err != nil {
// 			if quotaReservation != nil {
// 				if finalizeErr := quotaReservation.Finalize(gmw.Ctx(ctx), 0); finalizeErr != nil {
// 					logger.Warn("rollback token reservation", zap.Error(finalizeErr))
// 				}
// 			}
// 			clearTokenReservation(ctx)
// 		}
// 	}()
// 	user, err := getUserByAuthHeader(ctx)
// 	if err != nil {
// 		return nil, errors.Wrap(err, "get user")
// 	}

// 	newUrl := fmt.Sprintf("%s/%s", user.APIBase, "v1/chat/completions")
// 	if ctx.Request.URL.RawQuery != "" {
// 		newUrl += "?" + ctx.Request.URL.RawQuery
// 	}

// 	var reqBody []byte
// 	if gutils.Contains([]string{http.MethodPost, http.MethodPut}, ctx.Request.Method) {
// 		frontendReq, err = bodyChecker(ctx.Request.Body)
// 		if err != nil {
// 			return nil, errors.Wrap(err, "request is illegal")
// 		}

// 		// enhance user query
// 		if config.Config.RamjetURL != "" &&
// 			frontendReq.LaiskyExtra != nil &&
// 			!frontendReq.LaiskyExtra.ChatSwitch.DisableHttpsCrawler {
// 			frontendReq.embeddingUrlContent(ctx, user)
// 		}

// 		if frontendReq.LaiskyExtra != nil &&
// 			frontendReq.LaiskyExtra.ChatSwitch.EnableGoogleSearch {
// 			// treat google search as expensive operation
// 			if user.IsFree {
// 				ratelimitCost := gconfig.Shared.GetInt("openai.rate_limit_expensive_models_interval_secs")
// 				if !expensiveModelRateLimiter.AllowN(ratelimitCost) {
// 					return nil, errors.New("web search is limited for free users"
// 						"you need upgrade to a paid membership to enable this feature unlimitedly, " +
// 						"more info at https://wiki.laisky.com/projects/gpt/pay/")
// 				}
// 			}

// 			frontendReq.embeddingGoogleSearch(ctx, user)
// 		}

// 		// fmt.Println(frontendReq.Messages)
// 		frontendReq.LaiskyExtra = nil

// 		if err := IsModelAllowed(ctx, user, frontendReq); err != nil {
// 			return nil, errors.Wrapf(err, "check is model allowed for user %q", user.UserName)
// 		}

// 		if frontendReq != nil && len(frontendReq.Messages) > 0 {
// 			reservation, reserveErr := ReserveTokens(ctx, user, frontendReq)
// 			if reserveErr != nil {
// 				var quotaErr *QuotaExceededError
// 				if errors.As(reserveErr, &quotaErr) {
// 					if quotaErr.RetryAfter > 0 {
// 						secs := int(math.Ceil(quotaErr.RetryAfter.Seconds()))
// 						if secs < 1 {
// 							secs = 1
// 						}
// 						ctx.Header("Retry-After", strconv.Itoa(secs))
// 					}
// 					return nil, errors.Errorf(
// 						"Free-tier quota exceeded: you can use up to %d tokens every 10-minute window, you have used %d tokens. Please wait about %s before trying again, or upgrade to a paid membership at https://wiki.laisky.com/projects/gpt/pay/.",
// 						quotaErr.Limit,
// 						quotaErr.Used,
// 						formatQuotaRetryAfter(quotaErr.RetryAfter),
// 					)
// 				}

// 				return nil, errors.Wrap(reserveErr, "reserve token quota")
// 			}

// 			quotaReservation = reservation
// 		}

// 		if strings.HasPrefix(frontendReq.Model, "o") ||
// 			strings.HasPrefix(frontendReq.Model, "claude-") ||
// 			strings.HasPrefix(frontendReq.Model, "gpt-oss") ||
// 			strings.HasPrefix(frontendReq.Model, "gpt-5") &&
// 				frontendReq.ReasoningEffort == "" {
// 			frontendReq.ReasoningEffort = "high"
// 		}

// 		var openaiReq any
// 		// lastMessage := frontendReq.Messages[len(frontendReq.Messages)-1]
// 		var nImages int
// 		for _, msg := range frontendReq.Messages {
// 			nImages += len(msg.Files)
// 		}

// 	MODEL_SWITCH:
// 		switch frontendReq.Model {
// 		case "gpt-4-turbo-preview",
// 			"gpt-4-1106-preview",
// 			"gpt-4-0125-preview",
// 			"gpt-4",
// 			"gpt-4-0613",
// 			"gpt-4-32k",
// 			"gpt-4-32k-0613",
// 			"gpt-3.5-turbo-16k",
// 			"gpt-3.5-turbo-16k-0613",
// 			"gpt-3.5-turbo",
// 			"gpt-3.5-turbo-0613",
// 			"gpt-3.5-turbo-1106",
// 			"gpt-3.5-turbo-0125",
// 			"o1-mini",
// 			"o3-mini",
// 			"o4-mini",
// 			"openai/gpt-oss-20b",
// 			"openai/gpt-oss-120b",
// 			"claude-instant-1",
// 			"claude-2",
// 			// "mixtral-8x7b-32768",
// 			"gemma2-9b-it",
// 			"gemma-3-27b-it",
// 			"llama3-8b-8192",
// 			"llama3-70b-8192",
// 			"llama-3.1-8b-instant",
// 			"llama-3.3-70b-versatile",
// 			"llama-3.1-405b-instruct",
// 			"qwen-qwq-32b",
// 			"qwen/qwen3-32b",
// 			"moonshotai/kimi-k2-instruct",
// 			"moonshotai/kimi-k2-instruct-0905",
// 			"deepseek-chat",
// 			"deepseek-reasoner",
// 			"deepseek-coder",
// 			"gemini-pro":
// 			req := new(OpenaiChatReq[string])
// 			if err := copier.Copy(req, frontendReq); err != nil {
// 				return nil, errors.Wrap(err, "copy to chat req")
// 			}

// 			openaiReq = req
// 		case "claude-3.7-sonnet-thinking":
// 			if frontendReq.MaxTokens <= 1024 {
// 				return nil, errors.Errorf("max tokens should be greater than 1024")
// 			}
// 			frontendReq.TopP = 0
// 			frontendReq.Model = strings.TrimSuffix(frontendReq.Model, "-thinking")

// 			if frontendReq.Thinking == nil {
// 				frontendReq.Thinking = &Thinking{
// 					Type:         "enabled",
// 					BudgetTokens: int(math.Min(1024, float64(frontendReq.MaxTokens/2))),
// 				}
// 			}

// 			if nImages == 0 {
// 				req := new(OpenaiChatReq[string])
// 				if err := copier.Copy(req, frontendReq); err != nil {
// 					return nil, errors.Wrap(err, "copy to chat req")
// 				}

// 				openaiReq = req
// 				break MODEL_SWITCH
// 			}

// 			openaiReq, err = processVisionRequest(user, frontendReq)
// 			if err != nil {
// 				return nil, errors.Wrap(err, "process vision request")
// 			}
// 		case "claude-3-opus", // support text and vision at the same time
// 			"claude-3.5-sonnet",
// 			"claude-3.5-sonnet-8k",
// 			"claude-3.7-sonnet",
// 			"claude-3-haiku",
// 			"claude-3.5-haiku",
// 			"claude-haiku-4-5",
// 			"claude-4-opus",
// 			"claude-4.1-opus",
// 			"claude-opus-4-5",
// 			"claude-4-sonnet",
// 			"claude-sonnet-4-5",
// 			"o1",
// 			"o1-preview",
// 			"o3",
// 			"o3-pro",
// 			"gpt-4.1",
// 			"gpt-4.1-mini",
// 			"gpt-4.1-nano",
// 			"gpt-5",
// 			"gpt-5-pro",
// 			"gpt-5-codex",
// 			"gpt-5-mini",
// 			"gpt-5-nano",
// 			"gpt-5.1",
// 			"gpt-5.1-codex",
// 			"gpt-5.2",
// 			"gpt-5.2-pro",
// 			"gpt-4o",
// 			"gpt-4o-search-preview",
// 			"gpt-4o-mini",
// 			"gpt-4o-mini-search-preview",
// 			"gpt-4-turbo-2024-04-09",
// 			"gpt-4-turbo",
// 			"gemini-2.0-pro",
// 			"gemini-2.5-pro",
// 			"gemini-2.0-flash",
// 			"gemini-2.5-flash",
// 			"gemini-2.0-flash-thinking",
// 			"gemini-2.0-flash-exp-image-generation",
// 			"gemini-2.5-flash-image-preview",
// 			"gemini-3-pro-preview":
// 			if nImages == 0 { // no images, text only
// 				req := new(OpenaiChatReq[string])
// 				if err := copier.Copy(req, frontendReq); err != nil {
// 					return nil, errors.Wrap(err, "copy to chat req")
// 				}
// 				openaiReq = req
// 				break MODEL_SWITCH
// 			}

// 			openaiReq, err = processVisionRequest(user, frontendReq)
// 			if err != nil {
// 				return nil, errors.Wrap(err, "process vision request")
// 			}
// 		case "gpt-4-vision-preview", // only support vision
// 			"gemini-pro-vision":
// 			openaiReq, err = processVisionRequest(user, frontendReq)
// 			if err != nil {
// 				return nil, errors.Wrap(err, "process vision request")
// 			}
// 		case "text-davinci-003":
// 			newUrl = fmt.Sprintf("%s/%s", user.APIBase, "v1/completions")
// 			openaiReq = new(OpenaiCompletionReq)
// 			if err := copier.Copy(openaiReq, frontendReq); err != nil {
// 				return nil, errors.Wrap(err, "copy to completion req")
// 			}
// 		default:
// return nil, errors.Errorf("unsupport chat model %q", frontendReq.Model)
// 			req := new(OpenaiChatReq[string])
// 			if err := copier.Copy(req, frontendReq); err != nil {
// 				return nil, errors.Wrap(err, "copy to chat req")
// 			}

// 			openaiReq = req
// 		}

// 		if reqBody, err = json.Marshal(openaiReq); err != nil {
// 			return nil, errors.Wrap(err, "marshal new body")
// 		}

// 		logger.Debug("prepare request to upstream server") // zap.ByteString("payload", reqBody),

// 	}

// 	if frontendReq == nil {
// 		frontendReq = &FrontendReq{}
// 	}

// 	logger.Debug("send request to upstream server",
// 		zap.String("url", newUrl),
// 		zap.ByteString("payload", reqBody))
// 	req, err := http.NewRequestWithContext(gmw.Ctx(ctx),
// 		ctx.Request.Method, newUrl, bytes.NewReader(reqBody))
// 	if err != nil {
// 		return nil, errors.Wrap(err, "new request")
// 	}
// 	CopyHeader(req.Header, ctx.Request.Header)
// 	req.Header.Set("authorization", "Bearer "+user.OpenaiToken)

// 	// if set header "Accept-Encoding" manually,
// 	// golang's http client will not auto decompress response body
// 	req.Header.Del("Accept-Encoding")

// 	return frontendReq, req, nil
// }

func req2CacheKey(req *FrontendReq) (string, error) {
	if req == nil {
		return "", errors.New("empty frontend request")
	}

	data, err := json.Marshal(req)
	if err != nil {
		return "", errors.Wrap(err, "marshal req")
	}

	hashed := sha1.Sum(data)
	return hex.EncodeToString(hashed[:]), nil
}

// func tryExtractCompletionTokens(body []byte) (int, bool) {
// 	if len(body) == 0 {
// 		return 0, false
// 	}

// 	var payload struct {
// 		Usage struct {
// 			CompletionTokens int `json:"completion_tokens"`
// 		} `json:"usage"`
// 	}
// 	if err := json.Unmarshal(body, &payload); err != nil {
// 		return 0, false
// 	}

// 	if payload.Usage.CompletionTokens < 0 {
// 		return 0, false
// 	}

// 	return payload.Usage.CompletionTokens, true
// }

// func formatQuotaRetryAfter(d time.Duration) string {
// 	if d <= 0 {
// 		return "a few seconds"
// 	}

// 	if d < time.Minute {
// 		secs := int(math.Ceil(d.Seconds()))
// 		if secs <= 1 {
// 			return "1 second"
// 		}
// 		return fmt.Sprintf("%d seconds", secs)
// 	}

// 	minutes := int(math.Ceil(d.Minutes()))
// 	if minutes <= 1 {
// 		return "1 minute"
// 	}

// 	return fmt.Sprintf("%d minutes", minutes)
// }

// streamHeartbeatInterval is the period between SSE heartbeats for most clients.
var streamHeartbeatInterval = 10 * time.Second

// safariStreamHeartbeatInterval is the shorter heartbeat period used for Safari,
// which drops idle event streams sooner.
var safariStreamHeartbeatInterval = 5 * time.Second

// enableHeartBeatForStreamReq starts periodic SSE heartbeat frames on the response
// of a streaming request; Safari clients also get one heartbeat immediately.
//
// Periodic heartbeats are SSE comment frames, which spec-compliant clients ignore,
// so they never surface as events. Every frame is written under the gin-middlewares
// context lock, so it never interleaves with stream chunks written under the same
// lock; a periodic frame is skipped while that lock is busy. Heartbeats stop when
// the request context ends or when the returned stop func is called.
//
// Parameters:
//   - gctx: Gin context of the streaming request; its writer lock must be installed.
//
// It returns stop, which ends the heartbeats and waits until the heartbeat goroutine
// has exited, so nothing is written after stop returns. The goroutine never blocks
// on the writer lock, so stop cannot hang even if a writer panicked while holding
// it. Call stop before writing the terminal [DONE] frame and before the handler
// returns. Calling stop more than once is safe.
func enableHeartBeatForStreamReq(gctx *gin.Context) (stop func()) {
	ctx := gmw.Ctx(gctx)
	logger := gmw.GetLogger(gctx)

	if _, err := gmw.GetLockFromCtx(ctx); err != nil {
		logger.Debug("stream heartbeat disabled: no writer lock in context", zap.Error(err))
		return func() {}
	}

	// Detect iOS Safari
	userAgent := gctx.Request.UserAgent()
	isSafari := strings.Contains(userAgent, "Safari") && !strings.Contains(userAgent, "Chrome")

	// Set appropriate heartbeat interval
	heartbeatInterval := streamHeartbeatInterval
	if isSafari {
		heartbeatInterval = safariStreamHeartbeatInterval
	}

	// Send initial heartbeat for Safari
	if isSafari {
		if err := writeStreamHeartbeat(ctx, gctx, safariInitialHeartbeatFrame, true); err != nil {
			logger.Warn("failed to send initial heartbeat", zap.Error(err))
			return func() {}
		}
	}

	heartCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)

		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-heartCtx.Done():
				return
			case <-ticker.C:
				if err := writeStreamHeartbeat(heartCtx, gctx, streamHeartbeatFrame, false); err != nil {
					logger.Debug("stop stream heartbeat", zap.Error(err))
					return
				}
			}
		}
	}()

	return sync.OnceFunc(func() {
		cancel()
		<-done
	})
}

const (
	// streamHeartbeatFrame is the periodic keep-alive: an SSE comment frame, which
	// spec-compliant parsers (and the web client) ignore instead of parsing as data.
	streamHeartbeatFrame = ": heartbeat\n\n"
	// safariInitialHeartbeatFrame is the unchanged legacy frame sent once to Safari
	// when the stream starts.
	safariInitialHeartbeatFrame = ": connection established\ndata: [HEARTBEAT]\n\n"
)

// writeStreamHeartbeat writes frame to the response and flushes it while holding
// the gin-middlewares context lock shared with the stream writer.
//
// Parameters:
//   - ctx: Context carrying the gin context and its lock; once it is done the
//     frame is skipped, because the stream is ending.
//   - gctx: Gin context whose writer receives the frame.
//   - frame: Complete SSE frame to write.
//   - wait: Whether to wait for the lock. When false and the lock is busy, the
//     frame is skipped: the stream is writing anyway, and the caller must never
//     block behind a writer that may have panicked while holding the lock.
//
// It returns an error when the lock is unavailable or the write fails.
func writeStreamHeartbeat(ctx context.Context, gctx *gin.Context, frame string, wait bool) error {
	lock, err := gmw.GetLockFromCtx(ctx)
	if err != nil {
		return errors.Wrap(err, "get stream writer lock")
	}

	if wait {
		lock.Lock()
	} else if !lock.TryLock() {
		return nil
	}
	defer lock.Unlock()

	select {
	case <-ctx.Done():
		return nil // the stream is ending; drop the frame
	default:
	}

	if _, err := io.WriteString(gctx.Writer, frame); err != nil {
		return errors.Wrap(err, "write heartbeat frame")
	}
	gctx.Writer.Flush()
	return nil
}

func bodyChecker(body io.ReadCloser) (userReq *FrontendReq, err error) {
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

	return userReq, err
}

// OneShotChatHandler handle one shot chat request
func OneShotChatHandler(gctx *gin.Context) {
	user, err := getUserByAuthHeader(gctx)
	if web.AbortErr(gctx, err) {
		return
	}

	req := new(OneShotChatRequest)
	if err := gctx.BindJSON(req); web.AbortErr(gctx, err) {
		return
	}

	resp, err := userOneshotChat(gmw.Ctx(gctx), user, "", req.SystemPrompt, req.UserPrompt)
	if web.AbortErr(gctx, err) {
		return
	}

	gctx.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}
