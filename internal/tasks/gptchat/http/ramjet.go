package http

import (
	"io"
	"net/http"
	"strings"

	gutils "github.com/Laisky/go-utils/v4"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/log"
)

// RamjetProxyHandler proxy to ramjet url
func RamjetProxyHandler(ctx *gin.Context) {
	defer gutils.LogErr(ctx.Request.Body.Close, log.Logger)
	url := ctx.Request.URL
	targetUrl := ramjetURL + "/" + strings.TrimPrefix(
		strings.TrimPrefix(url.Path, "/"), "gptchat/ramjet/")
	targetUrl += "?" + url.RawQuery

	req, err := http.NewRequestWithContext(ctx.Request.Context(),
		ctx.Request.Method,
		targetUrl,
		ctx.Request.Body,
	)
	if AbortErr(ctx, err) {
		return
	}

	req.Header = ctx.Request.Header
	req.Header.Del("Accept-Encoding") // do not disable gzip
	if user, err := getUserFromToken(ctx); err != nil {
		AbortErr(ctx, err)
	} else {
		req.Header.Set("Authorization", "Bearer "+user.OpenaiToken)
	}

	resp, err := httpcli.Do(req)
	if AbortErr(ctx, err) {
		return
	}

	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if AbortErr(ctx, err) {
		return
	}

	for k, v := range resp.Header {
		if len(v) == 0 {
			continue
		}

		ctx.Header(k, v[0])
	}
	ctx.Data(resp.StatusCode, resp.Header.Get("Content-Type"), payload)
}
