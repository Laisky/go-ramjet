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
			ctx.AbortWithStatusJSON(http.StatusBadRequest, err.Error())
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, err.Error())
			return
		}

		ctx.Data(resp.StatusCode, resp.Header.Get(gutils.HTTPHeaderContentType), body)
	})
}

func proxy(ctx *gin.Context) (resp *http.Response, err error) {
	path := strings.TrimPrefix(ctx.Request.URL.Path, "/chat")
	newUrl := fmt.Sprintf("%s%s",
		strings.Trim(gconfig.Shared.GetString("openai.api"), "/"),
		path,
	)

	if ctx.Request.URL.RawQuery != "" {
		newUrl += "?" + ctx.Request.URL.RawQuery
	}

	body := ctx.Request.Body
	if ctx.Request.Method == http.MethodPost {
		body, err = bodyChecker(ctx.Request.Body)
		if err != nil {
			return nil, errors.Wrap(err, "request is illegal")
		}
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

type OpenaiReqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenaiReq struct {
	Model            string             `json:"model"`
	MaxTokens        uint               `json:"max_tokens"`
	Messages         []OpenaiReqMessage `json:"messages"`
	PresencePenalty  float64            `json:"presence_penalty"`
	FrequencyPenalty float64            `json:"frequency_penalty"`
	Stream           bool               `json:"stream"`
	Temperature      float64            `json:"temperature"`
	TopP             float64            `json:"top_p"`
	N                int                `json:"n"`
	// BestOf           int                `json:"best_of"`
}

func (r *OpenaiReq) fillDefault() {
	r.MaxTokens = gutils.OptionalVal(&r.MaxTokens, 500)
	r.Temperature = gutils.OptionalVal(&r.Temperature, 1)
	r.TopP = gutils.OptionalVal(&r.TopP, 1)
	r.N = gutils.OptionalVal(&r.N, 1)
	// r.BestOf = gutils.OptionalVal(&r.BestOf, 1)
}

func bodyChecker(body io.ReadCloser) (newBody io.ReadCloser, err error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return nil, errors.Wrap(err, "read request body")
	}

	data := new(OpenaiReq)
	if err = gutils.JSON.Unmarshal(payload, data); err != nil {
		return nil, errors.Wrap(err, "parse request")
	}
	data.fillDefault()

	// rewrite data
	data.Model = gconfig.Shared.GetString("openai.model")
	trimMessages(data)
	if data.MaxTokens > 1000 {
		return nil, errors.Errorf("max_tokens should less than 1000")
	}

	if payload, err = gutils.JSON.Marshal(data); err != nil {
		return nil, errors.Wrap(err, "marshal new body")
	}

	return io.NopCloser(bytes.NewReader(payload)), nil
}

func trimMessages(data *OpenaiReq) {
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
