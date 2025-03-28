package gptchat

import (
	"github.com/Laisky/zap"

	iconfig "github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	gptTasks "github.com/Laisky/go-ramjet/internal/tasks/gptchat/tasks"
	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

// bindTask bind heartbeat task
func bindTask() {
	log.Logger.Info("bind gptchat task...")
	if err := iconfig.SetupConfig(); err != nil {
		log.Logger.Panic("setup gptchat config", zap.Error(err))
	}

	gptTasks.RunDynamicWebCrawler()
	bindHTTP()
}

func init() {
	store.TaskStore.Store("gptchat", bindTask)
}
