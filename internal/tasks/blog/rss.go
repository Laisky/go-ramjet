package blog

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Laisky/errors/v2"
	utils "github.com/Laisky/go-utils/v5"
	glog "github.com/Laisky/go-utils/v5/log"
	"github.com/Laisky/zap"
	"github.com/gorilla/feeds"
	"github.com/minio/minio-go/v7"

	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/s3"
)

// RssWorker rss worker
type RssWorker struct {
	logger glog.Logger
	feed   *feeds.Feed
	db     *Blog
}

// NewRssWorker create rss worker
func NewRssWorker(blogdb *Blog) (*RssWorker, error) {
	w := &RssWorker{
		logger: log.Logger.Named("rss"),
		db:     blogdb,
	}

	return w, nil
}

type rssCfg struct {
	title,
	link,
	authorName,
	authorEmail string
}

// GenerateRSS scan all posts and generate rss
func (w *RssWorker) GenerateRSS(ctx context.Context, rsscfg *rssCfg) (err error) {
	log.Logger.Info("generateRSS")
	iter, err := w.db.GetPostIter(ctx)
	if err != nil {
		return errors.Wrap(err, "get post iter")
	}
	defer iter.Close(ctx) // nolint: errcheck

	w.feed = &feeds.Feed{
		Title: rsscfg.title,
		Link:  &feeds.Link{Href: rsscfg.link},
		Author: &feeds.Author{
			Name:  rsscfg.authorName,
			Email: rsscfg.authorEmail,
		},
		Created: utils.Clock.GetUTCNow(),
	}
	w.feed.Items = []*feeds.Item{}

	for iter.Next(ctx) {
		p := &Post{}
		if err = iter.Decode(p); err != nil {
			return errors.Wrap(err, "decode post")
		}

		// Sanitize content by wrapping it in CDATA
		content := fmt.Sprintf("<![CDATA[%s]]>", p.Cnt)
		title := fmt.Sprintf("<![CDATA[%s]]>", p.Title)

		w.feed.Items = append(w.feed.Items, &feeds.Item{
			Title:   title,
			Link:    &feeds.Link{Href: rsscfg.link + "p/" + p.Name + "/"},
			Id:      rsscfg.link + "p/" + p.Name + "/",
			Content: content,
			Author: &feeds.Author{
				Name: fmt.Sprintf("%v(%v)", rsscfg.authorEmail, rsscfg.authorName),
			},
			Created: p.CreatedAt,
		})
	}

	return nil
}

// Write2File write rss to file
func (w *RssWorker) Write2File(fpath string) (err error) {
	logger := w.logger.Named("file")
	logger.Info("run Write2File", zap.String("fpath", fpath))

	fp, err := os.CreateTemp("", "*")
	if err != nil {
		return errors.Wrap(err, "create temp file")
	}

	if err = w.feed.WriteRss(fp); err != nil {
		return errors.Wrap(err, "write rss")
	}

	if err = fp.Close(); err != nil {
		return errors.Wrap(err, "close file")
	}

	if err = os.Rename(fp.Name(), fpath); err != nil {
		return errors.Wrap(err, "rename file")
	}

	logger.Info("write rss to file", zap.String("fpath", fpath))
	return nil
}

// Write2S3 write rss to s3
func (w *RssWorker) Write2S3(ctx context.Context,
	endpoint,
	accessKey,
	accessSecret,
	bucket,
	objKey string,
) error {
	logger := w.logger.Named("s3")
	logger.Info("run Write2File", zap.String("s3", endpoint))

	s3cli, err := s3.GetCli(
		endpoint,
		accessKey,
		accessSecret,
	)
	if err != nil {
		return errors.Wrap(err, "new s3 client")
	}

	payload, err := w.feed.ToRss()
	if err != nil {
		return errors.Wrap(err, "to rss")
	}

	if _, err = s3cli.PutObject(ctx,
		bucket,
		objKey,
		strings.NewReader(payload),
		int64(len([]byte(payload))),
		minio.PutObjectOptions{
			ContentType: "application/xml",
		},
	); err != nil {
		return errors.Wrapf(err, "put object %v", objKey)
	}

	logger.Info("write rss to s3", zap.String("s3", endpoint))
	return nil
}
