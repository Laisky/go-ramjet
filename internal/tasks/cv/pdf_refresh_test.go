package cv

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// scriptedContentStore is a versioned content store whose reads can fail on demand, or hold one read, taken
// before the hold, until the test releases it.
type scriptedContentStore struct {
	versionedContentStore
	failLoads atomic.Bool
	holdNext  atomic.Pointer[chan struct{}]
	loads     atomic.Int64
}

// Load counts the read, fails while failLoads is set, and otherwise returns the content current at the time of the
// read, waiting first for a pending hold to be released when one is set.
func (s *scriptedContentStore) Load(ctx context.Context) (ContentPayload, error) {
	s.loads.Add(1)
	if s.failLoads.Load() {
		return ContentPayload{}, errors.New("content read outage")
	}
	payload, err := s.versionedContentStore.Load(ctx)
	if hold := s.holdNext.Swap(nil); hold != nil {
		select {
		case <-*hold:
		case <-ctx.Done():
		}
	}
	return payload, err
}

// requestCVPDF performs GET /cv/pdf through the handler with logger as the request logger when it is not nil.
// It returns the response status code.
func requestCVPDF(h *handler, logger glog.Logger) int {
	ginCtx, recorder := newCVTestContext(http.MethodGet, "/cv/pdf")
	if logger != nil {
		gmw.SetLogger(ginCtx, logger)
	}
	h.downloadPDF(ginCtx)
	return recorder.Code
}

// TestDownloadPDFRefreshLoadFailureBacksOff verifies that while the CV content cannot be read, spaced public
// downloads of a missing PDF read and log the failure once per backoff window, at warning level, instead of once
// per request, and that a download after the window tries again.
func TestDownloadPDFRefreshLoadFailureBacksOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newBlockingPDFRenderer()
		defer renderer.releaseAll()
		logger, entries := newObservedLogger(t)
		content := &scriptedContentStore{}
		content.failLoads.Store(true)
		service, pdfStore, _ := newFlightTestPDFService(t, renderer, content)
		h := &handler{store: content, pdfStore: pdfStore, pdfService: service}

		const downloads = 10
		spacing := pdfRefreshFailureBackoff / (downloads + 1)
		for range downloads {
			require.Equal(t, http.StatusNotFound, requestCVPDF(h, logger))
			synctest.Wait()
			time.Sleep(spacing)
		}
		require.Equal(t, int64(1), content.loads.Load(), "downloads within the backoff window must not read the content again")
		require.Equal(t, 1, entriesMentioning(entries, "content read outage"), "the failure must be logged once per window")
		for _, entry := range entries.All() {
			require.Less(t, entry.Level, zapcore.ErrorLevel, "a refresh read failure is a warning: %s", entry.Message)
		}

		time.Sleep(pdfRefreshFailureBackoff)
		require.Equal(t, http.StatusNotFound, requestCVPDF(h, logger))
		synctest.Wait()
		require.Equal(t, int64(2), content.loads.Load(), "a download after the backoff window must retry")
		require.Equal(t, 2, entriesMentioning(entries, "content read outage"))
		require.Zero(t, renderer.calls.Load())
	})
}
