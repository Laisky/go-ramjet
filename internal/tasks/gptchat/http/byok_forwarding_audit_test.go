package http

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/log"
)

// byokAuditTransport handles synthetic outbound requests without opening sockets.
type byokAuditTransport func(*http.Request) (*http.Response, error)

// RoundTrip returns a synthetic response for the supplied request.
func (fn byokAuditTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

// byokAuditResponse returns an in-memory JSON response to a synthetic request.
func byokAuditResponse(req *http.Request, body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}
}

// byokAuditSetup isolates globals and rejects all unmocked HTTP traffic.
func byokAuditSetup(t *testing.T) {
	t.Helper()
	originalConfig, originalClient, originalLogger := config.Config, httpcli, log.Logger
	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		config.Config, httpcli, log.Logger = originalConfig, originalClient, originalLogger
		http.DefaultTransport = originalTransport
	})
	setupTestConfig()
	http.DefaultTransport = byokAuditTransport(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("unmocked HTTP request prohibited")
	})
	logger, _ := embeddingTestLogger(t)
	log.Logger = logger
}

// TestBYOKAuditProxyForwarding preserves the selected caller key and API base.
func TestBYOKAuditProxyForwarding(t *testing.T) {
	for _, tc := range []struct{ name, token, expectedToken, expectedBase string }{
		{"byok", "sk-SYNTHETIC-ONLY-KEY-0123456789", "sk-SYNTHETIC-ONLY-KEY-0123456789", "http://100.64.0.10:3000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			byokAuditSetup(t)
			config.Config.RamjetURL = "http://ramjet.test:22280"
			ctx := newAuthContext(tc.token)
			recorder := httptest.NewRecorder()
			proxyCtx, _ := gin.CreateTestContext(recorder)
			proxyCtx.Request = ctx.Request
			ctx = proxyCtx
			ctx.Request.URL.Path = "/gptchat/ramjet/gptchat/query/chunks"
			ctx.Request.Header.Set("X-Laisky-Api-Base", "http://100.64.0.10:3000")
			ctx.Request.Body = io.NopCloser(strings.NewReader("{}"))
			httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, "ramjet.test:22280", req.URL.Host)
				require.Equal(t, "/gptchat/query/chunks", req.URL.Path)
				require.Equal(t, tc.expectedToken, req.Header.Get("Authorization"))
				require.Equal(t, tc.expectedBase, req.Header.Get("X-Laisky-Openai-Api-Base"))
				return byokAuditResponse(req, "{}"), nil
			})}
			RamjetProxyHandler(ctx)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, tc.expectedToken, GetRawUserToken(ctx))
			require.Equal(t, tc.expectedToken[:15], ctx.Request.Header.Get("X-Laisky-User-Id"))
		})
	}
}

// TestBYOKAuditChunksForwarding preserves the current user key and selected API base in chunk queries.
func TestBYOKAuditChunksForwarding(t *testing.T) {
	byokAuditSetup(t)
	key := "sk-SYNTHETIC-ONLY-CHUNK-0123456789"
	ctx := newAuthContext(key)
	ctx.Request.Header.Set("X-Laisky-Api-Base", "http://100.64.0.10:3000")
	user, err := getUserByAuthHeader(ctx)
	require.NoError(t, err)
	httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, key, req.Header.Get("Authorization"))
		require.Equal(t, "http://100.64.0.10:3000", req.Header.Get("X-Laisky-Openai-Api-Base"))
		return byokAuditResponse(req, "{\"results\":\"synthetic-only\"}"), nil
	})}
	result, err := queryChunks(ctx, queryChunksArgs{user: user, query: "synthetic", ext: ".txt", model: "test-model", content: []byte("local content")})
	require.NoError(t, err)
	require.Equal(t, "synthetic-only", result)
}

// TestBYOKAuditKeyExcludedFromLogs requires both complete and partial caller credentials to be absent.
func TestBYOKAuditKeyExcludedFromLogs(t *testing.T) {
	byokAuditSetup(t)
	key := "sk-SYNTHETIC-ONLY-PRIVATE-KEY-0123456789"
	logger, entries := embeddingTestLogger(t)
	ctx := newAuthContext(key)
	gmw.SetLogger(ctx, logger)
	_, err := getUserByAuthHeader(ctx)
	require.NoError(t, err)
	logged := fmt.Sprint(entries.All())
	require.NotContains(t, logged, key)
	require.NotContains(t, logged, key[:15])
}

// TestBYOKAuditInvalidKeyExcludedFromErrors requires rejected caller credentials to be absent from errors.
func TestBYOKAuditInvalidKeyExcludedFromErrors(t *testing.T) {
	byokAuditSetup(t)
	for _, key := range []string{"sk-SYNTH", "laisky-SYNTH", "FREETIER-SYNTH"} {
		_, err := getUserByAuthHeader(newAuthContext(key))
		require.Error(t, err)
		require.NotContains(t, err.Error(), key)
	}
}

// TestBYOKAuditRedirectSemantics characterizes existing Go redirect handling without imposing destination policy.
func TestBYOKAuditRedirectSemantics(t *testing.T) {
	for _, target := range []string{"http://other.test/final", "http://child.ramjet.test/final", "https://ramjet.test/final"} {
		t.Run(target, func(t *testing.T) {
			client, err := gutils.NewHTTPClient()
			require.NoError(t, err)
			require.Nil(t, client.CheckRedirect)
			var seen []*http.Request
			client.Transport = byokAuditTransport(func(req *http.Request) (*http.Response, error) {
				seen = append(seen, req.Clone(req.Context()))
				if len(seen) == 1 {
					return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{target}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
				}
				return byokAuditResponse(req, "{}"), nil
			})
			req := httptest.NewRequest(http.MethodGet, "http://ramjet.test/start", nil)
			req.RequestURI = ""
			req.Header.Set("Authorization", "sk-SYNTHETIC-ONLY-REDIRECT-0123456789")
			req.Header.Set("X-Laisky-User-Id", "sk-SYNTHETIC-ONL")
			resp, err := client.Do(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Len(t, seen, 2)
			require.Equal(t, "sk-SYNTHETIC-ONL", seen[1].Header.Get("X-Laisky-User-Id"))
			if strings.Contains(target, "other.test") {
				require.Empty(t, seen[1].Header.Get("Authorization"))
			} else {
				require.Equal(t, req.Header.Get("Authorization"), seen[1].Header.Get("Authorization"))
			}
		})
	}
}
