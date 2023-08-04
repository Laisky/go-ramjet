// Package auditlog implements 3rd-party auditlog service.
package auditlog

import (
	"context"
	"crypto/x509"
	"time"

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
		gconfig.Shared.GetString("tasks.auditlog.db.col_task"),
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

	// bind http
	_ = newRouter(logger, svc)

	// bind tasks
	go store.TaskStore.TickerAfterRun(time.Minute, func() {
		if err := svc.checkClunterFingerprint(ctx,
			gconfig.Shared.GetString("tasks.auditlog.cluster_fingerprint_url"),
		); err != nil {
			logger.Error("checkClunterFingerprint", zap.Error(err))
		}
	})

	logger.Info("bind audit task done")
}

func init() {
	store.TaskStore.Store("auditlog", bindTask)
}
