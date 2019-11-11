package heartbeat

import (
	"runtime"
	"time"

	"github.com/Laisky/go-ramjet/tasks/store"
	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
)

func runTask() {
	store.TaskStore.Trigger(TaskDoneEvt, nil, nil, nil)
}

func evtHandler(evt *store.Event) {
	utils.Logger.Info("heartbeat", zap.Int("goroutine", runtime.NumGoroutine()))
}

// bindTask bind heartbeat task
func bindTask() {
	utils.Logger.Info("bind heartbeat task...")
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
