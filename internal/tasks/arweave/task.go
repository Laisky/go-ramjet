package arweave

import (
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/arweave/config"
	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

// bindTask bind heartbeat task
func bindTask() {
	log.Logger.Info("bind arweave task...")

	if err := config.SetupConfig(); err != nil {
		log.Logger.Panic("setup arweave config", zap.Error(err))
	}

	bindHTTP()
}

func init() {
	store.TaskStore.Store("arweave", bindTask)
}
