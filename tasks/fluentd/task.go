package fluentd

import (
	"sync"
	"time"

	"github.com/Laisky/go-ramjet/tasks/store"
	utils "github.com/Laisky/go-utils"
	"go.uber.org/zap"
)

func runTask() {
	var (
		wg     = &sync.WaitGroup{}
		metric = &fluentdMonitorMetric{
			MonitorType: "fluentd",
			Timestamp:   utils.UTCNow().Format(time.RFC3339),
		}
	)
	for name, config := range settings {
		wg.Add(1)
		go checkFluentdHealth(wg, name, config.HealthCheckURL, metric)
	}
	wg.Wait()

	err := checkForAlert(metric)
	if err != nil {
		utils.Logger.Error("send fluentd alert got error", zap.Error(err))
	}
	err = pushResultToES(metric)
	if err != nil {
		utils.Logger.Error("push fluentd metric got error", zap.Error(err))
	}
}

func bindTask() {
	utils.Logger.Info("bind fluentd monitor...")
	settings = loadFluentdSettings()
	go store.Ticker(utils.Settings.GetDuration("tasks.fluentd.interval")*time.Second, runTask)
}

func init() {
	store.Store("fl-monitor", bindTask)
}
