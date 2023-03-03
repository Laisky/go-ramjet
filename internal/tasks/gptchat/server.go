package gptchat

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

func setupHTTPCli() error {
	proxyurl, err := url.Parse(gconfig.Shared.GetString("openai.proxy"))
	if err != nil {
		return errors.Wrap(err, "parse proxy")
	}

	httpcli = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyurl),
			IdleConnTimeout: 30 * time.Second,
		},
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

type OpenaiReq struct {
	Model     string `json:"model"`
	MaxTokens uint   `json:"max_tokens"`
}

func bodyChecker(body io.ReadCloser) (newBody io.ReadCloser, err error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return nil, errors.Wrap(err, "read request body")
	}

	data := make(map[string]interface{})
	if err = gutils.JSON.Unmarshal(payload, &data); err != nil {
		return nil, errors.Wrap(err, "parse request")
	}

	// rewrite data
	data["model"] = "gpt-3.5-turbo"

	// check model
	// if v, ok := data["model"].(string); ok && v != "gpt-3.5-turbo-0301" {
	// 	return nil, errors.Errorf("only support `gpt-3.5-turbo-0301` model")
	// }
	if v, ok := data["max_tokens"].(float64); ok && v > 1000 {
		return nil, errors.Errorf("max_tokens should less than 1000")
	}

	if payload, err = gutils.JSON.Marshal(data); err != nil {
		return nil, errors.Wrap(err, "marshal new body")
	}

	return io.NopCloser(bytes.NewReader(payload)), nil
}
