package heartbeat

import (
	"runtime"
	"time"

	gconfig "github.com/Laisky/go-config"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
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
	if gconfig.Shared.GetBool("debug") {
		gconfig.Shared.Set("tasks.heartbeat.interval", 10)
	}

	bindHTTP()
	go store.TaskStore.TickerAfterRun(gconfig.Shared.GetDuration("tasks.heartbeat.interval")*time.Second, runTask)
}

func init() {
	store.TaskStore.Store("heartbeat", bindTask)
	store.TaskStore.RegisterListener(TaskDoneEvt, "heartbeat", evtHandler)
}
