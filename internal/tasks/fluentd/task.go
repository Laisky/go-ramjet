package fluentd

import (
	"sync"
	"time"

	"github.com/Laisky/go-ramjet/library/log"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
)

func runTask() {
	log.Logger.Info("running fl-monitor")
	defer log.Logger.Info("fl-monitor done")
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
		log.Logger.Error("send fluentd alert got error", zap.Error(err))
	}

	// err = pushResultToES(metric)
	// if err != nil {
	// 	log.Logger.Error("push fluentd metric got error", zap.Error(err))
	// }
}

func bindTask() {
	log.Logger.Info("bind fluentd monitor...")
	go store.TaskStore.TickerAfterRun(utils.Settings.GetDuration("tasks.fluentd.interval")*time.Second, runTask)
}

func init() {
	store.TaskStore.Store("fl-monitor", bindTask)
}
