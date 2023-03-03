package gptchat

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/zap"

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

	req, err := http.NewRequest(ctx.Request.Method, newUrl, ctx.Request.Body)
	if err != nil {
		return nil, errors.Wrap(err, "new request")
	}

	req.Header = ctx.Request.Header
	resp, err = httpcli.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "do request")
	}

	return resp, nil
}
