package blog

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/Laisky/errors/v2"
	utils "github.com/Laisky/go-utils/v6"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gorilla/feeds"
	"github.com/minio/minio-go/v7"

	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/s3"
)

const rssVersionsToKeep = 3

type objectVersionClient interface {
	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	ListObjects(ctx context.Context, bucketName string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo
	RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error
}

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

		// Let the feeds library handle CDATA wrapping
		w.feed.Items = append(w.feed.Items, &feeds.Item{
			Title:   p.Title,
			Link:    &feeds.Link{Href: rsscfg.link + "p/" + p.Name + "/"},
			Id:      rsscfg.link + "p/" + p.Name + "/",
			Content: p.Cnt,
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
	logger.Info("run Write2S3",
		zap.String("endpoint", endpoint),
		zap.String("bucket", bucket),
		zap.String("object", objKey))

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

	if err := w.persistRSSObjectToS3(ctx, logger, s3cli, bucket, objKey, payload, rssVersionsToKeep); err != nil {
		return err
	}

	logger.Info("write rss to s3",
		zap.String("endpoint", endpoint),
		zap.String("bucket", bucket),
		zap.String("object", objKey))
	return nil
}

func (w *RssWorker) persistRSSObjectToS3(ctx context.Context, logger glog.Logger, cli objectVersionClient, bucket, key, payload string, keep int) error {
	payloadSize := int64(len(payload))
	if keep < 0 {
		keep = 0
	}
	preTrimTarget := keep - 1
	if preTrimTarget < 0 {
		preTrimTarget = 0
	}

	if preTrimTarget > 0 {
		logger.Debug("pre trimming rss versions before upload",
			zap.String("bucket", bucket),
			zap.String("object", key),
			zap.Int("target_keep", preTrimTarget))
	}
	if err := keepLatestS3ObjectVersions(ctx, logger, cli, bucket, key, preTrimTarget); err != nil {
		return errors.Wrap(err, "pre-trim s3 rss history")
	}

	upload := func() error {
		logger.Debug("uploading rss payload",
			zap.String("bucket", bucket),
			zap.String("object", key),
			zap.Int64("bytes", payloadSize))
		_, err := cli.PutObject(ctx,
			bucket,
			key,
			strings.NewReader(payload),
			payloadSize,
			minio.PutObjectOptions{ContentType: "application/xml"},
		)
		return err
	}

	if err := upload(); err != nil {
		if isVersionLimitError(err) && keep > 0 {
			logger.Debug("hit object version cap, trimming and retrying",
				zap.String("bucket", bucket),
				zap.String("object", key))
			if trimErr := keepLatestS3ObjectVersions(ctx, logger, cli, bucket, key, preTrimTarget); trimErr != nil {
				return errors.Wrap(trimErr, "trim s3 rss history after limit error")
			}
			if retryErr := upload(); retryErr != nil {
				return errors.Wrapf(retryErr, "put object %v", key)
			}
		} else {
			return errors.Wrapf(err, "put object %v", key)
		}
	}

	if err := keepLatestS3ObjectVersions(ctx, logger, cli, bucket, key, keep); err != nil {
		return errors.Wrap(err, "trim s3 rss history")
	}

	return nil
}

// keepLatestS3ObjectVersions trims S3 version history to the most recent keep entries.
func keepLatestS3ObjectVersions(ctx context.Context, logger glog.Logger, cli objectVersionClient, bucket, key string, keep int) error {
	if keep <= 0 {
		logger.Debug("skipping rss version trim",
			zap.String("bucket", bucket),
			zap.String("object", key),
			zap.Int("keep", keep))
		return nil
	}

	opts := minio.ListObjectsOptions{
		Prefix:       key,
		Recursive:    false,
		WithVersions: true,
	}

	versions := make([]minio.ObjectInfo, 0, keep+4)
	for object := range cli.ListObjects(ctx, bucket, opts) {
		if object.Err != nil {
			return errors.Wrap(object.Err, "list object versions")
		}
		if object.Key != key {
			continue
		}
		if object.VersionID == "" {
			continue
		}
		if object.IsDeleteMarker {
			continue
		}
		versions = append(versions, object)
	}

	if len(versions) <= keep {
		return nil
	}

	sort.Slice(versions, func(i, j int) bool {
		if versions[i].LastModified.Equal(versions[j].LastModified) {
			return versions[i].VersionID > versions[j].VersionID
		}
		return versions[i].LastModified.After(versions[j].LastModified)
	})

	logger.Debug("trimming excess rss versions",
		zap.String("bucket", bucket),
		zap.String("object", key),
		zap.Int("existing_versions", len(versions)),
		zap.Int("keep", keep))
	for _, version := range versions[keep:] {
		if err := cli.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{VersionID: version.VersionID}); err != nil {
			return errors.Wrapf(err, "remove object version %s", version.VersionID)
		}
		logger.Debug("removed old rss version",
			zap.String("bucket", bucket),
			zap.String("object", key),
			zap.String("version", version.VersionID))
	}

	return nil
}

func isVersionLimitError(err error) bool {
	if err == nil {
		return false
	}
	resp := minio.ToErrorResponse(err)
	if resp.Code == "MaxVersionsExceeded" {
		return true
	}
	return strings.Contains(strings.ToLower(resp.Message), "limit on the number of versions you can create")
}
