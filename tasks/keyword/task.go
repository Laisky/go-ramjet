package keyword

import (
	"runtime"
	"runtime/debug"
	"time"

	"github.com/Laisky/go-ramjet/tasks/store"
	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
)

func runTask() {
	blogdb, err := NewBlogDB(
		utils.Settings.GetString("tasks.keyword.db.addr"),
		utils.Settings.GetString("tasks.keyword.db.dbName"),
		utils.Settings.GetString("tasks.keyword.db.postColName"),
		utils.Settings.GetString("tasks.keyword.db.keywordColName"),
	)
	if err != nil {
		utils.Logger.Error("connect to database got error", zap.Error(err))
	}
	defer blogdb.Close()

	iter := blogdb.GetPostIter()
	p := &Post{}
	analyser := NewAnalyser()
	var (
		words      []string
		minimalCnt = 3
		topN       = 5
		errCnt     = 0
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
		// fmt.Println("end >>", words, len(words))
		if !utils.Settings.GetBool("dry") {
			err = blogdb.UpdatePostTagsById(p.Id.Hex(), words)
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

	runtime.GC() // bos taken too much memory
	debug.FreeOSMemory()
}

func bindTask() {
	utils.Logger.Info("bind keyword task...")
	go store.TickerAfterRun(utils.Settings.GetDuration("tasks.keyword.interval")*time.Second, runTask)
}

func init() {
	store.Store("keyword", bindTask)
}
