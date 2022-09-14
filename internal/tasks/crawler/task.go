package crawler

import (
	"time"

	gconfig "github.com/Laisky/go-config"
	gutils "github.com/Laisky/go-utils/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

var muCrawler = gutils.NewMutex()

var svc *Service

// fetchAllDocus fetch all pages by sitemaps
func fetchAllDocus() {
	if !muCrawler.TryLock() {
		return
	}
	defer muCrawler.ForceRelease()

	log.Logger.Info("running web crawler")
	defer log.Logger.Info("web crawler done")

	if err := svc.CrawlAllPages(
		gconfig.Shared.GetStringSlice("tasks.crawler.sitemaps"),
	); err != nil {
		log.Logger.Error("crawl all pages", zap.Error(err))
		time.Sleep(10 * time.Second) // db reconnect
	}
}

func bindTask() {
	log.Logger.Info("bind web crawler sync monitor...")

	initSvc()
	registerWeb()

	go store.TaskStore.TickerAfterRun(gconfig.Shared.GetDuration("tasks.crawler.interval")*time.Second, fetchAllDocus)
}

func initSvc() {
	var err error
	svc, err = NewService(
		gconfig.Shared.GetString("db.crawler.addr"),
		gconfig.Shared.GetString("db.crawler.db"),
		gconfig.Shared.GetString("db.crawler.user"),
		gconfig.Shared.GetString("db.crawler.passwd"),
		gconfig.Shared.GetString("db.crawler.col_docu"),
	)
	if err != nil {
		log.Logger.Panic("new service", zap.Error(err))
	}
}

func init() {
	store.TaskStore.Store("crawler", bindTask)
}
