// Package blog implements blog tasks.
package blog

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/zap"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

func prepareDB(ctx context.Context) (db *Blog, err error) {
	if db, err = NewBlogDB(ctx,
		gconfig.Shared.GetString("db.blog.addr"),
		gconfig.Shared.GetString("db.blog.db"),
		gconfig.Shared.GetString("db.blog.user"),
		gconfig.Shared.GetString("db.blog.passwd"),
		gconfig.Shared.GetString("db.blog.collections.posts"),
		gconfig.Shared.GetString("db.blog.collections.stats"),
	); err != nil {
		return nil, errors.Wrapf(err, "connect to blog db %s/%s/%s",
			gconfig.Shared.GetString("db.blog.addr"),
			gconfig.Shared.GetString("db.blog.db"),
			gconfig.Shared.GetString("db.blog.collections.posts"),
		)
	}
	return
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
	defer db.Close(ctx)

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
	pool.Go(func() error {
		fpath := gconfig.Shared.GetString("tasks.blog.rss.rss_file_path")
		if fpath != "" {
			if err := w.Write2File(fpath); err != nil {
				return errors.Wrapf(err, "write rss to file %s", fpath)
			}
		}

		return nil
	})

	pool.Go(func() error {
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
	defer db.Close(ctx)

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
