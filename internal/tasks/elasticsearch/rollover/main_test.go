package rollover_test

import (
	"os"
	"testing"

	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/testify/require"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/elasticsearch/rollover"
	"github.com/Laisky/go-ramjet/library/log"
)

func setUp(t testing.TB) (api string, idxSts []*rollover.IdxSetting) {
	t.Helper()
	cfg := os.Getenv("GO_RAMJET_SETTINGS")
	if cfg == "" {
		t.Skip("integration test disabled: set GO_RAMJET_SETTINGS to run")
	}
	if err := gconfig.Shared.LoadFromFile(cfg); err != nil {
		log.Logger.Panic("setup settings", zap.Error(err))
	}

	// api = gconfig.Shared.GetString("tasks.elasticsearch-v2.url")
	// gconfig.Shared.Set("debug", true)
	idxSts, err := rollover.LoadSettings()
	require.NoError(t, err)
	api = idxSts[0].API
	return api, idxSts
}

func TestLoadSettings(t *testing.T) {
	setUp(t)
	st, err := rollover.LoadSettings()
	require.NoError(t, err)
	for _, v := range st {
		t.Logf("%+v", v)
	}
}

func TestLoadAllIndicesNames(t *testing.T) {
	api, _ := setUp(t)
	indicies, err := rollover.LoadAllIndicesNames(api)
	require.NoError(t, err)
	for _, idx := range indicies {
		t.Log(idx)
	}
}
