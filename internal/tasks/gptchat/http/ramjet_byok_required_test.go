package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// TestRamjetBYOKRequiredBeforeProxyDispatch rejects requests that would previously spend server-owned credentials.
func TestRamjetBYOKRequiredBeforeProxyDispatch(t *testing.T) {
	for _, key := range []string{"", "FREETIER-SYNTHETIC-CLIENT-0123456789", config.FreetierUserToken, "sk-SHORT", "sk-SYNTHETIC WITH SPACE", " sk-SYNTHETIC-LEADING-SPACE", "sk-SYNTHETIC-TRAILING-SPACE "} {
		t.Run(key, func(t *testing.T) {
			byokAuditSetup(t)
			original := newAuthContext(key)
			if key == "" {
				original.Request.Header.Del("Authorization")
			}
			original.Request.URL.Path = "/gptchat/ramjet/gptchat/query/chunks"
			original.Request.Body = io.NopCloser(strings.NewReader("{}"))
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = original.Request
			calls := 0
			httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				return byokAuditResponse(req, "{}"), nil
			})}
			RamjetProxyHandler(ctx)
			require.GreaterOrEqual(t, recorder.Code, http.StatusBadRequest)
			require.Zero(t, calls)
		})
	}
}

// TestRamjetBYOKRequiredBeforeChunkDispatch prevents caller-triggered chunk queries from using a configured server key.
func TestRamjetBYOKRequiredBeforeChunkDispatch(t *testing.T) {
	byokAuditSetup(t)
	ctx := newAuthContext("")
	calls := 0
	httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		return byokAuditResponse(req, "{\"results\":\"not-allowed\"}"), nil
	})}
	_, err := queryChunks(ctx, queryChunksArgs{user: &config.UserConfig{OpenaiToken: "SERVER_OPENAI_TOKEN"}, query: "synthetic", ext: ".txt", model: "test-model", content: []byte("local")})
	require.Error(t, err)
	require.Zero(t, calls)
}

// TestRamjetBYOKCachedServerCredentialsRejected prevents cached account state from replacing the original caller key.
func TestRamjetBYOKCachedServerCredentialsRejected(t *testing.T) {
	for _, supplied := range []string{"", "sk-SYNTHETIC-ORIGINAL-CALLER-KEY"} {
		t.Run(supplied, func(t *testing.T) {
			byokAuditSetup(t)
			ctx := newAuthContext(supplied)
			ctx.Set(ctxKeyUser, &config.UserConfig{BYOK: true, Token: "sk-SYNTHETIC-ORIGINAL-CALLER-KEY", OpenaiToken: "SERVER_SYNTHETIC_SUBSTITUTE", UserName: "persisted-user-id"})
			_, err := resolveRamjetUser(ctx)
			require.Error(t, err)
		})
	}
}
