package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestSSOCallbackReferrerPolicy prevents callback credentials from reaching same-origin subresource referrers.
func TestSSOCallbackReferrerPolicy(t *testing.T) {
	for _, query := range []string{"sso_code=synthetic-canary", "sso_token=synthetic-canary", "%73so_code=synthetic-canary", "SSO_CODE=synthetic-canary", "sso_code=synthetic-canary&sso_token=synthetic-canary", "broken%=synthetic-canary"} {
		t.Run(query, func(t *testing.T) {
			router := gin.New()
			router.Use(securityMiddleware)
			router.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "callback") })
			response := httptest.NewRecorder()
			(&normalizeHandler{handler: router}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?"+query, nil))
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, "no-referrer", response.Header().Get("Referrer-Policy"))
		})
	}
}

// TestSSORefererRedaction protects diagnostics for requests from a previously loaded callback document.
func TestSSORefererRedaction(t *testing.T) {
	for _, referrer := range []string{"https://cv.laisky.com/?view=kept&sso_code=synthetic-canary", "https://chat.laisky.com/?view=kept&%73so_token=synthetic-canary", "https://chat.laisky.com/?view=kept&SSO_CODE=synthetic-canary", "https://chat.laisky.com/?view=kept&broken%=synthetic-canary"} {
		t.Run(referrer, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
			request.Header.Set("Referer", referrer)
			handler := &normalizeHandler{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NotContains(t, r.Referer(), "synthetic-canary")
				require.Contains(t, r.Referer(), "view=kept")
				w.WriteHeader(http.StatusOK)
			})}
			handler.ServeHTTP(httptest.NewRecorder(), request)
		})
	}
}

// TestSSOPrivacyPreservesOrdinaryRequests prevents privacy hardening from changing unrelated routes and referrers.
func TestSSOPrivacyPreservesOrdinaryRequests(t *testing.T) {
	const referrer = "https://cv.laisky.com/cv?view=one%20two&code=ordinary&token=ordinary"
	router := gin.New()
	router.Use(securityMiddleware)
	router.GET("/", func(c *gin.Context) {
		require.Equal(t, referrer, c.Request.Referer())
		require.Equal(t, "view=kept&code=ordinary", c.Request.URL.RawQuery)
		c.String(http.StatusOK, "ordinary")
	})
	request := httptest.NewRequest(http.MethodGet, "/?view=kept&code=ordinary", nil)
	request.Header.Set("Referer", referrer)
	response := httptest.NewRecorder()
	(&normalizeHandler{handler: router}).ServeHTTP(response, request)
	require.Equal(t, "strict-origin-when-cross-origin", response.Header().Get("Referrer-Policy"))
}

// TestSSORefererRedactsEveryHeaderValue preserves ordinary referrers while redacting duplicate credential-bearing values.
func TestSSORefererRedactsEveryHeaderValue(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	request.Header.Add("Referer", "https://cv.laisky.com/?sso_code=synthetic-canary")
	request.Header.Add("Referer", "https://cv.laisky.com/?sso_token=synthetic-canary")
	request.Header.Add("Referer", "https://cv.laisky.com/cv?view=one%20two")
	handler := &normalizeHandler{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := r.Header.Values("Referer")
		require.Len(t, values, 3)
		require.Equal(t, "https://cv.laisky.com/?sso_code=REDACTED", values[0])
		require.Equal(t, "https://cv.laisky.com/?sso_token=REDACTED", values[1])
		require.Equal(t, "https://cv.laisky.com/cv?view=one%20two", values[2])
		w.WriteHeader(http.StatusOK)
	})}
	handler.ServeHTTP(httptest.NewRecorder(), request)
}
