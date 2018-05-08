package heartbeat

import (
	"runtime"
	"time"

	log "github.com/cihub/seelog"
	"github.com/spf13/viper"
	"github.com/Laisky/go-ramjet/tasks/store"
	"github.com/Laisky/go-ramjet/utils"
)

func runTask() {
	log.Infof("heartbeat with %v active goroutines", runtime.NumGoroutine())

	// reload settings
	utils.LoadSettings()
}

// bindTask bind heartbeat task
func bindTask() {
	log.Info("bind heartbeat task...")
	if viper.GetBool("debug") {
		viper.Set("tasks.heartbeat.interval", 1)
	}

	go store.Ticker(viper.GetDuration("tasks.heartbeat.interval")*time.Second, runTask)
}

func init() {
	store.Store("heartbeat", bindTask)
}
