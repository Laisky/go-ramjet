package http

import (
	"fmt"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/log"
)

// byokAuthAuditContext creates an isolated request with synthetic configuration and an observed logger.
func byokAuthAuditContext(t *testing.T, key string) (*gin.Context, *observer.ObservedLogs) {
	t.Helper()
	originalConfig, originalLogger := config.Config, log.Logger
	t.Cleanup(func() { config.Config, log.Logger = originalConfig, originalLogger })
	config.Config = &config.OpenAI{
		Token: "SERVER_SYNTHETIC_ONLY_KEY", DefaultImageToken: "SERVER_SYNTHETIC_IMAGE_KEY",
		API: "https://default.test", DefaultImageUrl: "https://default.test/v1/images/generations",
		UserTokens: []*config.UserConfig{{Token: config.FreetierUserToken, UserName: "public", APIBase: "https://free.test"}},
	}
	core, entries := observer.New(zap.DebugLevel)
	logger, err := glog.NewWithName("byok-audit", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	log.Logger = logger
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/", nil)
	ctx.Request.Header.Set("Authorization", "Bearer "+key)
	gmw.SetLogger(ctx, logger)
	return ctx, entries
}

// TestBYOKAuthAuditIdentityAndCredentialsPreserved keeps persisted BYOK identity and selected credentials unchanged.
func TestBYOKAuthAuditIdentityAndCredentialsPreserved(t *testing.T) {
	key := "sk-SYNTHETIC-ONLY-PRIVATE-KEY-0123456789"
	ctx, _ := byokAuthAuditContext(t, key)
	ctx.Request.Header.Set("X-Laisky-Api-Base", "http://100.64.0.10:3000")
	user, err := getUserByAuthHeader(ctx)
	require.NoError(t, err)
	require.Equal(t, key, user.OpenaiToken)
	require.Equal(t, key, user.ImageToken)
	require.Equal(t, key[:15], user.UserName)
	require.Equal(t, "http://100.64.0.10:3000", user.APIBase)
	require.Equal(t, key, GetRawUserToken(ctx))
}

// TestBYOKAuthAuditKeyExcludedFromLogs rejects complete or partial credentials in authentication logs.
func TestBYOKAuthAuditKeyExcludedFromLogs(t *testing.T) {
	key := "sk-SYNTHETIC-ONLY-PRIVATE-KEY-0123456789"
	ctx, entries := byokAuthAuditContext(t, key)
	_, err := getUserByAuthHeader(ctx)
	require.NoError(t, err)
	logged := fmt.Sprint(entries.All())
	require.NotContains(t, logged, key)
	require.NotContains(t, logged, key[:15])
}

// TestBYOKAuthAuditInvalidKeyExcludedFromErrors requires rejected synthetic credentials to be absent from errors.
func TestBYOKAuthAuditInvalidKeyExcludedFromErrors(t *testing.T) {
	for _, key := range []string{"sk-SYNTH", "laisky-SYNTH", "FREETIER-SYNTH"} {
		t.Run(key, func(t *testing.T) {
			ctx, _ := byokAuthAuditContext(t, key)
			_, err := getUserByAuthHeader(ctx)
			require.Error(t, err)
			require.NotContains(t, err.Error(), key)
		})
	}
}

// TestBYOKAuthAuditInvalidURLCredentialsExcludedFromLogs preserves ignored invalid overrides without logging embedded secrets.
func TestBYOKAuthAuditInvalidURLCredentialsExcludedFromLogs(t *testing.T) {
	for _, base := range []string{
		"http://SYNTHETIC_URL_USER:SYNTHETIC_URL_PASS@provider.test",
		"https://provider.test?api_key=SYNTHETIC_QUERY_KEY",
		"https://provider.test/%SYNTHETIC_PARSE_KEY",
	} {
		t.Run(base, func(t *testing.T) {
			ctx, entries := byokAuthAuditContext(t, "sk-SYNTHETIC-ONLY-PRIVATE-KEY")
			ctx.Request.Header.Set("X-Laisky-Api-Base", base)
			user, err := getUserByAuthHeader(ctx)
			require.NoError(t, err)
			require.Equal(t, "https://oneapi.laisky.com", user.APIBase)
			require.NotContains(t, fmt.Sprint(entries.All()), base)
		})
	}
}

// TestBYOKAuthAuditFreeTierCredentialExcludedFromLogs preserves the existing free-tier identity and server-owned defaults.
func TestBYOKAuthAuditFreeTierCredentialExcludedFromLogs(t *testing.T) {
	key := "FREETIER-SYNTHETIC-PRIVATE-CLIENT-0123456789"
	ctx, entries := byokAuthAuditContext(t, key)
	ctx.Request.Header.Set("X-Laisky-Api-Base", "http://ignored.test")
	user, err := getUserByAuthHeader(ctx)
	require.NoError(t, err)
	require.Equal(t, key[:15], user.UserName)
	require.Equal(t, "SERVER_SYNTHETIC_ONLY_KEY", user.OpenaiToken)
	require.Equal(t, "https://free.test", user.APIBase)
	require.NotContains(t, fmt.Sprint(entries.All()), key)
}
