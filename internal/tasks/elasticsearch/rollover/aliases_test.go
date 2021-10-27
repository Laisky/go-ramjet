package rollover_test

import (
	"regexp"
	"testing"

	"github.com/Laisky/go-ramjet/internal/tasks/elasticsearch/rollover"
)

func TestGetAliasURL(t *testing.T) {
	var (
		st = &rollover.IdxSetting{
			IdxWriteAlias: "sit-cp-logs-write",
			IdxAlias:      "sit-cp-logs-alias",
			Mapping:       "cp",
			API:           "http://readonly:readonly@172.16.4.160:8200/",
		}
		url = "http://readonly:readonly@172.16.4.160:8200/_cat/aliases/" + st.IdxWriteAlias + "/?v&format=json"
	)

	got := rollover.GetAliasURL(st)
	if got != url {
		t.Errorf("expect %v, got %v", url, got)
	}
}

func TestLoadAliases(t *testing.T) {
	var (
		st = &rollover.IdxSetting{
			IdxWriteAlias: "sit-cp-logs-write",
			IdxAlias:      "sit-cp-logs-alias",
			Mapping:       "cp",
		}
		url     = "http://readonly:readonly@172.16.4.160:8200/_cat/aliases/" + st.IdxWriteAlias + "/?v&format=json"
		err     error
		ret     []*rollover.AliasesResp
		matched bool
	)

	ret, err = rollover.LoadAliases(url)
	if err != nil {
		t.Errorf("got error %+v", err)
	}
	matched, err = regexp.MatchString("sit-cp-logs-\\d{4}\\.\\d{2}\\.\\d{2}-\\d+", ret[0].Index)
	if err != nil {
		t.Errorf("got error %+v", err)
	}
	if !matched {
		t.Errorf("aliase not matched for idx %v", ret[0].Index)
	}
}

func TestIsIdxIsWriteAlias(t *testing.T) {
	var (
		idx      = "sit-cp-logs-2018.04.01-0000002"
		aliases1 = []*rollover.AliasesResp{
			{
				Index: "sit-cp-logs-2018.04.01-0000002",
			},
		}
		aliases2 = []*rollover.AliasesResp{
			{
				Index: "sit-cp-logs-2018.04.01-0000001",
			},
		}
		aliases3 = []*rollover.AliasesResp{
			{
				Index: "sit-cp-logs-2018.04.01-0000001",
			},
			{
				Index: "sit-cp-logs-2018.04.01-0000002",
			},
		}
	)

	if rollover.IsIdxIsWriteAlias(idx, aliases1) != true {
		t.Errorf("test case fail for idx %v", idx)
	}
	if rollover.IsIdxIsWriteAlias(idx, aliases2) != false {
		t.Errorf("test case fail for idx %v", idx)
	}
	if rollover.IsIdxIsWriteAlias(idx, aliases3) != true {
		t.Errorf("test case fail for idx %v", idx)
	}
}
