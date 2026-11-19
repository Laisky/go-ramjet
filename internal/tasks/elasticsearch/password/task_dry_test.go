package password

import (
	"fmt"
	"strings"
	"testing"

	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v6"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/log"
)

// setConfig sets key for the duration of t and restores its previous value.
func setConfig(t *testing.T, key string, value any) {
	t.Helper()
	previous := gconfig.Shared.Get(key)
	gconfig.Shared.Set(key, value)
	t.Cleanup(func() { gconfig.Shared.Set(key, previous) })
}

// TestRunTaskDryModeNeverLogsPassword guards against writing the generated
// Elasticsearch password to the logs during a dry run, which AGENTS.md forbids.
// The dry run must still report that it would change the password for each API.
func TestRunTaskDryModeNeverLogsPassword(t *testing.T) {
	const secret = "dry-run-test-secret"
	setConfig(t, "dry", true)
	setConfig(t, "tasks.elasticsearch-v2.password.secret", secret)
	setConfig(t, "tasks.elasticsearch-v2.password.username", "elastic")
	setConfig(t, "tasks.elasticsearch-v2.password.apis", []string{"https://user:pass@es.invalid/_security/user/elastic/_password"})

	core, entries := observer.New(zapcore.DebugLevel)
	logger, err := glog.New(glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
	require.NoError(t, err)
	old := log.Logger
	log.Logger = logger
	t.Cleanup(func() { log.Logger = old })

	password := GeneratePasswdByDate(gutils.UTCNow(), secret)
	runTask()

	require.NotEmpty(t, entries.FilterMessage("change password via post").All(), "dry run must still report the change")
	for _, entry := range entries.All() {
		require.NotContains(t, entry.Message, password)
		for key, value := range entry.ContextMap() {
			require.NotContains(t, strings.ToLower(key), "password", "entry %q must not carry a password field", entry.Message)
			require.NotContains(t, fmt.Sprint(value), password, "entry %q field %q", entry.Message, key)
		}
	}
}
