package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/stretchr/testify/require"
)

// TestNormalizeHandlerSSOCredentials verifies the outer HTTP boundary hides callback credentials before middleware.
func TestNormalizeHandlerSSOCredentials(t *testing.T) {
	for _, query := range []string{"sso_code=credential-canary", "sso_token=credential-canary", "SSO_CODE=credential-canary&sso_code=credential-canary", "%73so_code=credential-canary", "sso_code=credential-canary%ZZ", "broken%=credential-canary"} {
		t.Run(query, func(t *testing.T) {
			handler := &normalizeHandler{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NotContains(t, r.URL.String(), "credential-canary")
				require.NotContains(t, r.RequestURI, "credential-canary")
				require.Equal(t, "kept", r.URL.Query().Get("view"))
				w.WriteHeader(http.StatusOK)
			})}
			req := httptest.NewRequest(http.MethodGet, "/?view=kept&"+query, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			require.Equal(t, http.StatusOK, response.Code)
		})
	}
}

// TestNormalizeHandlerPreservesOrdinaryQuery verifies unrelated request behavior is unchanged.
func TestNormalizeHandlerPreservesOrdinaryQuery(t *testing.T) {
	const query = "view=one%20two&view=three&code=ordinary&token=ordinary&filter=a%3Bb"
	handler := &normalizeHandler{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, query, r.URL.RawQuery)
		require.Equal(t, "/?"+query, r.RequestURI)
		w.WriteHeader(http.StatusNoContent)
	})}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?"+query, nil))
	require.Equal(t, http.StatusNoContent, response.Code)
}

// TestSSOCanonicalCallbackDoesNotRedirect preserves the exact registered SPA callback root.
func TestSSOCanonicalCallbackDoesNotRedirect(t *testing.T) {
	router := gin.New()
	router.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "SPA bootstrap") })
	handler := &normalizeHandler{handler: router}
	for _, host := range []string{"cv.laisky.com", "chat.laisky.com", "chat2.laisky.com"} {
		t.Run(host, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://"+host+"/?sso_code=credential-canary&state=nonce", nil))
			require.Equal(t, http.StatusOK, response.Code)
			require.Empty(t, response.Header().Get("Location"))
			require.Equal(t, "SPA bootstrap", response.Body.String())
		})
	}
}
