package rollover_test

import (
	"context"
	"regexp"
	"testing"
	"time"

	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/elasticsearch/rollover"
	"github.com/Laisky/go-ramjet/library/log"
)

func TestRemoveIndexByName(t *testing.T) {
	ctx := context.Background()
	api, _ := setUp(t)
	err := rollover.RemoveIndexByName(ctx, api, "test-rollover")
	if err != nil {
		t.Errorf("got error %+v", err)
	}
}

func TestFilterToBeDeleteIndicies(t *testing.T) {
	var (
		allInd = []string{
			"sit-geely-logs-2018.01.02-1",
			"sit-geely-logs-2018.01.02-000001",
			"sit-geely-logs-2018.01.02-000002",
			"uat-geely-logs-2018.01.02-000001",
		}
		idxSetting = &rollover.IdxSetting{
			Regexp:  regexp.MustCompile("sit-geely-logs-(.{10}).*"),
			Expires: 3600.0,
		}
		expect = []string{
			"sit-geely-logs-2018.01.02-1",
			"sit-geely-logs-2018.01.02-000001",
			"sit-geely-logs-2018.01.02-000002",
		}
		got []string
		err error
	)
	got, err = rollover.FilterToBeDeleteIndicies(allInd, idxSetting)
	if err != nil {
		t.Errorf("got error %+v", err)
	}
	if len(got) != 3 || got[0] != expect[0] || got[1] != expect[1] {
		t.Errorf("expect %v, got %v", expect, got)
	}
}

func TestIsIdxShouldDelete(t *testing.T) {
	var (
		dateStr     = "2018.01.02"
		expires     = 1 * time.Hour
		now         time.Time
		expect, got bool
		err         error
	)
	// case
	now, _ = time.Parse("2006-01-02 15:04:05-0700", "2018-01-03 00:01:00+0800")
	expect = false
	got, err = rollover.IsIdxShouldDelete(now, dateStr, expires)
	if err != nil {
		t.Errorf("expect %v, got error %+v", expect, err)
	}
	if got != expect {
		t.Errorf("expect %v, got %v", expect, got)
	}

	// case
	now, _ = time.Parse("2006-01-02 15:04:05-0700", "2018-01-05 01:00:01+0800")
	expect = true
	got, err = rollover.IsIdxShouldDelete(now, dateStr, expires)
	if err != nil {
		t.Errorf("expect %v, got error %+v", expect, err)
	}
	if got != expect {
		t.Errorf("expect %v, got %v", expect, got)
	}
}

func init() {
	if err := gconfig.Shared.LoadFromFile("/Users/laisky/repo/google/configs/go-ramjet/settings.yml"); err != nil {
		log.Logger.Panic("setup settings", zap.Error(err))
	}
}
