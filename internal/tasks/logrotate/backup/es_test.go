package backup_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/go-ramjet/internal/tasks/logrotate/backup"
)

func TestScanFiles(t *testing.T) {
	for _, fpath := range backup.ScanFiles(".", "") {
		if _, err := os.Stat(fpath); os.IsNotExist(err) {
			t.Errorf("file not exists: %v", fpath)
		}
	}
}

func TestGenRsyncCMD(t *testing.T) {
	expect := "rsync -tvhz fluent.conf 172.16.4.110::ivilog_bak"
	r := backup.GenRsyncCMD("fluent.conf", "172.16.4.110::ivilog_bak")
	if strings.Join(r, " ") != expect {
		t.Errorf("expect %v, got %v", expect, strings.Join(r, " "))
	}
}

func TestRunSysCMD(t *testing.T) {
	got, err := backup.RunSysCMD([]string{"uptime"})
	if err != nil {
		t.Errorf("%+v", err)
	}
	// match common linux uptime output like: ' 14:20:52 up 1 day,  3:22,  2 users,  load average: 0.44, 0.37, 0.36'
	if matched, err := regexp.MatchString(`\busers?,\s+load average`, got); !matched || err != nil {
		t.Errorf("unexpected uptime output: %q (err=%v)", got, err)
	}
}

func TestIsFileReadyToUpload(t *testing.T) {
	var (
		regex, fname, layout string
		now                  time.Time
		expect, got          bool
		err                  error
	)

	// case 1
	regex = "^(\\d{8})\\.log\\.gz$"
	fname = "20180418.log.gz"
	layout = "20060102 15-0700"
	now, _ = time.Parse(layout, "20180419 11+0800")
	expect = true
	if got, err = backup.IsFileReadyToUpload(regex, fname, now); err != nil {
		t.Errorf("got error %+v", err)
	} else if got != expect {
		t.Errorf("expect %v, got %v", expect, got)
	}

	// case2
	now, _ = time.Parse(layout, "20180419 10+0800")
	expect = false
	got, err = backup.IsFileReadyToUpload(regex, fname, now)
	if err != nil {
		t.Errorf("got error %+v", err)
	}
	if got != expect {
		t.Errorf("expect %v, got %v", expect, got)
	}
}
