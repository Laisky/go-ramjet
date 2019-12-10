package blog

import (
	"fmt"
	"os"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/gorilla/feeds"
)

type rssCfg struct {
	title,
	link,
	authorName,
	authorEmail string
}

func generateRSSFile(rsscfg *rssCfg, fpath string, blogdb *Blog) {
	utils.Logger.Info("generateRSSFile")
	iter := blogdb.GetPostIter()
	p := &Post{}
	feed := &feeds.Feed{
		Title: rsscfg.title,
		Link:  &feeds.Link{Href: rsscfg.link},
		Author: &feeds.Author{
			Name:  rsscfg.authorName,
			Email: rsscfg.authorEmail,
		},
		Created: utils.Clock.GetUTCNow(),
	}
	feed.Items = []*feeds.Item{}
	n := 0
	for iter.Next(p) {
		feed.Items = append(feed.Items, &feeds.Item{
			Title:   p.Title,
			Link:    &feeds.Link{Href: rsscfg.link + "p/" + p.Name + "/"},
			Id:      rsscfg.link + "p/" + p.Name + "/",
			Content: p.Cnt,
			Author: &feeds.Author{
				Name: fmt.Sprintf("%v(%v)", rsscfg.authorEmail, rsscfg.authorName),
			},
			Created: p.CreatedAt,
		})
		n++

		if utils.Settings.GetBool("debug") && n > 5 {
			break
		}
	}
	rss, err := feed.ToRss()
	if err != nil {
		utils.Logger.Error("generate rss", zap.Error(err))
		return
	}
	fp, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE, 0664)
	if err != nil {
		utils.Logger.Error("open file", zap.String("fpath", fpath), zap.Error(err))
		return
	}
	if err = fp.Truncate(0); err != nil {
		utils.Logger.Error("truncate file", zap.Error(err))
		return
	}
	if _, err = fp.Seek(0, 0); err != nil {
		utils.Logger.Error("seek file", zap.Error(err))
		return
	}
	if _, err = fp.WriteString(rss); err != nil {
		utils.Logger.Error("write rss to file", zap.String("fpath", fpath), zap.Error(err))
	}
	utils.Logger.Info("generated rss", zap.Int("n_posts", n))
}
