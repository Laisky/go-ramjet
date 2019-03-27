package fluentd

import (
	"sync"
	"time"

	"github.com/Laisky/go-ramjet/tasks/store"
	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
)

func runTask() {
	utils.Logger.Info("running fl-monitor")
	defer utils.Logger.Info("fl-monitor done")
	wg := &sync.WaitGroup{}
	metric := &sync.Map{}
	settings := loadFluentdSettings()
	for _, cfg := range settings {
		wg.Add(1)
		go checkFluentdHealth(wg, cfg, metric)
	}
	wg.Wait()

	err := checkForAlert(metric)
	if err != nil {
		utils.Logger.Error("send fluentd alert got error", zap.Error(err))
	}

	// err = pushResultToES(metric)
	// if err != nil {
	// 	utils.Logger.Error("push fluentd metric got error", zap.Error(err))
	// }
}

func bindTask() {
	utils.Logger.Info("bind fluentd monitor...")
	go store.Ticker(utils.Settings.GetDuration("tasks.fluentd.interval")*time.Second, runTask)
}

func init() {
	store.Store("fl-monitor", bindTask)
}
