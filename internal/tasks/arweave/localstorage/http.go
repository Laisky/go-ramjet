package localstorage

import (
	"bufio"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
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
	// requestBackgroundWork tracks goroutines a proxy request leaves behind: the
	// archive scheduler, sources still being canceled, and the cleanup of their
	// bodies. Each ends shortly after its request; waiting on it gives a clean
	// shutdown point and lets tests synchronize with that work.
	requestBackgroundWork sync.WaitGroup
)

// sniffLen is how many leading bytes http.DetectContentType considers.
const sniffLen = 512

// CacheHandler proxies a user-selected public url, preferring a live fetch and
// falling back to the S3 archive, and schedules the url for archiving.
//
// Proxied content is third-party content served on this origin, so it is marked
// untrusted: active types are sandboxed into an opaque, script-free origin, while
// PDFs stay viewable in the browser's built-in viewer.
//
// The target is admitted with the shared destination policy before any fetch,
// cache lookup or archive scheduling. Refused targets are answered with 400 and
// logged below the alert level; redirects are admitted again on every hop.
//
// The live fetch starts at once and the archive lookup after cacheFallbackDelay;
// the first to succeed answers, the other is canceled and any body it returns is
// closed. Work that briefly outlives the request (the bounded archive scheduler
// started by scheduleArchive, canceled sources and their cleanup) is tracked by
// requestBackgroundWork. A failure after streaming began aborts the transfer.
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

	scheduleArchive(requestCtx, logger, targetUrl)

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

	// Detect the type from the first bytes, as net/http would, so the untrusted
	// content policy can tell active documents from passive ones before writing.
	content := bufio.NewReaderSize(body, sniffLen)
	head, err := content.Peek(sniffLen)
	if err != nil && !errors.Is(err, io.EOF) {
		web.AbortErr(ctx, errors.Wrapf(err, "read content from %q", targetUrl))
		return
	}
	contentType := http.DetectContentType(head)

	logger.Debug("serve proxied content", zap.String("cache_status", cacheStatus), zap.String("content_type", contentType))
	web.MarkUntrustedContent(ctx, contentType)
	ctx.Header("Content-Type", contentType)
	ctx.Header("X-Laisky-Cache-Status", cacheStatus)
	// Once bytes are written the status is committed, so a failure mid-copy must
	// abort the transfer rather than append an error document to the partial body.
	if _, err := io.Copy(ctx.Writer, content); err != nil {
		web.AbortStreamErr(ctx, errors.Wrapf(err, "copy content from %q", targetUrl))
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
