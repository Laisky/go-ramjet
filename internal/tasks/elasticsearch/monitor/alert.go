package monitor

import (
	"fmt"
	"time"

	"github.com/Laisky/go-ramjet/library/log"

	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/alert"
)

// monitorNodeMetrics check node metrics to determine whether to throw alert
func monitorNodeMetrics(st *ClusterSt, alertSt *AlertSt, metrics []*NodeMetric) {
	log.Logger.Debug("monitorNodeMetrics")
	// monitor fs storage
	title := "[Ramjet]ES Storage Alert"
	cnt := time.Now().Format(time.RFC3339) + "\n"
	isNeedAlert := false

	for _, m := range metrics {
		if m.FSMetric.UsageRate > alertSt.Conditions["fs_storage_rate"].(float64) {
			isNeedAlert = true

			store.TaskStore.Trigger(NodeStorageAlertEvt, map[string]interface{}{"node": m, "cluster": st}, nil, nil)
			cnt += fmt.Sprintf("%v's storage is at: %v\n", m.NodeName, m.FSMetric.UsageRate)
		}
	}

	if !isNeedAlert {
		return
	}

	for name, addr := range alertSt.Receivers {
		if err := alert.Manager.Send(
			addr,
			name,
			title,
			cnt,
		); err != nil {
			log.Logger.Error("try to send fs alert email got error", zap.Error(err))
		}
	}
}
