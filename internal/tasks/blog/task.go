// Package blog implements blog tasks.
package blog

import (
	"context"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v5"
	"github.com/Laisky/zap"
	"go.mongodb.org/mongo-driver/bson"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

var (
	blogDBMu   sync.RWMutex
	blogDB     *Blog
	blogDBCfg  blogDBConfig
	newBlogDB  = NewBlogDB
	pingBlogDB = defaultPingBlogDB
)

type blogDBConfig struct {
	addr           string
	dbName         string
	user           string
	pwd            string
	postColName    string
	keywordColName string
}

// prepareDB returns a cached blog database connection for task executions.
// It initializes the connection once per process and reuses it across runs.
func prepareDB(ctx context.Context) (db *Blog, err error) {
	cfg := blogDBConfig{
		addr:           gconfig.Shared.GetString("db.blog.addr"),
		dbName:         gconfig.Shared.GetString("db.blog.db"),
		user:           gconfig.Shared.GetString("db.blog.user"),
		pwd:            gconfig.Shared.GetString("db.blog.passwd"),
		postColName:    gconfig.Shared.GetString("db.blog.collections.posts"),
		keywordColName: gconfig.Shared.GetString("db.blog.collections.stats"),
	}

	blogDBMu.RLock()
	cacheDB := blogDB
	cacheCfg := blogDBCfg
	blogDBMu.RUnlock()
	cachePingFailed := false

	if cacheDB != nil && cacheCfg == cfg {
		if err := pingBlogDB(ctx, cacheDB); err == nil {
			return cacheDB, nil
		} else {
			log.Logger.Warn("blog mongodb ping failed, reconnecting", zap.Error(err))
			cachePingFailed = true
		}
	}

	blogDBMu.Lock()
	defer blogDBMu.Unlock()

	if blogDB != nil && blogDBCfg == cfg {
		if !(cachePingFailed && blogDB == cacheDB) {
			if err := pingBlogDB(ctx, blogDB); err == nil {
				return blogDB, nil
			} else {
				log.Logger.Warn("blog mongodb ping failed, reconnecting", zap.Error(err))
			}
		}
	}

	if db, err = newBlogDB(ctx,
		cfg.addr,
		cfg.dbName,
		cfg.user,
		cfg.pwd,
		cfg.postColName,
		cfg.keywordColName,
	); err != nil {
		return nil, errors.Wrapf(err, "connect to blog db %s/%s/%s",
			cfg.addr,
			cfg.dbName,
			cfg.postColName,
		)
	}

	if blogDB != nil && blogDB.db != nil {
		blogDB.Close(ctx)
	}

	blogDB = db
	blogDBCfg = cfg
	return blogDB, nil
}

func defaultPingBlogDB(ctx context.Context, db *Blog) error {
	if db == nil || db.db == nil {
		return errors.New("blog db not initialized")
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return db.db.CurrentDB().RunCommand(pingCtx, bson.D{{Key: "ping", Value: 1}}).Err()
}

func runRSSTask() {
	log.Logger.Info("runRSSTask")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	db, err := prepareDB(ctx)
	if err != nil {
		log.Logger.Error("connect to database got error", zap.Error(err))
		return
	}

	w, err := NewRssWorker(db)
	if err != nil {
		log.Logger.Error("NewRssWorker got error", zap.Error(err))
		return
	}

	if err := w.GenerateRSS(ctx, &rssCfg{
		title:       gconfig.Shared.GetString("tasks.blog.rss.title"),
		link:        gconfig.Shared.GetString("tasks.blog.rss.link"),
		authorName:  gconfig.Shared.GetString("tasks.blog.rss.author.name"),
		authorEmail: gconfig.Shared.GetString("tasks.blog.rss.author.email"),
	}); err != nil {
		log.Logger.Error("generate rss got error", zap.Error(err))
		return
	}

	var pool errgroup.Group
	pool.Go(func() (err error) {
		fpath := gconfig.Shared.GetString("tasks.blog.rss.rss_file_path")
		if fpath != "" {
			if err := w.Write2File(fpath); err != nil {
				return errors.Wrapf(err, "write rss to file %s", fpath)
			}
		}

		return nil
	})

	pool.Go(func() (err error) {
		if gconfig.S.GetBool("tasks.blog.rss.upload_to_s3.enable") {
			if err := w.Write2S3(ctx,
				gconfig.S.GetString("tasks.blog.rss.upload_to_s3.endpoint"),
				gconfig.S.GetString("tasks.blog.rss.upload_to_s3.access_key"),
				gconfig.S.GetString("tasks.blog.rss.upload_to_s3.access_secret"),
				gconfig.S.GetString("tasks.blog.rss.upload_to_s3.bucket"),
				gconfig.S.GetString("tasks.blog.rss.upload_to_s3.object_key"),
			); err != nil {
				return errors.Wrap(err, "write rss to s3")
			}
		}

		return nil
	})

	if err := pool.Wait(); err != nil {
		log.Logger.Error("run rss task got error", zap.Error(err))
	}
}

func runKeywordTask() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	log.Logger.Info("runKeywordTask")
	startAt := time.Now()
	db, err := prepareDB(context.Background())
	if err != nil {
		log.Logger.Error("connect to database got error", zap.Error(err))
		return
	}

	iter, err := db.GetPostIter(ctx)
	if err != nil {
		log.Logger.Error("get post iter got error", zap.Error(err))
		return
	}

	analyser := NewAnalyser()
	var (
		words              []string
		minimalCnt, errCnt int
		topN               = 5
		total              int
	)
	for iter.Next(ctx) {
		p := &Post{}
		if err := iter.Decode(p); err != nil {
			log.Logger.Error("decode post got error", zap.Error(err))
			return
		}

		minimalCnt = 3
		for {
			words = analyser.Cut2Words(p.Cnt, minimalCnt, topN)
			if len(words) == 0 {
				minimalCnt--
			} else {
				break
			}

			if minimalCnt < 0 {
				break
			}
		}
		if !gconfig.Shared.GetBool("dry") {
			err := db.UpdatePostTagsByID(ctx, p.ID, words)
			if err != nil {
				errCnt++
				log.Logger.Error("update post tags got error", zap.Error(err))

				if errCnt > 3 {
					log.Logger.Error("too many errors during update post tags, exit...")
					return
				}
			}
		}

		total++
		log.Logger.Debug("update keywords", zap.String("name", p.Name))
	}

	log.Logger.Info("succeed updated keywords",
		zap.String("cost", gutils.CostSecs(time.Since(startAt))),
		zap.Int("count", total))
	gutils.TriggerGC()
}

func bindKeywordTask() {
	log.Logger.Info("bind keyword task...")
	go store.TaskStore.TickerAfterRun(gconfig.Shared.GetDuration("tasks.blog.interval")*time.Second, runKeywordTask)
}

func bindRSSTask() {
	log.Logger.Info("bind rss task...")
	// fmt.Println(">>", gconfig.Shared.GetDuration("tasks.blog.interval"))
	go store.TaskStore.TickerAfterRun(gconfig.Shared.GetDuration("tasks.blog.interval")*time.Second, runRSSTask)
}

func init() {
	store.TaskStore.Store("rss", bindRSSTask)
	store.TaskStore.Store("keyword", bindKeywordTask)
}
