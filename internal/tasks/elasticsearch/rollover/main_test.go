package rollover_test

import (
	"testing"

	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/elasticsearch/rollover"
	"github.com/Laisky/go-ramjet/library/log"
)

var (
	api    string
	idxSts []*rollover.IdxSetting
)

func init() {
	setUp()
}

func setUp() {
	if err := gconfig.Shared.LoadFromFile("/etc/go-ramjet/settings.yml"); err != nil {
		log.Logger.Panic("setup settings", zap.Error(err))
	}

	// api = gconfig.Shared.GetString("tasks.elasticsearch-v2.url")
	// gconfig.Shared.Set("debug", true)
	idxSts = rollover.LoadSettings()
	api = idxSts[0].API
}

func TestLoadSettings(t *testing.T) {
	st := rollover.LoadSettings()
	for _, v := range st {
		t.Logf("%+v", v)
	}
}

func TestLoadAllIndicesNames(t *testing.T) {
	indicies, err := rollover.LoadAllIndicesNames(api)
	if err != nil {
		t.Error(err.Error())
	}
	for _, idx := range indicies {
		t.Log(idx)
	}
}
