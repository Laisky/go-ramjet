package twitter

import (
	"time"

	gutils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

var mu = gutils.NewMutex()

func runTask() {
	if !mu.TryLock() {
		return
	}
	defer mu.ForceRelease()

	log.Logger.Info("running twitter sync search")
	defer log.Logger.Info("twitter sync done")

	if err := svc.SyncSearchTweets(); err != nil {
		log.Logger.Error("sync search tweets", zap.Error(err))
	}
}

func bindTask() {
	log.Logger.Info("bind twitter search sync monitor...")
	initSvc()

	go store.TaskStore.TickerAfterRun(gutils.Settings.GetDuration("tasks.twitter.search.sync.interval")*time.Second, runTask)
}

func init() {
	store.TaskStore.Store("twitter-sync-search", bindTask)
}
