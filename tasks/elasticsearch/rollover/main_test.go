package rollover_test

import (
	"testing"

	"github.com/Laisky/go-utils"
	"github.com/Laisky/go-ramjet/tasks/elasticsearch/rollover"
)

var (
	api    string
	idxSts []*rollover.IdxSetting
)

func init() {
	setUp()
}

func setUp() {
	utils.Settings.Setup("/etc/go-ramjet/settings")

	// api = utils.Settings.GetString("tasks.elasticsearch-v2.url")
	// utils.Settings.Set("debug", true)
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
