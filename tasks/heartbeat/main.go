package heartbeat

import (
	"runtime"
	"time"

	log "github.com/cihub/seelog"
	"github.com/go-ramjet/tasks/store"
	"github.com/go-ramjet/utils"
	"github.com/spf13/viper"
)

func setNext(f func()) {
	utils.LoadSettings()
	time.AfterFunc(viper.GetDuration("tasks.heartbeat.interval")*time.Second, func() {
		store.PutReadyTask(f)
	})
}

func runTask() {
	defer log.Flush()
	log.Infof("heartbeat with %v active gorouines", runtime.NumGoroutine())
	go setNext(runTask)
}

// bindTask bind heartbeat task
func bindTask() {
	defer log.Flush()
	log.Info("Bind heartbeat task...")
	if viper.GetBool("debug") {
		viper.Set("tasks.heartbeat.interval", 1)
	}
	go setNext(runTask)
}

func init() {
	store.Store(bindTask)
}
