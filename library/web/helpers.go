package web

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
)

const httpStatusClientClosedRequest = 499

// isContextCanceledErr returns true if err indicates the request was canceled by the client
// or the request context has ended.
func isContextCanceledErr(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) {
		return true
	}

	// Some network layers don't wrap context.Canceled, but surface it as a string.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "client disconnected") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset by peer")
}

// AbortErr abort with error
func AbortErr(ctx *gin.Context, err error) bool {
	if err == nil {
		return false
	}

	logger := gmw.GetLogger(ctx)
	if isContextCanceledErr(err) {
		logger.Debug("request canceled", zap.Error(err))
		ctx.AbortWithStatus(httpStatusClientClosedRequest)
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		logger.Warn("request timeout", zap.Error(err))
		ctx.AbortWithStatusJSON(http.StatusGatewayTimeout, gin.H{
			"err": err.Error(),
		})
		return true
	}

	logger.Error("chat abort", zap.Error(err))
	ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
		"err": err.Error(),
	})
	return true
}

// relayedRepresentationHeaders describe a relayed body and its caching. They
// must not label an error document that replaces that body.
var relayedRepresentationHeaders = []string{
	"Accept-Ranges", "Cache-Control", "Content-Disposition", "Content-Encoding", "Content-Language",
	"Content-Length", "Content-Location", "Content-Range", "Content-Type", "Etag", "Expires", "Last-Modified",
}

// AbortStreamErr ends ctx's response with err when part of the body may already
// have been written, as when relaying a stream, and reports whether err was non-nil.
//
// Before the status is written it answers like AbortErr, after removing the
// headers a relay may already have copied for its body (type, length, coding,
// range, validators and caching) and marking the answer Cache-Control: no-store,
// so the error document is complete, decodable and never cached as the relayed
// resource. Protective headers such as Content-Security-Policy stay. Afterwards the
// status is committed and the client is reading the body: appending an error
// document would corrupt it, and ending normally would make the truncated body
// look complete. Instead err is logged with AbortErr's severity and the response
// is aborted through http.ResponseController: HTTP/1.1 closes the connection
// before the final chunk and HTTP/2 resets the stream, so the client sees a
// failed transfer. An HTTP/1.0 body is delimited by the connection closing, so
// there the abort cannot be told from a normal end; the body is still never
// followed by an error document. Proxies in front of streamed relays should
// therefore use HTTP/1.1 upstream. Use it for opaque bodies; streams whose
// clients parse an appended error, such as chat SSE, keep using AbortErr.
func AbortStreamErr(ctx *gin.Context, err error) bool {
	if err != nil && ctx.Writer.Written() {
		logger := gmw.GetLogger(ctx)
		switch {
		case isContextCanceledErr(err):
			logger.Debug("stream canceled after the response was committed", zap.Error(err))
		case errors.Is(err, context.DeadlineExceeded):
			logger.Warn("stream timed out after the response was committed", zap.Error(err))
		default:
			logger.Error("stream failed after the response was committed", zap.Error(err))
		}
		ctx.Abort()
		if abortErr := http.NewResponseController(ctx.Writer).SetWriteDeadline(time.Now()); abortErr != nil {
			logger.Warn("abort committed response", zap.Error(abortErr))
		}
		return true
	}
	if err != nil {
		header := ctx.Writer.Header()
		for _, name := range relayedRepresentationHeaders {
			header.Del(name)
		}
		header.Set("Cache-Control", "no-store")
	}
	return AbortErr(ctx, err)
}
