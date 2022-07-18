package twitter

import (
	"time"

	gconfig "github.com/Laisky/go-config"
	gutils "github.com/Laisky/go-utils/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

var muSearch = gutils.NewMutex()
var muReplica = gutils.NewMutex()

func syncSearch() {
	if !muSearch.TryLock() {
		return
	}
	defer muSearch.ForceRelease()

	log.Logger.Info("running twitter sync search")
	defer log.Logger.Info("twitter sync search done")

	if err := svc.SyncSearchTweets(); err != nil {
		log.Logger.Error("sync search tweets", zap.Error(err))
	}
}

func syncReplica() {
	if !muReplica.TryLock() {
		return
	}
	defer muReplica.ForceRelease()

	log.Logger.Info("running twitter sync replica")
	defer log.Logger.Info("twitter sync replica done")

	if err := svc.SyncReplicaTweets(); err != nil {
		log.Logger.Error("sync replica tweets", zap.Error(err))
	}
}

func bindTask() {
	log.Logger.Info("bind twitter search sync monitor...")
	if err := initSvc(); err != nil {
		log.Logger.Panic("init twitter svc", zap.Error(err))
	}

	go store.TaskStore.TickerAfterRun(gconfig.Shared.GetDuration("tasks.twitter.search.sync.interval")*time.Second, syncSearch)
	go store.TaskStore.TickerAfterRun(gconfig.Shared.GetDuration("tasks.twitter.search.sync.interval")*time.Second, syncReplica)
}

func init() {
	store.TaskStore.Store("twitter-sync-search", bindTask)
}
