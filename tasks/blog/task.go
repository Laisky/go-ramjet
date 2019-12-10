package blog

import (
	"time"

	"github.com/Laisky/go-ramjet/tasks/store"
	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
)

func prepareDB() (db *Blog, err error) {
	if db, err = NewBlogDB(
		utils.Settings.GetString("tasks.blog.db.addr"),
		utils.Settings.GetString("tasks.blog.db.dbName"),
		utils.Settings.GetString("tasks.blog.db.user"),
		utils.Settings.GetString("tasks.blog.db.passwd"),
		utils.Settings.GetString("tasks.blog.db.postColName"),
		utils.Settings.GetString("tasks.blog.db.keywordColName"),
	); err != nil {
		return nil, err
	}
	return
}

func runRSSTask() {
	utils.Logger.Info("runRSSTask")
	db, err := prepareDB()
	if err != nil {
		utils.Logger.Error("connect to database got error", zap.Error(err))
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
	utils.Logger.Info("runKeywordTask")
	db, err := prepareDB()
	if err != nil {
		utils.Logger.Error("connect to database got error", zap.Error(err))
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
			err := db.UpdatePostTagsById(p.Id.Hex(), words)
			if err != nil {
				errCnt++
				utils.Logger.Error("update post tags got error", zap.Error(err))

				if errCnt > 3 {
					utils.Logger.Error("too many errors during update post tags, exit...")
					return
				}
			}
		}

		utils.Logger.Info("update keywords", zap.String("name", p.Name))
	}

	utils.TriggerGC()
}

func bindKeywordTask() {
	utils.Logger.Info("bind keyword task...")
	go store.TaskStore.TickerAfterRun(utils.Settings.GetDuration("tasks.blog.interval")*time.Second, runKeywordTask)
}

func bindRSSTask() {
	utils.Logger.Info("bind rss task...")
	go store.TaskStore.TickerAfterRun(utils.Settings.GetDuration("tasks.blog.interval")*time.Second, runRSSTask)
}

func init() {
	store.TaskStore.Store("rss", bindRSSTask)
	store.TaskStore.Store("keyword", bindKeywordTask)
}
