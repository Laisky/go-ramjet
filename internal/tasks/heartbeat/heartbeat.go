package heartbeat

import (
	"runtime"
	"time"

	"github.com/Laisky/go-ramjet/library/log"

	"github.com/Laisky/go-utils/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
)

func runTask() {
	store.TaskStore.Trigger(TaskDoneEvt, nil, nil, nil)
}

func evtHandler(evt *store.Event) {
	log.Logger.Info("heartbeat", zap.Int("goroutine", runtime.NumGoroutine()))
}

// bindTask bind heartbeat task
func bindTask() {
	log.Logger.Info("bind heartbeat task...")
	if utils.Settings.GetBool("debug") {
		utils.Settings.Set("tasks.heartbeat.interval", 10)
	}

	bindHTTP()
	go store.TaskStore.TickerAfterRun(utils.Settings.GetDuration("tasks.heartbeat.interval")*time.Second, runTask)
}

func init() {
	store.TaskStore.Store("heartbeat", bindTask)
	store.TaskStore.RegisterListener(TaskDoneEvt, "heartbeat", evtHandler)
}
