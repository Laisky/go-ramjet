package rollover_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Laisky/go-ramjet/internal/tasks/elasticsearch/rollover"

	"github.com/Laisky/go-utils/v2"
)

func TestGetIdxRolloverReqBodyByIdxAlias(t *testing.T) {
	var (
		jb  *bytes.Buffer
		err error
		st  = &rollover.IdxSetting{
			IdxAlias: "sit-geely-logs-alias",
		}
	)
	jb, err = rollover.GetIdxRolloverReqBodyByIdxAlias(st)
	if err != nil {
		t.Error(err.Error())
	}

	if !strings.Contains(jb.String(), st.IdxAlias) {
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

	utils.Settings.Set("dry", true)
	err = rollover.NewIndex(api, st)
	if err != nil {
		t.Error(err.Error())
	}
}
