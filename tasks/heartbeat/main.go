package heartbeat

import (
	"runtime"
	"time"

	"github.com/Laisky/go-ramjet/tasks/store"
	"github.com/Laisky/go-utils"
)

func runTask() {
	utils.Logger.Infof("heartbeat with %v active goroutines", runtime.NumGoroutine())

	// reload settings
	utils.Settings.LoadSettings()
}

// bindTask bind heartbeat task
func bindTask() {
	utils.Logger.Info("bind heartbeat task...")
	if utils.Settings.GetBool("debug") {
		utils.Settings.Set("tasks.heartbeat.interval", 1)
	}

	bindHttp()
	go store.Ticker(utils.Settings.GetDuration("tasks.heartbeat.interval")*time.Second, runTask)
}

func init() {
	store.Store("heartbeat", bindTask)
}
