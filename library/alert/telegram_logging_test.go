package alert

import (
	"bytes"
	"testing"

	gconfig "github.com/Laisky/go-config/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/log"
)

// TestTelegramSetupDoesNotLogCredentialURL verifies setup keeps useful diagnostics without echoing configured credentials.
func TestTelegramSetupDoesNotLogCredentialURL(t *testing.T) {
	previousURL := gconfig.Shared.GetString("telegram.api")
	gconfig.Shared.Set("telegram.api", "https://synthetic-user:private-password-canary@example.test/bot/private-path-canary?token=private-query-canary")
	t.Cleanup(func() { gconfig.Shared.Set("telegram.api", previousURL) })
	previousLogger := log.Logger
	var output bytes.Buffer
	logger, err := glog.NewWithName("go-ramjet", glog.LevelInfo, zap.WrapCore(func(zapcore.Core) zapcore.Core {
		return zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&output), zapcore.InfoLevel)
	}))
	require.NoError(t, err)
	log.Logger = logger
	t.Cleanup(func() { log.Logger = previousLogger })
	client := &TelegramCli{}
	client.Setup()
	require.NotNil(t, client.cli)
	require.Contains(t, output.String(), "configured telegram alert client")
	require.NotContains(t, output.String(), "canary")
	require.NotContains(t, output.String(), "example.test")
	require.NotContains(t, output.String(), `"url"`)
}
