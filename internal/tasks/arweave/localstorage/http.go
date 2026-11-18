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

	"github.com/Laisky/go-ramjet/library/egress"
	"github.com/Laisky/go-ramjet/library/web"
)

var (
	// loadOriginContent fetches the live target through the pinned egress client; tests replace it.
	loadOriginContent = LoadContentByUrl
	// loadCachedContent fetches the archived copy from S3; tests replace it.
	loadCachedContent = LoadContentFromS3
	// cacheFallbackDelay is the live fetch's head start before the archive is raced against it.
	cacheFallbackDelay = time.Second
)

// CacheHandler proxies a user-selected public url, preferring a live fetch and
// falling back to the S3 archive, and schedules the url for archiving.
//
// The target is admitted with the shared destination policy before any fetch,
// cache lookup or archive scheduling. Refused targets are answered with 400 and
// logged below the alert level; redirects are admitted again on every hop.
//
// The live fetch starts at once and the archive lookup after cacheFallbackDelay;
// the first to succeed answers, the other is canceled and its body closed, and
// no goroutine outlives the request except the archive scheduler.
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

	// The gin context is recycled after this handler returns; the archive
	// scheduler keeps only request values and outlives request cancellation.
	archiveCtx := context.WithoutCancel(requestCtx)
	go func() {
		err := SaveUrlContent(archiveCtx, taskItem{url: targetUrl})
		if err != nil {
			logger.Warn("save url content to s3", zap.Error(err))
		}
	}()

	body, cacheStatus, err := firstAvailableContent(requestCtx, logger, targetUrl, []contentSource{
		{name: "origin url", status: "DYNAMIC", load: loadOriginContent},
		{name: "s3 cache", status: "CACHE", delay: cacheFallbackDelay, load: loadCachedContent},
	})
	if err != nil {
		abortTargetError(ctx, logger, errors.Wrapf(err, "load content from %q", targetUrl))
		return
	}
	defer func() {
		if closeErr := body.Close(); closeErr != nil {
			logger.Debug("close proxied content", zap.Error(closeErr))
		}
	}()

	logger.Debug("serve proxied content", zap.String("cache_status", cacheStatus))
	ctx.Header("X-Laisky-Cache-Status", cacheStatus)
	if _, err := io.Copy(ctx.Writer, body); err != nil {
		web.AbortErr(ctx, errors.Wrapf(err, "copy content from %q", targetUrl))
		return
	}

	ctx.Status(http.StatusOK)
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
