package notes

import (
	"context"
	"time"

	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
	gutils "github.com/Laisky/go-utils/v5"
	glog "github.com/Laisky/go-utils/v5/log"
	"github.com/Laisky/zap"
	"golang.org/x/sync/errgroup"
)

var muTelegram = gutils.NewMutex()

func fetchTelegramNotes(logger glog.Logger, svc *Service) func() {
	return func() {
		if !muTelegram.TryLock() {
			return
		}
		defer muTelegram.ForceRelease()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		logger.Info("running telegram notes crawler")
		defer logger.Info("telegram notes crawler done")

		lastPostID, err := svc.GetLatestPostID(ctx)
		if err != nil {
			logger.Error("get latest post id", zap.Error(err))
			return
		}

		var pool errgroup.Group
		pool.SetLimit(10)

		for i := 1; i < lastPostID+5; i++ {
			i := i
			pool.Go(func() error {
				logger.Info("fetch note", zap.Int("post_id", i))
				return svc.FetchContent(ctx, i)
			})
		}

		if err := pool.Wait(); err != nil {
			logger.Error("all task done", zap.Error(err))
		}
	}
}

func bindTask() {
	logger := log.Logger.Named("telegram_notes")
	logger.Info("bind telegram notes crawler...")

	ctx := context.Background()
	svc, err := NewService(ctx,
		logger,
		gconfig.Shared.GetString("db.telegram.addr"),
		gconfig.Shared.GetString("db.telegram.db"),
		gconfig.Shared.GetString("db.telegram.user"),
		gconfig.Shared.GetString("db.telegram.passwd"),
		gconfig.Shared.GetString("db.telegram.col_notes"),
	)
	if err != nil {
		logger.Panic("new service", zap.Error(err))
	}

	go store.TaskStore.TickerAfterRun(
		time.Hour*24,
		fetchTelegramNotes(logger, svc),
	)
}

func init() {
	store.TaskStore.Store("telegram_notes", bindTask)
}
