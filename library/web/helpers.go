package web

import (
	"context"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
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
