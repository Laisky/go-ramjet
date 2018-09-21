package heartbeat

import (
	"runtime"
	"time"

	"github.com/Laisky/go-ramjet/tasks/store"
	"github.com/Laisky/go-utils"
	"go.uber.org/zap"
)

func runTask() {
	utils.Logger.Info("heartbeat", zap.Int("goroutine", runtime.NumGoroutine()))

	// reload settings
	if err := utils.Settings.LoadSettings(); err != nil {
		utils.Logger.Error("reload settings got error", zap.Error(err))
	}
}

// bindTask bind heartbeat task
func bindTask() {
	utils.Logger.Info("bind heartbeat task...")
	if utils.Settings.GetBool("debug") {
		utils.Settings.Set("tasks.heartbeat.interval", 10)
	}

	bindHTTP()
	go store.TickerAfterRun(utils.Settings.GetDuration("tasks.heartbeat.interval")*time.Second, runTask)
}

func init() {
	store.Store("heartbeat", bindTask)
}
