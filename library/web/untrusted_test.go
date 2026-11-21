package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// serveRelayed answers one request through the default security middleware with
// body relayed as untrusted content of contentType.
func serveRelayed(t *testing.T, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(securityMiddleware)
	r.GET("/relay", func(c *gin.Context) {
		MarkUntrustedContent(c, contentType)
		c.Data(http.StatusOK, contentType, []byte(body))
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/relay", nil))
	require.Equal(t, http.StatusOK, w.Code)
	return w
}

// TestMarkUntrustedContentSandboxesActiveTypes keeps relayed active content in an
// opaque, script-free origin even behind the default security middleware, whose
// framing policy it retains. Unknown or malformed types are treated as active.
func TestMarkUntrustedContentSandboxesActiveTypes(t *testing.T) {
	require.Contains(t, UntrustedContentPolicy, "sandbox")
	require.NotContains(t, UntrustedContentPolicy, "allow-same-origin")
	require.NotContains(t, UntrustedContentPolicy, "allow-scripts")
	require.Contains(t, UntrustedContentPolicy, "frame-ancestors 'self'")

	for _, contentType := range []string{
		"text/html; charset=utf-8", "image/svg+xml", "application/xhtml+xml", "text/xml",
		"text/plain; charset=utf-8", "application/octet-stream", "", "not a media type",
	} {
		w := serveRelayed(t, contentType, "<script>alert(1)</script>")
		require.Equal(t, []string{UntrustedContentPolicy}, w.Header().Values("Content-Security-Policy"), contentType)
		require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"), contentType)
	}
}

// TestMarkUntrustedContentKeepsPDFViewable leaves PDFs unsandboxed, because the
// browser's built-in viewer refuses sandboxed documents and runs outside this
// origin anyway, while keeping the framing policy and disabling sniffing.
func TestMarkUntrustedContentKeepsPDFViewable(t *testing.T) {
	for _, contentType := range []string{"application/pdf", "Application/PDF; name=x.pdf"} {
		w := serveRelayed(t, contentType, "%PDF-1.7\n")
		require.Equal(t, []string{PassiveContentPolicy}, w.Header().Values("Content-Security-Policy"), contentType)
		require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"), contentType)
	}
}

// TestContentPoliciesKeepTheirWireValues pins the exact header values of the
// relayed-content policies, which share sameOriginFramingPolicy with the
// default policy, so building them from one directive changes no response.
func TestContentPoliciesKeepTheirWireValues(t *testing.T) {
	require.Equal(t, "frame-ancestors 'self'", sameOriginFramingPolicy)
	require.Equal(t, "sandbox allow-popups allow-popups-to-escape-sandbox; frame-ancestors 'self'", UntrustedContentPolicy)
	require.Equal(t, "frame-ancestors 'self'", PassiveContentPolicy)
}
