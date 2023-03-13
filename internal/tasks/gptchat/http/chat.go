package http

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/go-ramjet/library/log"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/copier"
)

func AbortErr(ctx *gin.Context, err error) bool {
	if err == nil {
		return false
	}

	log.Logger.Error("openai chat abort", zap.Error(err))
	ctx.AbortWithStatusJSON(http.StatusBadRequest, fmt.Sprintf("%+v", err))
	return true
}

var (
	dataReg = regexp.MustCompile(`data: (\{.*\})`)
)

func APIHandler(ctx *gin.Context) {
	defer ctx.Request.Body.Close()
	logger := log.Logger.Named("chat")

	resp, err := proxy(ctx)
	if AbortErr(ctx, err) {
		return
	}
	defer resp.Body.Close()

	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("X-Accel-Buffering", "no")
	ctx.Header("Transfer-Encoding", "chunked")
	for k, v := range resp.Header {
		ctx.Header(k, strings.Join(v, ";"))
	}

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

	for reader.Scan() {
		line := reader.Bytes()
		logger.Debug("got response line", zap.ByteString("line", line))

		var chunk []byte
		if matched := dataReg.FindAllSubmatch(line, -1); len(matched) != 0 {
			chunk = matched[0][1]
		}

		resp := new(OpenaiCOmpletionStreamResp)
		err = gutils.JSON.Unmarshal(chunk, resp)
		if AbortErr(ctx, err) {
			return
		}

		_, err = io.Copy(ctx.Writer, bytes.NewReader(append(line, []byte("\n\n")...)))
		if AbortErr(ctx, err) {
			return
		}

		// check if resp is end
		if !isStream ||
			len(resp.Choices) == 0 ||
			resp.Choices[0].FinishReason != "" {
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

	body := ctx.Request.Body
	var frontendReq *FrontendReq
	if gutils.Contains([]string{http.MethodPost, http.MethodPut}, ctx.Request.Method) {
		frontendReq, err = bodyChecker(ctx.Request.Body)
		if err != nil {
			return nil, errors.Wrap(err, "request is illegal")
		}

		var openaiReq any
		switch frontendReq.Model {
		case "gpt-3.5-turbo":
			newUrl = fmt.Sprintf("%s/%s", gconfig.Shared.GetString("openai.api"), "v1/chat/completions")
			req := new(OpenaiChatReq)
			if err := copier.Copy(req, frontendReq); err != nil {
				return nil, errors.Wrap(err, "copy to chat req")
			}

			// if frontendReq.StaticContext != "" {
			// 	req.Messages = append([]OpenaiReqMessage{{
			// 		Role:    "user",
			// 		Content: frontendReq.StaticContext,
			// 	}}, req.Messages...)
			// }

			openaiReq = req
		case "code-davinci-002":
			newUrl = fmt.Sprintf("%s/%s", gconfig.Shared.GetString("openai.api"), "v1/completions")
			openaiReq = new(OpenaiCompletionReq)
			if err := copier.Copy(openaiReq, frontendReq); err != nil {
				return nil, errors.Wrap(err, "copy to completion req")
			}
		default:
			return nil, errors.Errorf("unknown model %q", frontendReq.Model)
		}

		payload, err := gutils.JSON.Marshal(openaiReq)
		if err != nil {
			return nil, errors.Wrap(err, "marshal new body")
		}
		log.Logger.Debug("send request", zap.ByteString("req", payload))
		body = io.NopCloser(bytes.NewReader(payload))
	}

	req, err := http.NewRequest(ctx.Request.Method, newUrl, body)
	if err != nil {
		return nil, errors.Wrap(err, "new request")
	}

	req.Header = ctx.Request.Header
	req.Header.Set("authorization", "Bearer "+gconfig.Shared.GetString("openai.token"))
	resp, err = httpcli.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "do request")
	}

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
	if err = gutils.JSON.Unmarshal(payload, data); err != nil {
		return nil, errors.Wrap(err, "parse request")
	}
	data.fillDefault()

	trimMessages(data)
	maxTokens := uint(MaxTokens())
	if data.MaxTokens > maxTokens {
		return nil, errors.Errorf("max_tokens should less than %d", maxTokens)
	}

	return data, err
}

func trimMessages(data *FrontendReq) {
	maxMessages := MaxMessages()
	maxTokens := MaxTokens()

	if len(data.Messages) > maxMessages {
		data.Messages = data.Messages[len(data.Messages)-maxMessages:]
	}

	for i := range data.Messages {
		cnt := data.Messages[i].Content
		if len(cnt) > maxTokens {
			cnt = cnt[len(cnt)-maxTokens:]
		}
	}
}