package http

import (
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
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
	if err = setUserAuth(ctx, req); AbortErr(ctx, err) {
		return
	}

	resp, err := httpcli.Do(req) //nolint: bodyclose
	if AbortErr(ctx, err) {
		return
	}

	defer gutils.LogErr(resp.Body.Close, log.Logger)
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

// setUserAuth parse and set user auth to request header
func setUserAuth(ctx *gin.Context, req *http.Request) error {
	user, err := getUserFromToken(ctx)
	if err != nil {
		return errors.Wrap(err, "get user from token")
	}

	token := user.OpenaiToken

	// generate image need special token
	if strings.HasPrefix(req.URL.Path, "/gptchat/image/") {
		token = user.ImageToken

		model := "image-" + strings.TrimPrefix(req.URL.Path, "/gptchat/image/")
		if err = user.IsModelAllowed(model); err != nil {
			return errors.Wrapf(err, "check model %q", model)
		}
	}

	req.Header.Set("Authorization", token)
	return nil
}
