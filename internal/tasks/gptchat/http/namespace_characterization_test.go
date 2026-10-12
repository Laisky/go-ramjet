//go:build namespace_characterization

package http

// These opt-in tests record a known ownership gap. They are not security
// acceptance criteria and must not be used as an ownership policy contract.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// namespaceQuotaHook records allowed quota commands without permitting a dial.
type namespaceQuotaHook struct {
	freeUserQuotaHook
	commands []string
	keys     []string
	forbid   bool
}

// ProcessHook captures scope selection and delegates only to the strict local fake.
func (h *namespaceQuotaHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	base := h.freeUserQuotaHook.ProcessHook(next)
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.commands = append(h.commands, cmd.Name())
		if h.forbid {
			return errors.New("unexpected token quota operation for dataset characterization")
		}
		args := cmd.Args()
		if len(args) < 2 {
			return errors.New("missing synthetic quota key")
		}
		h.keys = append(h.keys, fmt.Sprint(args[1]))
		return base(ctx, cmd)
	}
}

// namespaceQuotaSetup installs a strict synthetic quota client and restores globals.
func namespaceQuotaSetup(t *testing.T, forbid bool) *namespaceQuotaHook {
	t.Helper()
	setupFreeUserTestQuota(t)
	hook := &namespaceQuotaHook{forbid: forbid}
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	client.AddHook(hook)
	tokenQuotaMgr.client = client
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return hook
}

// namespaceDenyHTTP rejects and counts any HTTP before an explicit local fixture.
func namespaceDenyHTTP() *int {
	calls := new(int)
	httpcli = &http.Client{Transport: byokAuditTransport(func(_ *http.Request) (*http.Response, error) {
		*calls++
		return nil, errors.New("unexpected HTTP in namespace characterization")
	})}
	return calls
}

// TestNamespaceCharacterizationDatasetScope records prefix collisions and header behavior through the actual proxy.
func TestNamespaceCharacterizationDatasetScope(t *testing.T) {
	keys := []string{"sk-SYNTHETIC-01-A", "sk-SYNTHETIC-01-B", "sk-SYNTHETIC-02-C"}
	require.NotEqual(t, keys[0], keys[1])
	require.Equal(t, keys[0][:15], keys[1][:15])
	require.NotEqual(t, keys[0][:15], keys[2][:15])
	scopes := make(map[string]string)
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		for keyIndex, key := range keys {
			for _, freeHeader := range []string{"", "true", "false"} {
				t.Run(fmt.Sprintf("%s/key%d/isFree=%s", method, keyIndex, freeHeader), func(t *testing.T) {
					byokAuditSetup(t)
					unexpectedHTTP := namespaceDenyHTTP()
					hook := namespaceQuotaSetup(t, true)
					config.Config.RamjetURL = "http://ramjet.fixture"
					recorder := httptest.NewRecorder()
					ctx, _ := gin.CreateTestContext(recorder)
					body := ""
					if method == http.MethodDelete {
						body = `{"datasets":["synthetic-dataset"]}`
					}
					ctx.Request = httptest.NewRequest(method, "/gptchat/ramjet/gptchat/files", strings.NewReader(body))
					ctx.Request.Header.Set("Authorization", "Bearer "+key)
					ctx.Request.Header.Set("X-Laisky-User-Id", "forged-other-namespace")
					if freeHeader != "" {
						ctx.Request.Header.Set("X-Laisky-User-Is-Free", freeHeader)
					}
					user, err := getUserByAuthHeader(ctx)
					require.NoError(t, err)
					require.True(t, user.BYOK)
					require.False(t, user.IsFree, "incoming header is not Go token-quota authority")
					require.Equal(t, key[:15], user.UserName)
					reservation, err := ReserveTokens(ctx, user, &FrontendReq{MaxTokens: 7})
					require.NoError(t, err)
					require.Nil(t, reservation)
					require.Zero(t, *unexpectedHTTP, "authentication/reservation must make no HTTP call")
					calls := 0
					httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
						calls++
						require.Equal(t, "ramjet.fixture", req.URL.Host)
						require.Equal(t, "/gptchat/files", req.URL.Path)
						require.Equal(t, method, req.Method)
						require.Equal(t, key, req.Header.Get("Authorization"))
						require.Equal(t, key[:15], req.Header.Get("X-Laisky-User-Id"))
						require.NotEqual(t, "forged-other-namespace", req.Header.Get("X-Laisky-User-Id"))
						require.Equal(t, freeHeader, req.Header.Get("X-Laisky-User-Is-Free"))
						payload, readErr := io.ReadAll(req.Body)
						require.NoError(t, readErr)
						require.Equal(t, body, string(payload))
						scopes[fmt.Sprintf("%s/%d/%s", method, keyIndex, freeHeader)] = req.Header.Get("X-Laisky-User-Id")
						return byokAuditResponse(req, "{}"), nil
					})}
					RamjetProxyHandler(ctx)
					require.Equal(t, http.StatusOK, recorder.Code)
					require.Equal(t, 1, calls)
					require.Empty(t, hook.commands, "dataset forwarding made no token-quota Redis calls")
					require.Equal(t, key, GetRawUserToken(ctx))
				})
			}
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		for _, freeHeader := range []string{"", "true", "false"} {
			require.Equal(t, scopes[fmt.Sprintf("%s/0/%s", method, freeHeader)], scopes[fmt.Sprintf("%s/1/%s", method, freeHeader)])
			require.NotEqual(t, scopes[fmt.Sprintf("%s/0/%s", method, freeHeader)], scopes[fmt.Sprintf("%s/2/%s", method, freeHeader)])
		}
	}
}

// TestNamespaceCharacterizationFreeChatQuotaScope records the actual reservation scope independently of dataset forwarding.
func TestNamespaceCharacterizationFreeChatQuotaScope(t *testing.T) {
	keys := []string{"FREETIER-000001-A", "FREETIER-000001-B", "FREETIER-000002-C"}
	require.NotEqual(t, keys[0], keys[1])
	require.Equal(t, keys[0][:15], keys[1][:15])
	require.NotEqual(t, keys[0][:15], keys[2][:15])
	scopes := make([]string, len(keys))
	for index, key := range keys {
		t.Run(fmt.Sprintf("key%d", index), func(t *testing.T) {
			byokAuditSetup(t)
			unexpectedHTTP := namespaceDenyHTTP()
			hook := namespaceQuotaSetup(t, false)
			ctx := newAuthContext(key)
			ctx.Request.Header.Set("X-Laisky-User-Id", "forged-other-namespace")
			ctx.Request.Header.Set("X-Laisky-User-Is-Free", "false")
			user, err := getUserByAuthHeader(ctx)
			require.NoError(t, err)
			require.True(t, user.IsFree, "incoming false header does not disable Go free-tier quota")
			require.False(t, user.BYOK)
			require.Equal(t, key[:15], user.UserName)
			reservation, err := ReserveTokens(ctx, user, &FrontendReq{MaxTokens: 7})
			require.NoError(t, err)
			require.NotNil(t, reservation)
			require.Zero(t, *unexpectedHTTP, "free reservation must make no HTTP call")
			expected := "ramjet:gptchat:quota:" + key[:15]
			require.Equal(t, expected, reservation.key)
			require.Equal(t, 7, reservation.reservedTotal)
			require.Equal(t, []string{"hmget", "hincrby", "expire"}, hook.commands)
			require.Equal(t, []string{expected, expected, expected}, hook.keys)
			require.Equal(t, int32(1), hook.reads.Load())
			scopes[index] = reservation.key
		})
	}
	require.Equal(t, scopes[0], scopes[1])
	require.NotEqual(t, scopes[0], scopes[2])
}
