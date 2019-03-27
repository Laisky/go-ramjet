package monitor

import (
	"fmt"
	"time"

	"github.com/Laisky/go-ramjet"
	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
)

func monitorNodeMetrics(alertSt *AlertSt, metrics []*NodeMetric) {
	utils.Logger.Debug("monitorNodeMetrics")
	// monitor fs storage
	title := "[Ramjet]ES Storage Alert"
	cnt := time.Now().Format(time.RFC3339) + "\n"
	isNeedAlert := false

	for _, m := range metrics {
		fmt.Println(m.FSMetric.UsageRate)
		fmt.Println("> ", alertSt.Conditions["fs_storage_rate"].(float64))
		if m.FSMetric.UsageRate > alertSt.Conditions["fs_storage_rate"].(float64) {
			isNeedAlert = true

			cnt += fmt.Sprintf("%v's storage is at: %v\n", m.NodeName, m.FSMetric.UsageRate)
		}
	}

	if !isNeedAlert {
		return
	}

	for name, addr := range alertSt.Receivers {
		if err := ramjet.Email.Send(
			addr,
			name,
			title,
			cnt,
		); err != nil {
			utils.Logger.Error("try to send fs alert email got error", zap.Error(err))
		}
	}
}
