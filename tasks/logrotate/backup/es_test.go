package backup

import (
	"os"
	"regexp"
	"testing"
)

func TestScanFiles(t *testing.T) {
	for _, fpath := range scanFiles(".", "") {
		if _, err := os.Stat(fpath); os.IsNotExist(err) {
			t.Errorf("file not exists: %v", fpath)
		}
	}
}

func TestGenRsyncCMD(t *testing.T) {
	expect := "rsync -tvhz fluent.conf 172.16.4.110::ivilog_bak"
	r := genRsyncCMD("fluent.conf", "172.16.4.110::ivilog_bak")
	if r != expect {
		t.Errorf("expect %v, got %v", expect, r)
	}
}

func TestRunSysCMD(t *testing.T) {
	got := runSysCMD("uptime")
	if matched, err := regexp.MatchString("users, load averages: ", got); !matched || err != nil {
		t.Errorf("matched error, got: %v", got)
	}
}
