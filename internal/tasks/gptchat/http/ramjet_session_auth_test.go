package http

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// TestRamjetPrivateRoutePolicy enumerates the exact account/public boundary and
// excludes path aliases and unsupported methods before any backend dispatch.
func TestRamjetPrivateRoutePolicy(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		private      bool
	}{
		{"GET", "/gptchat/files", true}, {"POST", "/gptchat/files", true}, {"DELETE", "/gptchat/files", true},
		{"GET", "/gptchat/ctx/list", true}, {"GET", "/gptchat/ctx/chat", true}, {"GET", "/gptchat/ctx/search", true},
		{"POST", "/gptchat/ctx/build", true}, {"POST", "/gptchat/ctx/active", true}, {"POST", "/gptchat/ctx/share", true},
		{"GET", "/gptchat/ctx/share", false}, {"POST", "/gptchat/query/chunks", false}, {"POST", "/gptchat/chunks", false},
		{"POST", "/gptchat/image/dalle", false}, {"GET", "/gptchat/query/query", false}, {"GET", "/gptchat/query/search", false},
		{"GET", "/gptchat/encrypted-files/legacy/file.pdf", false},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "http://ramjet.fixture"+tc.path, nil)
			private, err := ramjetPrivateRoute(req)
			require.NoError(t, err)
			require.Equal(t, tc.private, private)
		})
	}
	for _, tc := range []struct{ method, path string }{
		{"PUT", "/gptchat/files"}, {"GET", "/gptchat/ctx/build"}, {"DELETE", "/gptchat/ctx/share"},
		{"GET", "/gptchat/ctx/unknown"}, {"GET", "/gptchat/files/"}, {"GET", "/gptchat/%66iles"},
		{"GET", "/gptchat/ctx/../files"}, {"GET", "/gptchat/encrypted-files/../private.store"},
	} {
		t.Run("denied/"+tc.method+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "http://ramjet.fixture"+tc.path, nil)
			_, err := ramjetPrivateRoute(req)
			require.Error(t, err)
		})
	}
}

// TestRamjetPrivateSessionsDenyBeforeBackend proves missing, inactive, substituted
// and unavailable account proof cannot reach list/delete/upload/build work.
func TestRamjetPrivateSessionsDenyBeforeBackend(t *testing.T) {
	for _, op := range []struct{ method, path string }{
		{"GET", "/gptchat/files"}, {"DELETE", "/gptchat/files"}, {"POST", "/gptchat/files"}, {"POST", "/gptchat/ctx/build"},
	} {
		for _, mode := range []string{"missing", "malformed", "duplicate", "inactive", "wrong_account", "unavailable"} {
			t.Run(op.method+op.path+"/"+mode, func(t *testing.T) {
				byokAuditSetup(t)
				config.Config.RamjetURL = "http://ramjet.fixture"
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequest(op.method, "/gptchat/ramjet"+op.path, strings.NewReader("{}"))
				ctx.Request.Header.Set("Authorization", "Bearer sk-SYNTHETIC-MODEL-ONLY")
				ctx.Request.Header.Set("X-Laisky-User-Id", ramjetTestAccountA)
				ctx.Request.Header.Set("X-Laisky-User-Is-Free", "false")
				token, authority := ramjetSessionTestSetup(t, ramjetTestAccountA)
				switch mode {
				case "missing":
				case "malformed":
					ctx.Request.Header.Set(ramjetSessionHeader, "Bearer malformed")
				case "duplicate":
					ctx.Request.Header.Add(ramjetSessionHeader, "Bearer "+token)
					ctx.Request.Header.Add(ramjetSessionHeader, "Bearer "+token)
				case "inactive":
					authority.inactive = true
					ctx.Request.Header.Set(ramjetSessionHeader, "Bearer "+token)
				case "wrong_account":
					authority.overrideUID = ramjetTestAccountB
					ctx.Request.Header.Set(ramjetSessionHeader, "Bearer "+token)
				case "unavailable":
					authority.unavailable = true
					ctx.Request.Header.Set(ramjetSessionHeader, "Bearer "+token)
				}
				calls := 0
				httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) { calls++; return byokAuditResponse(req, "{}"), nil })}
				RamjetProxyHandler(ctx)
				expected := http.StatusUnauthorized
				if mode == "unavailable" {
					expected = http.StatusServiceUnavailable
				}
				require.Equal(t, expected, recorder.Code, recorder.Body.String())
				require.Zero(t, calls)
				if mode == "missing" || mode == "malformed" || mode == "duplicate" {
					require.Zero(t, authority.calls)
				} else {
					require.Equal(t, 1, authority.calls)
				}
				require.NotContains(t, recorder.Body.String(), token)
			})
		}
	}
}

// TestRamjetOriginalSessionCannotBecomeModelCredential excludes both known SSO
// issuer tokens and exact equality between the account/model header values.
func TestRamjetOriginalSessionCannotBecomeModelCredential(t *testing.T) {
	for _, mode := range []string{"issuer", "equal_headers"} {
		t.Run(mode, func(t *testing.T) {
			byokAuditSetup(t)
			token, authority := ramjetSessionTestSetup(t, ramjetTestAccountA)
			key := token
			if mode == "equal_headers" {
				key = "sk-SYNTHETIC-OPAQUE-SESSION-MODEL"
			}
			ctx := newAuthContext(key)
			ctx.Request.Header.Set(ramjetSessionHeader, "Bearer "+key)
			ctx.Request.Method = http.MethodPost
			ctx.Request.URL.Path = "/gptchat/ramjet/gptchat/query/chunks"
			ctx.Request.Body = io.NopCloser(strings.NewReader("{}"))
			calls := 0
			httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) { calls++; return byokAuditResponse(req, "{}"), nil })}
			RamjetProxyHandler(ctx)
			require.GreaterOrEqual(t, ctx.Writer.Status(), http.StatusBadRequest)
			require.Zero(t, calls)
			require.Zero(t, authority.calls)
		})
	}
}

// TestRamjetPrivateRedirectCannotReplaySession refuses upstream redirects and
// removes redirect responses so browsers cannot replay the original account token.
func TestRamjetPrivateRedirectCannotReplaySession(t *testing.T) {
	for _, target := range []string{"https://foreign.fixture/private", "http://ramjet.fixture/other", "https://ramjet.fixture/model"} {
		t.Run(target, func(t *testing.T) {
			byokAuditSetup(t)
			config.Config.RamjetURL = "http://ramjet.fixture"
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/gptchat/ramjet/gptchat/files", nil)
			token, authority := ramjetSessionTestSetup(t, ramjetTestAccountA)
			ctx.Request.Header.Set(ramjetSessionHeader, "Bearer "+token)
			calls := 0
			original := &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "ramjet.fixture", req.URL.Host)
				require.Equal(t, "Bearer "+token, req.Header.Get(ramjetSessionHeader))
				return &http.Response{StatusCode: http.StatusTemporaryRedirect, Header: http.Header{"Location": []string{target}}, Body: io.NopCloser(strings.NewReader("do not reflect")), Request: req}, nil
			})}
			httpcli = original
			RamjetProxyHandler(ctx)
			require.Equal(t, http.StatusBadGateway, recorder.Code)
			require.Empty(t, recorder.Header().Get("Location"))
			require.Equal(t, 1, calls)
			require.Equal(t, 1, authority.calls)
			require.Same(t, original, httpcli)
			require.Nil(t, original.CheckRedirect)
			require.NotContains(t, recorder.Body.String(), token)
			require.NotContains(t, recorder.Body.String(), target)
		})
	}
}

// TestRamjetPublicShareKeepsOnlyPublicSelector retains the legacy named-share
// owner selector, while stripping session credentials and never consulting SSO.
func TestRamjetPublicShareKeepsOnlyPublicSelector(t *testing.T) {
	byokAuditSetup(t)
	config.Config.RamjetURL = "http://ramjet.fixture"
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/gptchat/ramjet/gptchat/ctx/share?uid=query-owner&chatbot_name=published&q=synthetic", nil)
	ctx.Request.Header.Set("Authorization", "Bearer sk-SYNTHETIC-PUBLIC-MODEL")
	ctx.Request.Header.Set("X-Laisky-User-Id", "legacy-public-owner")
	authority := requireRamjetSession(t, ctx.Request.Header, ramjetTestAccountA)
	calls := 0
	httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "query-owner", req.URL.Query().Get("uid"))
		require.Equal(t, "legacy-public-owner", req.Header.Get("X-Laisky-User-Id"))
		require.Empty(t, req.Header.Get(ramjetSessionHeader))
		return byokAuditResponse(req, "{}"), nil
	})}
	RamjetProxyHandler(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, calls)
	require.Zero(t, authority.calls)
}

// TestOneAPIProxyDoesNotForwardAccountSession preserves model credential handling
// without exposing account sessions to the independent OneAPI provider.
func TestOneAPIProxyDoesNotForwardAccountSession(t *testing.T) {
	byokAuditSetup(t)
	key := "sk-SYNTHETIC-ONEAPI-MODEL"
	ctx := newAuthContext(key)
	ctx.Request.URL.Path = "/gptchat/oneapi/v1/chat/completions"
	ctx.Request.Body = io.NopCloser(strings.NewReader("{}"))
	authority := requireRamjetSession(t, ctx.Request.Header, ramjetTestAccountA)
	incoming := ctx.Request.Header.Clone()
	calls := 0
	httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "oneapi.laisky.com", req.URL.Host)
		require.Equal(t, key, req.Header.Get("Authorization"))
		require.Empty(t, req.Header.Get(ramjetSessionHeader))
		return byokAuditResponse(req, "{}"), nil
	})}
	OneapiProxyHandler(ctx)
	require.Equal(t, http.StatusOK, ctx.Writer.Status())
	require.Equal(t, 1, calls)
	require.Zero(t, authority.calls)
	require.Equal(t, incoming, ctx.Request.Header)
}

// TestRamjetUnknownOperationNeverDispatches keeps unknown method/path aliases from
// falling back into public or private behavior.
func TestRamjetUnknownOperationNeverDispatches(t *testing.T) {
	for index, target := range []string{"/gptchat/ramjet/gptchat/ctx/unknown", "/gptchat/ramjet/gptchat/%66iles", "/gptchat/ramjet/gptchat/files/", "/gptchat/ramjet/gptchat/files%23ignored", "/gptchat/ramjet/gptchat/files%3Fignored"} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			byokAuditSetup(t)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
			ctx.Request.Header.Set("Authorization", "Bearer sk-SYNTHETIC-MODEL")
			authority := requireRamjetSession(t, ctx.Request.Header, ramjetTestAccountA)
			calls := 0
			httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) { calls++; return byokAuditResponse(req, "{}"), nil })}
			RamjetProxyHandler(ctx)
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Zero(t, calls)
			require.Zero(t, authority.calls)
		})
	}
}
