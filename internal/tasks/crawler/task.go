package crawler

import (
	"time"

	gutils "github.com/Laisky/go-utils/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

var muCrawler = gutils.NewMutex()

var svc *Service

func syncCrawler() {
	if !muCrawler.TryLock() {
		return
	}
	defer muCrawler.ForceRelease()

	log.Logger.Info("running web crawler")
	defer log.Logger.Info("web crawler done")

	if err := svc.CrawlAllPages(
		gutils.Settings.GetStringSlice("tasks.crawler.sitemaps"),
	); err != nil {
		log.Logger.Panic("crawl all pages", zap.Error(err))
	}
}

func bindTask() {
	log.Logger.Info("bind web crawler sync monitor...")

	initSvc()
	registerWeb()

	go store.TaskStore.TickerAfterRun(gutils.Settings.GetDuration("tasks.crawler.interval")*time.Second, syncCrawler)
}

func initSvc() {
	var err error
	svc, err = NewService(gutils.Settings.GetString("db.crawler.dsn"))
	if err != nil {
		log.Logger.Panic("new service", zap.Error(err))
	}
}

func init() {
	store.TaskStore.Store("crawler", bindTask)
}
