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

// scriptedContentStore is a versioned content store whose reads can fail on demand, and which can hold one read,
// taken before the hold, or one save, committed before the hold, until the test releases it.
type scriptedContentStore struct {
	versionedContentStore
	failLoads    atomic.Bool
	holdNext     atomic.Pointer[chan struct{}]
	holdNextSave atomic.Pointer[chan struct{}]
	loads        atomic.Int64
}

// Save commits content and then, when a save hold is pending, waits for its release before returning, like a
// handler descheduled between committing the content and rendering its PDF.
func (s *scriptedContentStore) Save(ctx context.Context, content string) (ContentPayload, error) {
	payload, err := s.versionedContentStore.Save(ctx, content)
	if hold := s.holdNextSave.Swap(nil); hold != nil {
		select {
		case <-*hold:
		case <-ctx.Done():
		}
	}
	return payload, err
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

// TestFailedFinalReadFollowsContentCommitOrder verifies that when the content cannot be read before an upload, the
// save whose content the store committed last uploads its PDF, even if an earlier save reaches its render later.
func TestFailedFinalReadFollowsContentCommitOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newGatedPDFRenderer()
		defer renderer.openAll()
		content := &scriptedContentStore{versionedContentStore: versionedContentStore{current: "# Old"}}
		service, pdfStore, _ := newFlightTestPDFService(t, renderer, content)
		h := &handler{store: content, pdfStore: pdfStore, pdfService: service}

		// Save X commits and is then held before it reaches its PDF; save Y is requested meanwhile.
		hold := make(chan struct{})
		content.holdNextSave.Store(&hold)
		codes := make([]int, 2)
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i, text := range []string{"# X", "# Y"} {
			wg.Go(func() { codes[i], errs[i] = saveCVContent(context.Background(), h, nil, text) })
			synctest.Wait()
		}

		content.failLoads.Store(true)
		close(hold)
		synctest.Wait()
		renderer.openAll()
		wg.Wait()
		synctest.Wait()

		for i := range codes {
			require.NoError(t, errs[i])
			require.Equal(t, http.StatusOK, codes[i])
		}
		require.Equal(t, "# Y", content.current)
		require.Equal(t, "%PDF-1.4 # Y", storedPDF(t, pdfStore), "the stored PDF must follow the content committed last")
	})
}

// TestSaveContentWaitHonorsContext verifies a save waiting behind another save's commit gives up when its caller
// leaves, without committing its content.
func TestSaveContentWaitHonorsContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		content := &scriptedContentStore{versionedContentStore: versionedContentStore{current: "# Old"}}
		service, _, _ := newFlightTestPDFService(t, &trackingPDFRenderer{}, content)

		hold := make(chan struct{})
		content.holdNextSave.Store(&hold)
		var firstErr error
		var wg sync.WaitGroup
		wg.Go(func() { _, firstErr = service.SaveContent(context.Background(), "# X") })
		synctest.Wait()

		ctx, cancel := context.WithCancel(context.Background())
		var secondErr error
		wg.Go(func() { _, secondErr = service.SaveContent(ctx, "# Y") })
		synctest.Wait()
		cancel()
		synctest.Wait()
		require.ErrorIs(t, secondErr, context.Canceled)

		close(hold)
		wg.Wait()
		require.NoError(t, firstErr)
		require.Equal(t, "# X", content.current, "a save whose caller left while waiting must not commit")
		require.True(t, service.isLatestSave("# X"))
	})
}

// TestDownloadPDFRefreshStoreFailureIsAWarning verifies that when the render or upload behind a public refresh of a
// missing PDF fails or hangs, the failure is reported once, as the "cv pdf async refresh render failed" warning
// that background refreshes have always logged, and never at error level: the process forwards error-level entries
// to its alert channel, so an anonymous download must not be able to raise an alert.
func TestDownloadPDFRefreshStoreFailureIsAWarning(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		// renderer returns the renderer for the case and a function that releases it when the test ends.
		renderer func() (PDFRenderer, func())
		failPuts bool
		cause    string
	}{
		{name: "render fails", renderer: func() (PDFRenderer, func()) {
			renderer := newGatedPDFRenderer()
			renderer.fail = true
			renderer.openAll()
			return renderer, func() {}
		}, cause: "gated renderer failed"},
		{name: "upload fails", renderer: func() (PDFRenderer, func()) {
			renderer := newGatedPDFRenderer()
			renderer.openAll()
			return renderer, func() {}
		}, failPuts: true, cause: "pdf upload outage"},
		{name: "render hangs", renderer: func() (PDFRenderer, func()) {
			renderer := newBlockingPDFRenderer()
			return renderer, renderer.releaseAll
		}, cause: "blocking renderer stopped"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				renderer, release := tc.renderer()
				defer release()
				logger, entries := newObservedLogger(t)
				content := &versionedContentStore{current: "# CV"}
				service, pdfStore, client := newFlightTestPDFService(t, renderer, content)
				client.failPuts.Store(tc.failPuts)
				h := &handler{store: content, pdfStore: pdfStore, pdfService: service}

				require.Equal(t, http.StatusNotFound, requestCVPDF(h, logger))
				// Every case has ended once the shared timeout has passed.
				time.Sleep(pdfSharedRenderTimeout + time.Second)
				synctest.Wait()
				require.Empty(t, storedPDF(t, pdfStore))
				require.Equal(t, 1, entriesMentioning(entries, tc.cause), "the failure must be reported once")
				for _, entry := range entries.All() {
					require.Less(t, entry.Level, zapcore.ErrorLevel, "a public refresh must not log an alert-level entry: %s", entry.Message)
				}
				warnings := entries.FilterLevelExact(zapcore.WarnLevel).FilterMessage("cv pdf async refresh render failed")
				require.Equal(t, 1, warnings.Len(), "the failure must be reported as the background refresh warning")
			})
		})
	}
}

// slowLoadContentStore is a scripted content store whose every read of the latest content first takes delay, like
// a slow storage backend.
type slowLoadContentStore struct {
	scriptedContentStore
	delay time.Duration
}

// Load waits for delay, or fails when ctx ends first, and then reads like the scripted store.
func (s *slowLoadContentStore) Load(ctx context.Context) (ContentPayload, error) {
	timer := time.NewTimer(s.delay)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		return ContentPayload{}, errors.Wrap(ctx.Err(), "slow content read stopped")
	}
	return s.scriptedContentStore.Load(ctx)
}

// TestDownloadPDFRefreshHoldsUntilHungStoreWorkEnds verifies a refresh whose render hangs keeps later downloads
// from reading the content or rendering again until the shared timeout stops that render, and then backs off like
// any other failed refresh, even though the refresh's own deadline, which started before its content read, passes
// first.
func TestDownloadPDFRefreshHoldsUntilHungStoreWorkEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const readDelay = time.Second
		renderer := newBlockingPDFRenderer()
		defer renderer.releaseAll()
		content := &slowLoadContentStore{
			scriptedContentStore: scriptedContentStore{versionedContentStore: versionedContentStore{current: "# CV"}},
			delay:                readDelay,
		}
		service, pdfStore, client := newFlightTestPDFService(t, renderer, content)
		h := &handler{store: content, pdfStore: pdfStore, pdfService: service}
		sleepUntil := func(at time.Time) { time.Sleep(time.Until(at)) }

		started := time.Now()
		require.Equal(t, http.StatusNotFound, requestCVPDF(h, nil))
		// The refresh read, the store work's two currency reads and the start of its render take three reads.
		sleepUntil(started.Add(10 * readDelay))
		synctest.Wait()
		require.Equal(t, int64(1), renderer.calls.Load())
		loads := content.loads.Load()

		// The store work started one read after the refresh, so its render is stopped one read after the refresh's
		// own deadline. Downloads before and just after that must neither read the content nor render again.
		for _, at := range []time.Duration{pdfSharedRenderTimeout + readDelay/2, pdfSharedRenderTimeout + 2*readDelay} {
			sleepUntil(started.Add(at))
			require.Equal(t, http.StatusNotFound, requestCVPDF(h, nil))
			synctest.Wait()
		}
		require.Equal(t, int64(1), renderer.canceled.Load(), "the hung render must be stopped by the shared timeout")
		require.Equal(t, loads, content.loads.Load(), "no download may read the content while the hung refresh holds or backs off")
		require.Equal(t, int64(1), renderer.calls.Load())

		require.Zero(t, client.puts.Load())

		// Once the backoff window ends, a download retries, and the retry stores the PDF when rendering works again.
		renderer.releaseAll()
		retryAt := started.Add(pdfSharedRenderTimeout + readDelay + pdfRefreshFailureBackoff + time.Second)
		sleepUntil(retryAt)
		require.Equal(t, http.StatusNotFound, requestCVPDF(h, nil))
		sleepUntil(retryAt.Add(10 * readDelay))
		synctest.Wait()
		require.Greater(t, content.loads.Load(), loads, "a download after the backoff window must retry")
		require.Equal(t, int64(2), renderer.calls.Load())
		require.Equal(t, "%PDF-1.4 # CV", storedPDF(t, pdfStore))
	})
}
