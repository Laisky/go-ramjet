package arweave

import (
	"context"

	gmw "github.com/Laisky/gin-middlewares/v6"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/arweave/config"
	"github.com/Laisky/go-ramjet/internal/tasks/arweave/localstorage"
	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

// bindTask bind heartbeat task
func bindTask() {
	ctx := gmw.SetLogger(context.Background(), log.Logger.Named("arweave"))
	log.Logger.Info("bind arweave task...")

	if err := config.SetupConfig(); err != nil {
		log.Logger.Panic("setup arweave config", zap.Error(err))
	}

	localstorage.RunSaveUrlContent(ctx)

	bindHTTP()
}

func init() {
	store.TaskStore.Store("arweave", bindTask)
}
