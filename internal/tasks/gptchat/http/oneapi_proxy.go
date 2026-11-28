package http

import (
	"context"
	"crypto/subtle"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
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

// callerOneAPIToken accepts exactly one token with the One API key format.
func callerOneAPIToken(header http.Header) (string, bool) {
	token, ok := modelAuthorizationToken(header)
	return token, ok && isCallerModelToken(token) && !strings.ContainsAny(token, ".:|")
}

// modelAuthorizationToken parses the established bare or Bearer contract.
// An absent credential is distinct from one which is present but malformed.
func modelAuthorizationToken(header http.Header) (string, bool) {
	values := header.Values("Authorization")
	if len(values) > 1 || hasAlternateModelCredential(header) {
		return "", false
	}
	if len(values) == 0 {
		return "", true
	}
	token := values[0]
	if scheme, value, hasSpace := strings.Cut(token, " "); hasSpace && strings.EqualFold(scheme, "Bearer") {
		token = value
	}
	return token, validModelToken(header, token)
}

// hasAlternateModelCredential rejects ambiguous alternative model credentials.
func hasAlternateModelCredential(header http.Header) bool {
	if len(header.Values("X-Api-Key")) > 0 || len(header.Values("Api-Key")) > 0 {
		return true
	}
	for _, value := range header.Values("Sec-WebSocket-Protocol") {
		for protocol := range strings.SplitSeq(value, ",") {
			if strings.HasPrefix(strings.TrimSpace(protocol), "openai-insecure-api-key.") {
				return true
			}
		}
	}
	return false
}

// validModelToken permits token bytes unchanged and excludes account sessions.
func validModelToken(header http.Header, token string) bool {
	if len(token) == 0 || len(token) > 4096 || ssosession.HasLaiskyIssuer(token) ||
		strings.IndexFunc(token, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
				r == '-' || r == '_' || r == '.' || r == ':' || r == '|')
		}) >= 0 {
		return false
	}
	for _, value := range header.Values(ramjetSessionHeader) {
		session, ok := ssosession.BearerToken(value)
		if ok && subtle.ConstantTimeCompare([]byte(token), []byte(session)) == 1 {
			return false
		}
	}
	return true
}

// isCallerModelToken recognizes supported caller-owned API key families.
func isCallerModelToken(token string) bool {
	if len(token) < 15 || !(strings.HasPrefix(token, "sk-") || strings.HasPrefix(token, "laisky-")) {
		return false
	}
	if config.Config != nil {
		for _, entry := range config.Config.UserTokens {
			if entry != nil && entry.Token == config.FreetierUserToken &&
				subtle.ConstantTimeCompare([]byte(token), []byte(entry.OpenaiToken)) == 1 {
				return false
			}
		}
	}
	return true
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
