package cv

import (
	"context"
	"net/http"
	"sync"
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

// TestFailedFinalReadCannotOverwriteNewerSave verifies a write whose final currency read fails does not upload over
// the PDF of a newer save made in this process, whether that older write came from a public refresh or an earlier
// save, and that the newer save's PDF stays stored.
func TestFailedFinalReadCannotOverwriteNewerSave(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		// start begins the older write of "# X" and returns a check of its outcome, run once every write ended.
		start func(t *testing.T, h *handler, wg *sync.WaitGroup) func(t *testing.T)
	}{
		{name: "refresh", start: func(t *testing.T, h *handler, _ *sync.WaitGroup) func(t *testing.T) {
			t.Helper()
			require.Equal(t, http.StatusNotFound, requestCVPDF(h, nil))
			return func(*testing.T) {}
		}},
		{name: "save", start: func(t *testing.T, h *handler, wg *sync.WaitGroup) func(t *testing.T) {
			t.Helper()
			var code int
			var err error
			wg.Go(func() { code, err = saveCVContent(context.Background(), h, nil, "# X") })
			return func(t *testing.T) {
				t.Helper()
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, code, "the older save succeeds even when its upload is skipped")
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				renderer := newGatedPDFRenderer()
				defer renderer.openAll()
				content := &scriptedContentStore{versionedContentStore: versionedContentStore{current: "# X"}}
				service, pdfStore, _ := newFlightTestPDFService(t, renderer, content)
				h := &handler{store: content, pdfStore: pdfStore, pdfService: service}

				var wg sync.WaitGroup
				checkOlder := tc.start(t, h, &wg)
				synctest.Wait()
				require.Equal(t, []string{"# X"}, renderer.startedContents())

				var saveCode int
				var saveErr error
				wg.Go(func() { saveCode, saveErr = saveCVContent(context.Background(), h, nil, "# Y") })
				synctest.Wait()
				require.Equal(t, []string{"# X", "# Y"}, renderer.startedContents())
				renderer.open(1)
				synctest.Wait()
				require.Equal(t, "%PDF-1.4 # Y", storedPDF(t, pdfStore))

				content.failLoads.Store(true)
				renderer.open(0)
				wg.Wait()
				synctest.Wait()
				require.NoError(t, saveErr)
				require.Equal(t, http.StatusOK, saveCode)
				checkOlder(t)
				require.Equal(t, "%PDF-1.4 # Y", storedPDF(t, pdfStore), "an older write must not overwrite the newer save's PDF")
			})
		})
	}
}

// TestStaleRefreshDoesNotDisplaceLatestSave verifies a refresh that read the content just before a save replaced it
// does not make that save look superseded: when the save's final currency read fails, the save, being the latest
// save in this process, still uploads its PDF.
func TestStaleRefreshDoesNotDisplaceLatestSave(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newGatedPDFRenderer()
		defer renderer.openAll()
		content := &scriptedContentStore{versionedContentStore: versionedContentStore{current: "# X"}}
		service, pdfStore, _ := newFlightTestPDFService(t, renderer, content)
		h := &handler{store: content, pdfStore: pdfStore, pdfService: service}

		hold := make(chan struct{})
		content.holdNext.Store(&hold)
		require.Equal(t, http.StatusNotFound, requestCVPDF(h, nil))
		synctest.Wait()

		var saveCode int
		var saveErr error
		var wg sync.WaitGroup
		wg.Go(func() { saveCode, saveErr = saveCVContent(context.Background(), h, nil, "# Y") })
		synctest.Wait()
		require.Equal(t, []string{"# Y"}, renderer.startedContents())

		// The refresh now resumes with the content it read before the save, and finds it superseded.
		close(hold)
		synctest.Wait()
		require.Equal(t, []string{"# Y"}, renderer.startedContents())

		content.failLoads.Store(true)
		renderer.open(0)
		wg.Wait()
		require.NoError(t, saveErr)
		require.Equal(t, http.StatusOK, saveCode)
		require.Equal(t, "%PDF-1.4 # Y", storedPDF(t, pdfStore), "the latest save must still upload its PDF")
	})
}
