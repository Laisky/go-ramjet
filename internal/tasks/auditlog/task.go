// Package auditlog implements 3rd-party auditlog service.
package auditlog

import (
	"context"
	"crypto/x509"

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
		gconfig.Shared.GetString("tasks.auditlog.db.addr"),
		gconfig.Shared.GetString("tasks.auditlog.db.db"),
		gconfig.Shared.GetString("tasks.auditlog.db.user"),
		gconfig.Shared.GetString("tasks.auditlog.db.passwd"),
		gconfig.Shared.GetString("tasks.auditlog.db.col_log"),
	)
	if err != nil {
		logger.Panic("new db", zap.Error(err))
	}

	var rootcaPool *x509.CertPool
	if rootpem := gconfig.Shared.GetString("tasks.auditlog.root_ca_pem"); rootpem != "" {
		rootcaPool := x509.NewCertPool()
		rootcaPool.AppendCertsFromPEM([]byte(rootpem))
	}

	svc, err := newService(logger, db, rootcaPool)
	if err != nil {
		logger.Panic("new service", zap.Error(err))
	}

	_ = newRouter(logger, svc)
	logger.Info("bind audit task done")
}

func init() {
	store.TaskStore.Store("auditlog", bindTask)
}
