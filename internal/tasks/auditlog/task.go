// Package auditlog implements 3rd-party auditlog service.
package auditlog

import (
	"context"

	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/zap"
)

// bindTask bind heartbeat task
func bindTask() {
	logger := log.Logger.Named("auditlog")
	logger.Info("bind audit task...")

	ctx := context.Background()
	db, err := NewDB(ctx,
		gconfig.Shared.GetString("db.auditlog.addr"),
		gconfig.Shared.GetString("db.auditlog.db"),
		gconfig.Shared.GetString("db.auditlog.user"),
		gconfig.Shared.GetString("db.auditlog.passwd"),
		gconfig.Shared.GetString("db.auditlog.col_log"),
	)
	if err != nil {
		logger.Panic("new db", zap.Error(err))
	}

	svc, err := NewService(logger, db)
	if err != nil {
		logger.Panic("new service", zap.Error(err))
	}

	_ = newRouter(logger, svc)
	logger.Info("bind audit task done")
}

func init() {
	store.TaskStore.Store("auditlog", bindTask)
}
