package gptchat

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/copier"

	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/web"
)

var (
	once    sync.Once
	httpcli *http.Client
)

func setupHTTPCli() (err error) {
	httpcli, err = gutils.NewHTTPClient(
		gutils.WithHTTPClientTimeout(30*time.Second),
		gutils.WithHTTPClientProxy(gconfig.Shared.GetString("openai.proxy")),
	)
	if err != nil {
		return errors.Wrap(err, "new http client")
	}

	return nil
}

func bindHTTP() {
	if err := setupHTTPCli(); err != nil {
		log.Logger.Panic("setup http client", zap.Error(err))
	}

	web.Server.Any("/chat/*any", func(ctx *gin.Context) {
		defer ctx.Request.Body.Close()

		resp, err := proxy(ctx)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, fmt.Sprintf("%+v", err))
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, fmt.Sprintf("%+v", err))
			return
		}

		ctx.Data(resp.StatusCode, resp.Header.Get(gutils.HTTPHeaderContentType), body)
	})
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

			if frontendReq.StaticContext != "" {
				req.Messages = append([]OpenaiReqMessage{{
					Role:    "user",
					Content: frontendReq.StaticContext,
				}}, req.Messages...)
			}

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
	r.MaxTokens = gutils.OptionalVal(&r.MaxTokens, 500)
	r.Temperature = gutils.OptionalVal(&r.Temperature, 1)
	r.TopP = gutils.OptionalVal(&r.TopP, 1)
	r.N = gutils.OptionalVal(&r.N, 1)
	r.Model = gutils.OptionalVal(&r.Model, gconfig.Shared.GetString("openai.default_model"))
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
	maxTokens := gconfig.Shared.GetInt("openai.max_tokens")
	if data.MaxTokens > uint(maxTokens) {
		return nil, errors.Errorf("max_tokens should less than %d", maxTokens)
	}

	return data, err
}

func trimMessages(data *FrontendReq) {
	maxSessions := gconfig.Shared.GetInt("openai.max_sessions")
	maxTokens := gconfig.Shared.GetInt("openai.max_tokens")

	if len(data.Messages) > maxSessions {
		data.Messages = data.Messages[len(data.Messages)-maxSessions:]
	}

	for i := range data.Messages {
		cnt := data.Messages[i].Content
		if len(cnt) > maxTokens {
			cnt = cnt[len(cnt)-maxTokens:]
		}
	}
}
