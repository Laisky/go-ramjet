package fluentd

import (
	"testing"
	"time"

	"github.com/Laisky/go-ramjet"
	utils "github.com/Laisky/go-utils"
)

func TestCheckForAlert(t *testing.T) {
	m := &fluentdMonitorMetric{
		MonitorType: "fluentd",
		Timestamp:   utils.UTCNow().Format(time.RFC3339),
		IsSITAlive:  true,
		IsUATAlive:  true,
	}

	err := checkForAlert(m)
	if err != nil {
		t.Errorf("got error: %+v", err)
	}

	utils.Logger.Flush()
	t.Error()
}

func init() {
	utils.Settings.Setup("/Users/laisky/repo/pateo/configs/go-ramjet")
	utils.SetupLogger("debug")
	ramjet.Email.Setup()

}
