package http

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/go-utils/v4/json"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/copier"

	"github.com/Laisky/go-ramjet/library/log"
)

var (
	ratelimiter *gutils.Throttle
	dataReg     = regexp.MustCompile(`data: (\{.*\})`)
)

func init() {
	var err error
	if ratelimiter, err = gutils.NewThrottleWithCtx(context.Background(), &gutils.ThrottleCfg{
		Max:     10,
		NPerSec: 1,
	}); err != nil {
		log.Logger.Panic("new ratelimiter", zap.Error(err))
	}
}

func APIHandler(ctx *gin.Context) {
	defer ctx.Request.Body.Close() // nolint: errcheck,gosec
	logger := log.Logger.Named("chat")

	if !ratelimiter.Allow() { // check rate limit
		ctx.AbortWithStatusJSON(http.StatusTooManyRequests, "too many requests, please try again later")
		return
	}

	resp, err := proxy(ctx)
	if AbortErr(ctx, err) {
		return
	}
	defer resp.Body.Close() // nolint: errcheck,gosec

	// ctx.Header("Content-Type", "text/event-stream")
	// ctx.Header("Cache-Control", "no-cache")
	// ctx.Header("X-Accel-Buffering", "no")
	// ctx.Header("Transfer-Encoding", "chunked")
	CopyHeader(ctx.Writer.Header(), resp.Header)

	isStream := resp.Header.Get("Content-Type") == "text/event-stream"
	reader := bufio.NewScanner(resp.Body)
	reader.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF {
			return 0, nil, io.EOF
		}

		if i := bytes.Index(data, []byte("\n\n")); i >= 0 {
			return i + 1, data[0:i], nil
		}

		return 0, nil, nil
	})

	var lastResp *OpenaiCOmpletionStreamResp
	for reader.Scan() {
		line := reader.Bytes()
		logger.Debug("got response line", zap.ByteString("line", line))

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

		// check if resp is end
		if !isStream ||
			len(lastResp.Choices) == 0 ||
			lastResp.Choices[0].FinishReason != "" {
			return
		}
	}

	// write last line
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
	}
}

func proxy(ctx *gin.Context) (resp *http.Response, err error) {
	path := strings.TrimPrefix(ctx.Request.URL.Path, "/chat")
	newUrl := fmt.Sprintf("%s%s",
		gconfig.Shared.GetString("openai.api"),
		path,
	)

	if ctx.Request.URL.RawQuery != "" {
		newUrl += "?" + ctx.Request.URL.RawQuery
	}

	user, err := getUserFromToken(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get user")
	}

	body := ctx.Request.Body
	var frontendReq *FrontendReq
	if gutils.Contains([]string{http.MethodPost, http.MethodPut}, ctx.Request.Method) {
		frontendReq, err = bodyChecker(ctx.Request.Body)
		if err != nil {
			return nil, errors.Wrap(err, "request is illegal")
		}

		if !user.IsModelAllowed(frontendReq.Model) {
			return nil, errors.Errorf("model is not allowed for current user %q", user.UserName)
		}

		var openaiReq any
		switch frontendReq.Model {
		case "gpt-3.5-turbo", "gpt-4":
			newUrl = fmt.Sprintf("%s/%s", gconfig.Shared.GetString("openai.api"), "v1/chat/completions")
			req := new(OpenaiChatReq)
			if err := copier.Copy(req, frontendReq); err != nil {
				return nil, errors.Wrap(err, "copy to chat req")
			}

			openaiReq = req
		case "text-davinci-003":
			newUrl = fmt.Sprintf("%s/%s", gconfig.Shared.GetString("openai.api"), "v1/completions")
			openaiReq = new(OpenaiCompletionReq)
			if err := copier.Copy(openaiReq, frontendReq); err != nil {
				return nil, errors.Wrap(err, "copy to completion req")
			}
		default:
			return nil, errors.Errorf("unsupport chat model %q", frontendReq.Model)
		}

		payload, err := json.Marshal(openaiReq)
		if err != nil {
			return nil, errors.Wrap(err, "marshal new body")
		}
		log.Logger.Debug("prepare request", zap.ByteString("req", payload))
		body = io.NopCloser(bytes.NewReader(payload))
	}

	req, err := http.NewRequest(ctx.Request.Method, newUrl, body)
	if err != nil {
		return nil, errors.Wrap(err, "new request")
	}
	req = req.WithContext(ctx.Request.Context())
	CopyHeader(req.Header, ctx.Request.Header)
	req.Header.Set("authorization", "Bearer "+user.OpenaiToken)

	log.Logger.Debug("proxy request", zap.String("url", newUrl))
	resp, err = httpcli.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "do request %q", newUrl)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close() // nolint
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("[%d]%s", resp.StatusCode, string(body))
	}

	// do not close resp.Body
	return resp, nil
}

func (r *FrontendReq) fillDefault() {
	r.MaxTokens = gutils.OptionalVal(&r.MaxTokens, uint(MaxTokens()))
	r.Temperature = gutils.OptionalVal(&r.Temperature, 1)
	r.TopP = gutils.OptionalVal(&r.TopP, 1)
	r.N = gutils.OptionalVal(&r.N, 1)
	r.Model = gutils.OptionalVal(&r.Model, ChatModel())
	// r.BestOf = gutils.OptionalVal(&r.BestOf, 1)
}

func bodyChecker(body io.ReadCloser) (data *FrontendReq, err error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return nil, errors.Wrap(err, "read request body")
	}

	data = new(FrontendReq)
	if err = json.Unmarshal(payload, data); err != nil {
		return nil, errors.Wrap(err, "parse request")
	}
	data.fillDefault()

	trimMessages(data)
	maxTokens := uint(MaxTokens())
	if maxTokens != 0 && data.MaxTokens > maxTokens {
		return nil, errors.Errorf("max_tokens should less than %d", maxTokens)
	}

	return data, err
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
