package blog

import (
	"time"

	"github.com/Laisky/go-ramjet/library/log"

	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
)

func prepareDB() (db *Blog, err error) {
	if db, err = NewBlogDB(
		utils.Settings.GetString("db.blog.addr"),
		utils.Settings.GetString("db.blog.db"),
		utils.Settings.GetString("db.blog.user"),
		utils.Settings.GetString("db.blog.passwd"),
		utils.Settings.GetString("db.blog.collections.posts"),
		utils.Settings.GetString("db.blog.collections.stats"),
	); err != nil {
		return nil, err
	}
	return
}

func runRSSTask() {
	log.Logger.Info("runRSSTask")
	db, err := prepareDB()
	if err != nil {
		log.Logger.Error("connect to database got error", zap.Error(err))
		return
	}
	defer db.Close()
	generateRSSFile(
		&rssCfg{
			title:       utils.Settings.GetString("tasks.blog.rss.title"),
			link:        utils.Settings.GetString("tasks.blog.rss.link"),
			authorName:  utils.Settings.GetString("tasks.blog.rss.author.name"),
			authorEmail: utils.Settings.GetString("tasks.blog.rss.author.email"),
		},
		utils.Settings.GetString("tasks.blog.rss.rss_file_path"),
		db,
	)
}

func runKeywordTask() {
	log.Logger.Info("runKeywordTask")
	db, err := prepareDB()
	if err != nil {
		log.Logger.Error("connect to database got error", zap.Error(err))
		return
	}
	defer db.Close()

	iter := db.GetPostIter()
	p := &Post{}
	analyser := NewAnalyser()
	var (
		words              []string
		minimalCnt, errCnt int
		topN               = 5
	)
	for iter.Next(p) {
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
		if !utils.Settings.GetBool("dry") {
			err := db.UpdatePostTagsByID(p.ID.Hex(), words)
			if err != nil {
				errCnt++
				log.Logger.Error("update post tags got error", zap.Error(err))

				if errCnt > 3 {
					log.Logger.Error("too many errors during update post tags, exit...")
					return
				}
			}
		}

		log.Logger.Info("update keywords", zap.String("name", p.Name))
	}

	utils.TriggerGC()
}

func bindKeywordTask() {
	log.Logger.Info("bind keyword task...")
	go store.TaskStore.TickerAfterRun(utils.Settings.GetDuration("tasks.blog.interval")*time.Second, runKeywordTask)
}

func bindRSSTask() {
	log.Logger.Info("bind rss task...")
	go store.TaskStore.TickerAfterRun(utils.Settings.GetDuration("tasks.blog.interval")*time.Second, runRSSTask)
}

func init() {
	store.TaskStore.Store("rss", bindRSSTask)
	store.TaskStore.Store("keyword", bindKeywordTask)
}
