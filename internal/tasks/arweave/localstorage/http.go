package localstorage

import (
	"context"
	"io"
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	"github.com/Laisky/go-ramjet/library/web"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

type cacheResultItem struct {
	reader      io.ReadCloser
	cacheStatus string
}

// CacheHandler cache target url
func CacheHandler(ctx *gin.Context) {
	targetUrl := strings.TrimSpace(ctx.Query("url"))
	_, err := url.ParseRequestURI(targetUrl)
	if web.AbortErr(ctx, errors.Wrap(err, "invalid url")) {
		return
	}

	logger := gmw.GetLogger(ctx).With(zap.String("target_url", targetUrl))

	var pool errgroup.Group
	var resultChan = make(chan cacheResultItem)
	var errChan = make(chan error)

	taskCtx, cancel := context.WithCancel(gmw.Ctx(ctx))
	defer cancel()

	pool.Go(func() (err error) {
		reader, err := LoadContentByUrl(taskCtx, targetUrl)
		if err != nil {
			return errors.Wrapf(err, "load content by url %q", targetUrl)
		}

		select {
		case resultChan <- cacheResultItem{reader: reader, cacheStatus: "DYNAMIC"}:
			logger.Debug("load content from origin url")
		default:
			reader.Close()
		}
		return nil
	})
	pool.Go(func() error {
		reader, err := LoadContentFromS3(taskCtx, targetUrl)
		if err != nil {
			return errors.Wrapf(err, "load content from s3 %q", targetUrl)
		}

		select {
		case resultChan <- cacheResultItem{reader: reader, cacheStatus: "CACHE"}:
			logger.Debug("load content from s3 cache")
		default:
			reader.Close()
		}
		return nil
	})
	go func() {
		errChan <- pool.Wait()
	}()
	go func() {
		err := SaveUrlContent(ctx, taskItem{url: targetUrl})
		if err != nil {
			logger.Warn("save url content to s3", zap.Error(err))
		}
	}()

	select {
	case result := <-resultChan:
		defer result.reader.Close()
		ctx.Header("X-Cache-Status", result.cacheStatus)
		_, err = io.Copy(ctx.Writer, result.reader)
		if web.AbortErr(ctx, errors.Wrapf(err, "copy content from %q", targetUrl)) {
			return
		}

		ctx.Status(200)
	case err = <-errChan:
		web.AbortErr(ctx, errors.Wrapf(err, "load content from %q", targetUrl))
	}
}
