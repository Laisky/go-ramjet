// Package twitter implements twitter sync task.
package twitter

import (
	"context"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v6"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

var (
	twitterDaoMu  sync.RWMutex
	twitterDao    *mongoDao
	twitterDaoCfg mongoDialConfig
	newMongoDao   = NewDao
	pingMongoDao  = defaultPingMongoDao
)

type mongoDialConfig struct {
	addr   string
	dbName string
	user   string
	pwd    string
}

// getMongoDao returns the cached twitter MongoDB DAO for the given connection settings.
// It uses ctx for the initial connection and returns any connection error.
func getMongoDao(ctx context.Context, addr, dbName, user, pwd string) (*mongoDao, error) {
	cfg := mongoDialConfig{
		addr:   addr,
		dbName: dbName,
		user:   user,
		pwd:    pwd,
	}

	twitterDaoMu.RLock()
	cacheDao := twitterDao
	cacheCfg := twitterDaoCfg
	twitterDaoMu.RUnlock()
	cachePingFailed := false

	if cacheDao != nil && cacheCfg == cfg {
		if err := pingMongoDao(ctx, cacheDao); err == nil {
			return cacheDao, nil
		} else {
			log.Logger.Warn("twitter mongodb ping failed, reconnecting", zap.Error(err))
			cachePingFailed = true
		}
	}

	twitterDaoMu.Lock()
	defer twitterDaoMu.Unlock()

	if twitterDao != nil && twitterDaoCfg == cfg {
		if !(cachePingFailed && twitterDao == cacheDao) {
			if err := pingMongoDao(ctx, twitterDao); err == nil {
				return twitterDao, nil
			} else {
				log.Logger.Warn("twitter mongodb ping failed, reconnecting", zap.Error(err))
			}
		}

		if twitterDao.db != nil {
			if err := twitterDao.db.Close(ctx); err != nil {
				log.Logger.Warn("close twitter mongodb", zap.Error(err))
			}
		}
		twitterDao = nil
	}

	dao, err := newMongoDao(ctx, addr, dbName, user, pwd)
	if err != nil {
		return nil, errors.Wrap(err, "new twitter dao")
	}

	twitterDao = dao
	twitterDaoCfg = cfg
	return twitterDao, nil
}

func defaultPingMongoDao(ctx context.Context, dao *mongoDao) error {
	if dao == nil || dao.db == nil {
		return errors.New("mongo dao not initialized")
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return dao.db.CurrentDB().RunCommand(pingCtx, bson.D{{Key: "ping", Value: 1}}).Err()
}

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

	twitterDao, err := getMongoDao(ctx,
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
