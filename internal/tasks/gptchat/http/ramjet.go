package http

import (
	"crypto/subtle"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/web"
)

// RamjetProxyHandler proxy to ramjet url
func RamjetProxyHandler(ctx *gin.Context) {
	defer gutils.LogErr(ctx.Request.Body.Close, log.Logger)
	url := ctx.Request.URL
	if url.RawPath != "" {
		ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "A canonical Ramjet operation is required"})
		return
	}
	operationPath := "/" + strings.TrimPrefix(strings.TrimPrefix(url.Path, "/"), "gptchat/ramjet/")
	operation := *ctx.Request
	operationURL := *url
	operationURL.Path = operationPath
	operation.URL = &operationURL
	if _, routeErr := ramjetPrivateRoute(&operation); abortRamjetAccountAuth(ctx, routeErr) {
		return
	}
	targetUrl := config.Config.RamjetURL + operationPath
	targetUrl += "?" + url.RawQuery

	req, err := http.NewRequestWithContext(gmw.Ctx(ctx),
		ctx.Request.Method,
		targetUrl,
		ctx.Request.Body,
	)
	if web.AbortErr(ctx, err) {
		return
	}

	req.Header = ctx.Request.Header.Clone()
	req.Header.Del("Accept-Encoding") // do not disable gzip
	if _, routeErr := ramjetPrivateRoute(req); abortRamjetAccountAuth(ctx, routeErr) {
		return
	}
	if !ramjetMetadataOnly(req) || ctx.GetHeader("Authorization") != "" {
		if err = setUserAuth(ctx, req); web.AbortErr(ctx, err) {
			return
		}
	} else {
		// Pure account metadata never substitutes a configured server model key.
		req.Header.Del("Authorization")
		req.Header.Del("X-Laisky-Openai-Api-Base")
		req.Header.Set("X-Laisky-User-Is-Free", "true")
	}

	private, accountErr := setRamjetAccountAuth(ctx, req)
	if abortRamjetAccountAuth(ctx, accountErr) {
		return
	}
	client := httpcli
	if private {
		client = privateRamjetHTTPClient(httpcli)
	}

	resp, err := client.Do(req) //nolint: bodyclose
	if web.AbortErr(ctx, err) {
		return
	}

	defer gutils.LogErr(resp.Body.Close, log.Logger)
	if private && resp.StatusCode >= http.StatusMultipleChoices && resp.StatusCode < http.StatusBadRequest {
		// Do not expose a backend redirect for a browser to replay the session.
		ctx.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"error": "Private dataset service returned a redirect"})
		return
	}
	payload, err := io.ReadAll(resp.Body)
	if web.AbortErr(ctx, err) {
		return
	}

	for k, v := range resp.Header {
		if len(v) == 0 {
			continue
		}

		ctx.Header(k, v[0])
	}
	ctx.Header("Access-Control-Expose-Headers", "x-oneapi-request-id, x-request-id")
	ctx.Data(resp.StatusCode, resp.Header.Get("Content-Type"), payload)
}

// setUserAuth parse and set user auth to request header
func setUserAuth(gctx *gin.Context, req *http.Request) error {
	user, err := resolveRamjetUser(gctx)
	if err != nil {
		return errors.Wrap(err, "get user from token")
	}

	req.Header.Set("X-Laisky-Openai-Api-Base", user.APIBase)
	req.Header.Set("X-Laisky-User-Id", "public")
	req.Header.Del(ramjetSessionHeader)
	// Ramjet has no authoritative paid-account grant for these BYOK requests.
	// Untrusted caller state must never select its privileged resource budget.
	req.Header.Set("X-Laisky-User-Is-Free", "true")

	// if set header "Accept-Encoding" manually,
	// golang's http client will not auto decompress response body
	req.Header.Del("Accept-Encoding")

	token := user.OpenaiToken
	if strings.HasPrefix(req.URL.Path, "/gptchat/image/") {
		if subtle.ConstantTimeCompare([]byte(user.ImageToken), []byte(token)) != 1 {
			return errors.New("Ramjet requires matching caller-owned image credentials")
		}
		token = user.ImageToken
		model := "image-" + strings.TrimPrefix(req.URL.Path, "/gptchat/image/")
		if err = IsModelAllowed(gctx, user, &FrontendReq{Model: model}); err != nil {
			return errors.Wrapf(err, "check model %q", model)
		}
	}
	// Ramjet accepts only caller-owned keys. Inference, including billing,
	// stays on the API selected by the caller.
	req.Header.Set("Authorization", token)

	return nil
}
