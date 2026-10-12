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

// TestRamjetModelResolutionDiscardsCachedNames keeps BYOK credentials separate
// from account/storage identity and leaves generic user state unchanged.
func TestRamjetModelResolutionDiscardsCachedNames(t *testing.T) {
	for index, key := range []string{"sk-SYNTHETIC-01-A", "sk-SYNTHETIC-01-B", "sk-SYNTHETIC-ROTATED-KEY"} {
		t.Run(fmt.Sprintf("key%d", index), func(t *testing.T) {
			byokAuditSetup(t)
			ctx := newAuthContext(key)
			ctx.Request.Header.Set("X-Laisky-User-Id", "forged-victim")
			ctx.Request.Header.Set("X-Laisky-User-Is-Free", "false")
			original := &config.UserConfig{BYOK: true, Token: key, OpenaiToken: key, ImageToken: key, UserName: "forged-cached-victim", APIBase: "http://provider.fixture"}
			ctx.Set(ctxKeyUser, original)
			resolved, err := resolveRamjetUser(ctx)
			require.NoError(t, err)
			require.Equal(t, "public", resolved.UserName)
			require.Equal(t, "forged-cached-victim", original.UserName)
			require.NotSame(t, original, resolved)
			require.Equal(t, key, resolved.Token)
			require.Equal(t, key, resolved.OpenaiToken)
			require.Equal(t, key, resolved.ImageToken)
			require.Equal(t, original.APIBase, resolved.APIBase)
			require.False(t, resolved.IsFree)
		})
	}
}

// TestRamjetAccountProxyHeaders binds private list/delete operations to the
// verified original account while preserving model routing and caller headers.
func TestRamjetAccountProxyHeaders(t *testing.T) {
	keys := []string{"sk-SYNTHETIC-01-A", "sk-SYNTHETIC-01-B", "sk-SYNTHETIC-ROTATED-KEY"}
	require.Equal(t, keys[0][:15], keys[1][:15])
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		for index, key := range keys {
			for _, freeHeader := range []string{"", "true", "false"} {
				t.Run(fmt.Sprintf("%s/key%d/free=%s", method, index, freeHeader), func(t *testing.T) {
					byokAuditSetup(t)
					config.Config.RamjetURL = "http://ramjet.fixture"
					recorder := httptest.NewRecorder()
					ctx, _ := gin.CreateTestContext(recorder)
					body := ""
					if method == http.MethodDelete {
						body = `{"datasets":["synthetic-dataset"]}`
					}
					ctx.Request = httptest.NewRequest(method, "/gptchat/ramjet/gptchat/files", strings.NewReader(body))
					ctx.Request.Header.Set("Authorization", "Bearer "+key)
					ctx.Request.Header.Set("X-Laisky-User-Id", "forged-victim")
					ctx.Request.Header.Set("X-Laisky-Api-Base", "http://provider.fixture/v1")
					if freeHeader != "" {
						ctx.Request.Header.Set("X-Laisky-User-Is-Free", freeHeader)
					}
					token, authority := ramjetSessionTestSetup(t, ramjetTestAccountA)
					ctx.Request.Header.Set(ramjetSessionHeader, "Bearer "+token)
					incoming := ctx.Request.Header.Clone()
					general, err := getUserByAuthHeader(ctx)
					require.NoError(t, err)
					calls := 0
					httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
						calls++
						require.Equal(t, "ramjet.fixture", req.URL.Host)
						require.Equal(t, "/gptchat/files", req.URL.Path)
						require.Equal(t, method, req.Method)
						require.Equal(t, key, req.Header.Get("Authorization"))
						require.Equal(t, ramjetTestAccountA, req.Header.Get("X-Laisky-User-Id"))
						require.Equal(t, "Bearer "+token, req.Header.Get(ramjetSessionHeader))
						require.Equal(t, "true", req.Header.Get("X-Laisky-User-Is-Free"))
						require.Equal(t, "http://provider.fixture/v1", req.Header.Get("X-Laisky-Openai-Api-Base"))
						payload, err := io.ReadAll(req.Body)
						require.NoError(t, err)
						require.Equal(t, body, string(payload))
						return byokAuditResponse(req, "{}"), nil
					})}
					RamjetProxyHandler(ctx)
					require.Equal(t, http.StatusOK, recorder.Code)
					require.Equal(t, 1, calls)
					require.Equal(t, 1, authority.calls)
					require.Equal(t, incoming, ctx.Request.Header)
					require.Equal(t, key[:15], general.UserName)
					require.False(t, general.IsFree)
					require.Equal(t, key, GetRawUserToken(ctx))
				})
			}
		}
	}
}

// TestRamjetAccountsDoNotFollowModelKeys proves model-key rotation preserves one
// account and sharing a model key cannot select another account's private scope.
func TestRamjetAccountsDoNotFollowModelKeys(t *testing.T) {
	cases := []struct{ key, account string }{
		{"sk-SYNTHETIC-01-A", ramjetTestAccountA},
		{"sk-SYNTHETIC-01-B", ramjetTestAccountA},
		{"sk-SYNTHETIC-01-A", ramjetTestAccountB},
	}
	seen := []string{}
	for index, tc := range cases {
		t.Run(fmt.Sprintf("case%d", index), func(t *testing.T) {
			byokAuditSetup(t)
			config.Config.RamjetURL = "http://ramjet.fixture"
			ctx := newAuthContext(tc.key)
			ctx.Request.Method = http.MethodGet
			ctx.Request.URL.Path = "/gptchat/ramjet/gptchat/files"
			ctx.Request.Body = io.NopCloser(strings.NewReader(""))
			requireRamjetSession(t, ctx.Request.Header, tc.account)
			httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
				seen = append(seen, req.Header.Get("X-Laisky-User-Id"))
				return byokAuditResponse(req, "{}"), nil
			})}
			RamjetProxyHandler(ctx)
			require.Equal(t, http.StatusOK, ctx.Writer.Status())
		})
	}
	require.Equal(t, seen[0], seen[1])
	require.NotEqual(t, seen[0], seen[2])
}

// TestRamjetAccountMetadataDoesNotRequireModels ensures view/delete/list never
// substitute a server-owned model key when only an account session was supplied.
func TestRamjetAccountMetadataDoesNotRequireModels(t *testing.T) {
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/gptchat/files"}, {http.MethodDelete, "/gptchat/files"}, {http.MethodGet, "/gptchat/ctx/list"},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			byokAuditSetup(t)
			config.Config.RamjetURL = "http://ramjet.fixture"
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(tc.method, "/gptchat/ramjet"+tc.path, strings.NewReader("{}"))
			token, authority := ramjetSessionTestSetup(t, ramjetTestAccountA)
			ctx.Request.Header.Set(ramjetSessionHeader, "Bearer "+token)
			calls := 0
			httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				require.Empty(t, req.Header.Get("Authorization"))
				require.Empty(t, req.Header.Get("X-Laisky-Openai-Api-Base"))
				require.Equal(t, "Bearer "+token, req.Header.Get(ramjetSessionHeader))
				require.Equal(t, ramjetTestAccountA, req.Header.Get("X-Laisky-User-Id"))
				return byokAuditResponse(req, "{}"), nil
			})}
			RamjetProxyHandler(ctx)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, 1, calls)
			require.Equal(t, 1, authority.calls)
			_, cached := ctx.Get(ctxKeyUser)
			require.False(t, cached, "metadata must not resolve the configured free-tier model user")
		})
	}
}

// TestRamjetPublicChunksDoNotForwardAccountSessions preserves generic chat
// identities and strips account credentials from the model-only chunk caller.
func TestRamjetPublicChunksDoNotForwardAccountSessions(t *testing.T) {
	for index, key := range []string{"sk-SYNTHETIC-01-A", "sk-SYNTHETIC-01-B", "sk-SYNTHETIC-ROTATED-KEY"} {
		t.Run(fmt.Sprintf("key%d", index), func(t *testing.T) {
			byokAuditSetup(t)
			config.Config.RamjetURL = "http://ramjet.fixture"
			ctx := newAuthContext(key)
			ctx.Request.Header.Set("X-Laisky-User-Id", "forged-victim")
			ctx.Request.Header.Set("X-Laisky-User-Is-Free", "false")
			ctx.Request.Header.Set("X-Laisky-Api-Base", "http://provider.fixture")
			authority := requireRamjetSession(t, ctx.Request.Header, ramjetTestAccountA)
			incoming := ctx.Request.Header.Clone()
			user, err := getUserByAuthHeader(ctx)
			require.NoError(t, err)
			calls := 0
			httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "ramjet.fixture", req.URL.Host)
				require.Equal(t, "/gptchat/query/chunks", req.URL.Path)
				require.Equal(t, key, req.Header.Get("Authorization"))
				require.Equal(t, "public", req.Header.Get("X-Laisky-User-Id"))
				require.Empty(t, req.Header.Get(ramjetSessionHeader))
				require.Equal(t, "true", req.Header.Get("X-Laisky-User-Is-Free"))
				require.Equal(t, "http://provider.fixture", req.Header.Get("X-Laisky-Openai-Api-Base"))
				return byokAuditResponse(req, `{"results":"synthetic-only"}`), nil
			})}
			result, err := queryChunks(ctx, queryChunksArgs{user: user, query: "synthetic", ext: ".txt", model: "test-model", content: []byte("local")})
			require.NoError(t, err)
			require.Equal(t, "synthetic-only", result)
			require.Equal(t, 1, calls)
			require.Zero(t, authority.calls)
			require.Equal(t, incoming, ctx.Request.Header)
			require.Equal(t, key[:15], user.UserName)
			require.False(t, user.IsFree)
		})
	}
}
