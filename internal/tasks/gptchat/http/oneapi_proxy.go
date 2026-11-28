package http

import (
	"context"
	"crypto/subtle"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/ssosession"
)

const oneAPIHost = "oneapi.laisky.com"

// OneapiProxyHandler forwards requests to the fixed One API host with only an
// explicitly supplied caller-owned model token. One API decides token scope.
func OneapiProxyHandler(ctx *gin.Context) {
	token, ok := callerOneAPIToken(ctx.Request.Header)
	if !ok {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "A caller-provided One API token is required"})
		return
	}

	path, ok := strings.CutPrefix(ctx.Request.URL.Path, "/gptchat/oneapi")
	if !ok || !strings.HasPrefix(path, "/") {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}

	target := &url.URL{Scheme: "https", Host: oneAPIHost, Path: path}
	relayModelRequest(ctx, ctx.Request, target, httpcli, token)
}

// callerOneAPIToken accepts one explicit One API-shaped token, bare or Bearer;
// application tokens and session credentials are not One API tokens.
func callerOneAPIToken(header http.Header) (string, bool) {
	values := header.Values("Authorization")
	if len(values) != 1 || len(header.Values("X-Api-Key")) != 0 || len(header.Values("Api-Key")) != 0 {
		return "", false
	}
	token := values[0]
	if scheme, value, hasSpace := strings.Cut(token, " "); hasSpace && strings.EqualFold(scheme, "Bearer") {
		token = value
	}
	if len(token) < 15 || !(strings.HasPrefix(token, "sk-") || strings.HasPrefix(token, "laisky-")) {
		return "", false
	}
	for _, value := range header.Values("Sec-WebSocket-Protocol") {
		for protocol := range strings.SplitSeq(value, ",") {
			if strings.HasPrefix(strings.TrimSpace(protocol), "openai-insecure-api-key.") {
				return "", false
			}
		}
	}
	if strings.IndexFunc(token, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	}) >= 0 {
		return "", false
	}
	for _, value := range header.Values(ramjetSessionHeader) {
		session, ok := ssosession.BearerToken(value)
		if ok && subtle.ConstantTimeCompare([]byte(token), []byte(session)) == 1 {
			return "", false
		}
	}
	return token, true
}

// relayModelRequest sends request to the selected target with client and token.
// It streams the response without following redirects or rewriting errors.
func relayModelRequest(ctx *gin.Context, request *http.Request, target *url.URL, client *http.Client, token string) {
	var requestCtx context.Context
	var cancel context.CancelFunc
	if client.Timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx.Request.Context(), client.Timeout)
	} else {
		requestCtx, cancel = context.WithCancel(ctx.Request.Context())
	}
	defer cancel()

	proxy := httputil.ReverseProxy{
		Transport: client.Transport,
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL.Scheme = target.Scheme
			r.Out.URL.Host = target.Host
			r.Out.URL.Path = target.Path
			r.Out.URL.RawPath = target.RawPath
			r.Out.Host = target.Host
			r.Out.Header.Del("Cookie")
			r.Out.Header.Del(ramjetSessionHeader)
			r.Out.Header.Del("X-Laisky-Api-Base")
			r.Out.Header.Del("X-Laisky-User-Id")
			r.Out.Header.Set("Authorization", "Bearer "+token)
		},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("Set-Cookie")
			resp.Header.Set("Access-Control-Expose-Headers", "x-oneapi-request-id, x-request-id")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "Model provider is unavailable", http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(ctx.Writer, request.WithContext(requestCtx))
	ctx.Writer.WriteHeaderNow()
}
