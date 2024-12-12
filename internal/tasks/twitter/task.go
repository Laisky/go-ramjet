// Package twitter implements twitter sync task.
package twitter

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v5"
	glog "github.com/Laisky/go-utils/v5/log"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

func bindTask() {
	log.Logger.Info("bind twitter search sync monitor...")

	syncTweetsLock := gutils.NewMutex()

	interval := gconfig.Shared.GetDuration("tasks.twitter.search.sync.interval") * time.Second
	if interval < time.Second {
		interval = 60 * time.Second
	}

	go store.TaskStore.TickerAfterRun(interval,
		func() {
			if !syncTweetsLock.TryLock() {
				log.Logger.Debug("another sync tweets is running")
				return
			}
			defer syncTweetsLock.ForceRelease()

			if err := syncFromMongodb2Es(log.Logger.Named("sync-tweets")); err != nil {
				log.Logger.Error("sync tweets", zap.Error(err))
			}
		})
}

func syncFromMongodb2Es(logger glog.Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*3)
	defer cancel()

	twitterDao, err := NewDao(ctx,
		gconfig.Shared.GetString("db.twitter.addr"),
		gconfig.Shared.GetString("db.twitter.db"),
		gconfig.Shared.GetString("db.twitter.user"),
		gconfig.Shared.GetString("db.twitter.passwd"),
	)
	if err != nil {
		return errors.Wrap(err, "new twitter dao")
	}

	esDao, err := newElasticsearchDao(logger,
		gconfig.Shared.GetString("tasks.twitter.elasticsearch.addr"))
	if err != nil {
		return errors.Wrap(err, "new elasticsearch dao")
	}

	svc, err := newSvc(ctx, logger, twitterDao, esDao)
	if err != nil {
		return errors.Wrap(err, "new twitter svc")
	}

	if err = svc.syncTweets(ctx); err != nil {
		return errors.Wrap(err, "sync tweets")
	}

	return nil
}

func init() {
	store.TaskStore.Store("twitter-sync", bindTask)
}
