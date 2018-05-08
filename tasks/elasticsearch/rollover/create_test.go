package rollover_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/Laisky/go-ramjet/tasks/elasticsearch/rollover"
)

func TestGetIdxRolloverReqBodyByIdxAlias(t *testing.T) {
	var (
		jb    *bytes.Buffer
		err   error
		alias = "sit-geely-logs-alias"
	)
	jb, err = rollover.GetIdxRolloverReqBodyByIdxAlias(alias, "geely")
	if err != nil {
		t.Error(err.Error())
	}

	if !strings.Contains(jb.String(), alias) {
		t.Error(jb.String())
	}
}

func TestRolloverNewIndex(t *testing.T) {
	var (
		st = &rollover.IdxSetting{
			IdxWriteAlias: "sit-geely-logs-write",
			IdxAlias:      "sit-geely-logs-alias",
			Mapping:       "geely",
		}
		err error
	)

	viper.Set("dry", true)
	err = rollover.RolloverNewIndex(api, st)
	if err != nil {
		t.Error(err.Error())
	}
}
