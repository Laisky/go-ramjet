package http

import (
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/ssosession"
)

const ramjetSessionHeader = "X-Laisky-SSO-Token"

var ramjetSessionVerifier ssosession.SessionVerifier = ssosession.NewDefaultStrictVerifier()

// ramjetPrivateRoute classifies only canonical recognized operations. It returns
// whether original SSO account authentication is required before backend dispatch.
func ramjetPrivateRoute(req *http.Request) (bool, error) {
	if req == nil || req.URL == nil || req.URL.RawPath != "" {
		return false, errors.New("A canonical Ramjet operation is required")
	}
	switch req.URL.Path {
	case "/gptchat/files":
		switch req.Method {
		case http.MethodGet, http.MethodPost, http.MethodDelete:
			return true, nil
		}
	case "/gptchat/ctx/list", "/gptchat/ctx/chat", "/gptchat/ctx/search":
		if req.Method == http.MethodGet {
			return true, nil
		}
	case "/gptchat/ctx/build", "/gptchat/ctx/active":
		if req.Method == http.MethodPost {
			return true, nil
		}
	case "/gptchat/ctx/share":
		switch req.Method {
		case http.MethodPost:
			return true, nil
		case http.MethodGet:
			return false, nil
		}
	case "/gptchat/query/chunks", "/gptchat/chunks", "/gptchat/image/dalle":
		if req.Method == http.MethodPost {
			return false, nil
		}
	case "/gptchat/query/query", "/gptchat/query/search", "/gptchat":
		if req.Method == http.MethodGet {
			return false, nil
		}
	default:
		// Existing encrypted references use their own password capability.
		if req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/gptchat/encrypted-files/") &&
			!strings.Contains(req.URL.Path, "..") {
			return false, nil
		}
	}
	return false, errors.New("A recognized Ramjet operation and method are required")
}

// ramjetMetadataOnly identifies private operations that never need model keys.
func ramjetMetadataOnly(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	return req.URL.Path == "/gptchat/files" && (req.Method == http.MethodGet || req.Method == http.MethodDelete) ||
		req.URL.Path == "/gptchat/ctx/list" && req.Method == http.MethodGet
}

// originalRamjetSession extracts the one bounded original bearer session. It
// does not accept cookies, API keys, forwarded UID assertions or minted tokens.
func originalRamjetSession(header http.Header) (string, error) {
	values := header.Values(ramjetSessionHeader)
	if len(values) != 1 {
		return "", errors.Wrap(ssosession.ErrSessionInvalid, "an original SSO session is required")
	}
	token, ok := ssosession.BearerToken(values[0])
	if !ok || len(token) > ssosession.MaxTokenLength {
		return "", errors.Wrap(ssosession.ErrSessionInvalid, "an original SSO bearer session is required")
	}
	return token, nil
}

// setRamjetAccountAuth strips caller claims and forwards the verified original
// session only to recognized private Ramjet operations. Ramjet verifies it again.
func setRamjetAccountAuth(ctx *gin.Context, req *http.Request) (bool, error) {
	private, err := ramjetPrivateRoute(req)
	if err != nil {
		return false, errors.Wrap(err, "classify Ramjet operation")
	}
	req.Header.Del(ramjetSessionHeader)
	if !private {
		req.Header.Set("X-Laisky-User-Id", "public")
		if req.Method == http.MethodGet && req.URL.Path == "/gptchat/ctx/share" {
			// A legacy owner selector chooses only named published plaintext.
			if selector := ctx.GetHeader("X-Laisky-User-Id"); selector != "" {
				req.Header.Set("X-Laisky-User-Id", selector)
			}
		}
		return false, nil
	}
	token, err := originalRamjetSession(ctx.Request.Header)
	if err != nil {
		return false, errors.Wrap(err, "read original account session")
	}
	if ramjetSessionVerifier == nil {
		return false, errors.Wrap(ssosession.ErrUnavailable, "account session authority is unavailable")
	}
	identity, err := ramjetSessionVerifier.Verify(gmw.Ctx(ctx), token)
	if err != nil {
		return false, errors.Wrap(err, "verify original account session")
	}
	if identity == nil {
		return false, errors.Wrap(ssosession.ErrSessionInvalid, "SSO account identity is missing")
	}
	req.Header.Set(ramjetSessionHeader, "Bearer "+token)
	// This is informational only; the Python service independently resolves it.
	req.Header.Set("X-Laisky-User-Id", identity.UID)
	return true, nil
}

// abortRamjetAccountAuth maps strict account failures without exposing tokens,
// account payloads, model endpoints or transport errors to a caller.
func abortRamjetAccountAuth(ctx *gin.Context, err error) bool {
	if err != nil {
		switch {
		case errors.Is(err, ssosession.ErrUnavailable):
			ctx.Header("Retry-After", "30")
			ctx.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "SSO is temporarily unavailable"})
		case errors.Is(err, ssosession.ErrSessionInvalid):
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Sign in with Laisky SSO to use private datasets"})
		default:
			ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "A recognized Ramjet operation is required"})
		}
		return true
	}
	return false
}

// privateRamjetHTTPClient preserves the established client except that original
// account credentials must never follow a backend redirect.
func privateRamjetHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		return &http.Client{CheckRedirect: refuseRamjetRedirect}
	}
	isolated := *client
	isolated.CheckRedirect = refuseRamjetRedirect
	return &isolated
}

// refuseRamjetRedirect keeps the original session at the selected Ramjet service.
func refuseRamjetRedirect(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}
