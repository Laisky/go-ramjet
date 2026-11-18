package localstorage

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-ramjet/library/egress"
	"github.com/Laisky/go-ramjet/library/web"
)

type cacheResultItem struct {
	reader      io.ReadCloser
	cacheStatus string
}

// CacheHandler proxies a user-selected public url, preferring a live fetch and
// falling back to the S3 archive, and schedules the url for archiving.
//
// The target is admitted with the shared destination policy before any fetch,
// cache lookup or archive scheduling. Refused targets are answered with 400 and
// logged below the alert level; redirects are admitted again on every hop.
func CacheHandler(ctx *gin.Context) {
	targetUrl := strings.TrimSpace(ctx.Query("url"))
	logger := gmw.GetLogger(ctx).With(zap.String("target_url", targetUrl))
	if _, err := url.ParseRequestURI(targetUrl); err != nil {
		abortTargetError(ctx, logger, errors.WithStack(&invalidTargetError{cause: err}))
		return
	}

	requestCtx := gmw.Ctx(ctx)
	if _, err := egress.Admit(requestCtx, targetUrl, targetNetwork.Resolve); err != nil {
		abortTargetError(ctx, logger, errors.Wrapf(err, "admit target %q", targetUrl))
		return
	}

	var pool errgroup.Group
	var resultChan = make(chan cacheResultItem)
	var errChan = make(chan error)

	taskCtx, cancel := context.WithCancel(requestCtx)
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
		time.Sleep(time.Second)
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
	// The gin context is recycled after this handler returns; the archive
	// scheduler keeps only request values and outlives request cancellation.
	archiveCtx := context.WithoutCancel(requestCtx)
	go func() {
		err := SaveUrlContent(archiveCtx, taskItem{url: targetUrl})
		if err != nil {
			logger.Warn("save url content to s3", zap.Error(err))
		}
	}()

	select {
	case result := <-resultChan:
		defer result.reader.Close()
		ctx.Header("X-Laisky-Cache-Status", result.cacheStatus)
		_, err := io.Copy(ctx.Writer, result.reader)
		if web.AbortErr(ctx, errors.Wrapf(err, "copy content from %q", targetUrl)) {
			return
		}

		ctx.Status(200)
	case err := <-errChan:
		abortTargetError(ctx, logger, errors.Wrapf(err, "load content from %q", targetUrl))
	}
}

// invalidTargetError marks a target that is not even a parseable request URI.
type invalidTargetError struct{ cause error }

// Error describes the malformed target without adding anything beyond the parser's message.
func (e *invalidTargetError) Error() string { return "invalid url: " + e.cause.Error() }

// Unwrap returns the parser error.
func (e *invalidTargetError) Unwrap() error { return e.cause }

// abortTargetError answers a refused or failed target exactly once. Requests the
// caller can fix (malformed URLs and destination-policy refusals, including a
// redirect into a private network) get 400 and a WARN log, so they never consume
// the shared ERROR alert budget. Every other failure keeps the existing
// web.AbortErr behavior.
func abortTargetError(ctx *gin.Context, logger glog.Logger, err error) {
	var malformed *invalidTargetError
	reason, refused := egress.ReasonOf(err)
	switch {
	case errors.As(err, &malformed):
		logger.Warn("refuse malformed proxy target", zap.Error(err))
	case refused && reason.Expected():
		logger.Warn("refuse proxy target by destination policy", zap.String("reason", string(reason)), zap.Error(err))
	default:
		web.AbortErr(ctx, err)
		return
	}
	ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"err": err.Error()})
}
