package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// newTestGinContext creates a gin context with a file-backed logger.
//
// newTestGinContext returns the gin context, response recorder, log file path, and logger instance.
func newTestGinContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder, string, *glog.LoggerT) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	logFile := filepath.Join(t.TempDir(), "test-log.json")
	logger, err := glog.New(
		glog.WithName("test"),
		glog.WithLevel(glog.LevelDebug),
		glog.WithEncoding(glog.EncodingJSON),
		glog.WithOutputPaths([]string{logFile}),
		glog.WithErrorOutputPaths([]string{logFile}),
	)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
	gmw.SetLogger(ctx, logger)

	return ctx, w, logFile, logger
}

// readLogFile returns all log output written to the given path.
func readLogFile(t *testing.T, path string) string {
	t.Helper()

	bs, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(bs)
}

// TestAbortErr_ContextCanceled ensures cancellations are treated as expected behavior.
func TestAbortErr_ContextCanceled(t *testing.T) {
	t.Parallel()
	req := require.New(t)

	ctx, w, logFile, logger := newTestGinContext(t)

	ok := AbortErr(ctx, context.Canceled)
	req.True(ok)
	req.Equal(httpStatusClientClosedRequest, w.Code)
	req.NoError(logger.Sync())

	out := readLogFile(t, logFile)
	req.Contains(out, "request canceled")
	req.NotContains(out, "chat abort")
}

// TestAbortErr_DeadlineExceeded ensures timeouts are mapped to 504 and logged as WARN.
func TestAbortErr_DeadlineExceeded(t *testing.T) {
	t.Parallel()
	req := require.New(t)

	ctx, w, logFile, logger := newTestGinContext(t)

	ok := AbortErr(ctx, context.DeadlineExceeded)
	req.True(ok)
	req.Equal(http.StatusGatewayTimeout, w.Code)
	req.NoError(logger.Sync())

	out := readLogFile(t, logFile)
	req.Contains(out, "request timeout")
	req.NotContains(out, "chat abort")
}

// TestAbortErr_Default ensures real errors keep existing behavior: ERROR log + HTTP 400.
func TestAbortErr_Default(t *testing.T) {
	t.Parallel()
	req := require.New(t)

	ctx, w, logFile, logger := newTestGinContext(t)

	ok := AbortErr(ctx, nil)
	req.False(ok)

	boom := errors.New("boom")
	ok = AbortErr(ctx, boom)
	req.True(ok)
	req.Equal(http.StatusBadRequest, w.Code)
	req.NoError(logger.Sync())

	out := readLogFile(t, logFile)
	req.Contains(out, "chat abort")
	// Ensure we did log an error-level entry for a real error.
	req.True(strings.Contains(out, "\"level\":\"error\"") || strings.Contains(out, "\"level\":\"ERROR\""))
}
