package cv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// versionedContentStore is a content repository whose Save replaces the current content, like the S3 store.
type versionedContentStore struct {
	mu      sync.Mutex
	current string
}

// Load returns the current content.
func (s *versionedContentStore) Load(context.Context) (ContentPayload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ContentPayload{Content: s.current}, nil
}

// Save replaces the current content and returns it.
func (s *versionedContentStore) Save(_ context.Context, content string) (ContentPayload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = content
	return ContentPayload{Content: content}, nil
}

// ListHistory returns no history.
func (s *versionedContentStore) ListHistory(context.Context, int) ([]ContentHistoryEntry, error) {
	return nil, nil
}

// LoadVersion reports that no historical version exists.
func (s *versionedContentStore) LoadVersion(context.Context, string) (ContentPayload, error) {
	return ContentPayload{}, ErrContentVersionNotFound
}

// gatedPDFRenderer holds the i-th render until gate i opens, and records started and canceled renders.
type gatedPDFRenderer struct {
	mu         sync.Mutex
	gates      map[int]chan struct{}
	opened     map[int]bool
	started    []string
	canceled   []string
	fail       bool
	panicValue any
}

// newGatedPDFRenderer returns a renderer whose renders wait for their own gate.
func newGatedPDFRenderer() *gatedPDFRenderer {
	return &gatedPDFRenderer{gates: map[int]chan struct{}{}, opened: map[int]bool{}}
}

// gate returns the channel that releases render i, creating it when needed.
func (r *gatedPDFRenderer) gate(i int) chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gates[i] == nil {
		r.gates[i] = make(chan struct{})
	}
	return r.gates[i]
}

// open releases render i; opening a gate twice is a no-op.
func (r *gatedPDFRenderer) open(i int) {
	gate := r.gate(i)
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.opened[i] {
		r.opened[i] = true
		close(gate)
	}
}

// openAll releases every render that has started or may start next.
func (r *gatedPDFRenderer) openAll() {
	for i := range 16 {
		r.open(i)
	}
}

// startedContents returns the contents of renders that started, in order.
func (r *gatedPDFRenderer) startedContents() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.started...)
}

// canceledContents returns the contents of renders stopped by their context.
func (r *gatedPDFRenderer) canceledContents() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.canceled...)
}

// Render waits for its gate or ctx, then panics, fails or returns a deterministic PDF payload as configured.
func (r *gatedPDFRenderer) Render(ctx context.Context, content string) ([]byte, error) {
	r.mu.Lock()
	index := len(r.started)
	r.started = append(r.started, content)
	r.mu.Unlock()
	select {
	case <-r.gate(index):
	case <-ctx.Done():
		r.mu.Lock()
		r.canceled = append(r.canceled, content)
		r.mu.Unlock()
		return nil, errors.Wrap(ctx.Err(), "gated render stopped")
	}
	if r.panicValue != nil {
		panic(r.panicValue)
	}
	if r.fail {
		return nil, errors.New("gated renderer failed")
	}
	return []byte("%PDF-1.4 " + content), nil
}

// newObservedLogger returns a debug logger whose entries the test can inspect.
func newObservedLogger(t *testing.T) (glog.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, entries := observer.New(zap.DebugLevel)
	logger, err := glog.NewWithName("cv-pdf-test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	return logger, entries
}

// entriesMentioning counts log entries whose message or fields mention text.
func entriesMentioning(entries *observer.ObservedLogs, text string) int {
	count := 0
	for _, entry := range entries.All() {
		if strings.Contains(entry.Message, text) || strings.Contains(fmt.Sprint(entry.ContextMap()), text) {
			count++
		}
	}
	return count
}

// saveCVContent performs PUT /cv/content through the handler with ctx as the request context.
// It returns the response status code, or an error when the request body cannot be encoded.
func saveCVContent(ctx context.Context, h *handler, logger glog.Logger, content string) (int, error) {
	body, err := json.Marshal(contentRequest{Content: content})
	if err != nil {
		return 0, errors.Wrap(err, "encode content request")
	}
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/cv/content", bytes.NewReader(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = request
	if logger != nil {
		gmw.SetLogger(ginCtx, logger)
	}
	h.saveContent(ginCtx)
	return recorder.Code, nil
}

// storedPDF returns the stored PDF bytes as text, or an empty string when no PDF is stored.
func storedPDF(t *testing.T, pdfStore *S3PDFStore) string {
	t.Helper()
	reader, _, err := pdfStore.Open(context.Background())
	if errors.Is(err, ErrObjectNotFound) {
		return ""
	}
	require.NoError(t, err)
	defer func() { require.NoError(t, reader.Close()) }()
	payload, err := io.ReadAll(reader)
	require.NoError(t, err)
	return string(payload)
}

// TestPDFServicePanicReturnsErrorToJoinedCallers verifies a panicking render becomes an error, carrying the panic
// value and its stack, for every joined caller instead of crashing the process.
func TestPDFServicePanicReturnsErrorToJoinedCallers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newGatedPDFRenderer()
		renderer.panicValue = "goldmark exploded"
		defer renderer.openAll()
		service, _, _ := newFlightTestPDFService(t, renderer, &versionedContentStore{current: "# Panic"})

		const callers = 3
		errs := make([]error, callers)
		var wg sync.WaitGroup
		for i := range callers {
			wg.Go(func() {
				_, errs[i] = service.Render(context.Background(), "# Panic")
			})
		}
		synctest.Wait()
		renderer.open(0)
		wg.Wait()
		require.Len(t, renderer.startedContents(), 1)
		for i := range callers {
			require.ErrorContains(t, errs[i], "panic")
			require.ErrorContains(t, errs[i], "goldmark exploded")
			require.Contains(t, fmt.Sprintf("%+v", errs[i]), "gatedPDFRenderer", "the error must carry the panic stack")
		}
		require.Error(t, renderAndStoreErr(service, "# Panic"))
	})
}

// TestPDFServiceSaveABADoesNotStoreStalePDF verifies that with saves A, B and A again, the third save does not join
// the stale first flight and the stored PDF always matches the latest content, whatever order renders finish in.
func TestPDFServiceSaveABADoesNotStoreStalePDF(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newGatedPDFRenderer()
		defer renderer.openAll()
		content := &versionedContentStore{}
		service, pdfStore, _ := newFlightTestPDFService(t, renderer, content)
		h := &handler{store: content, pdfStore: pdfStore, pdfService: service}

		codes := make([]int, 3)
		errs := make([]error, 3)
		var wg sync.WaitGroup
		for i, text := range []string{"# A", "# B", "# A"} {
			wg.Go(func() { codes[i], errs[i] = saveCVContent(context.Background(), h, nil, text) })
			synctest.Wait()
		}
		require.Equal(t, 3, inFlightWrites(service), "the third save must not join the first save's stale write")
		renderer.open(0) // the first A render finishes while A is current again
		synctest.Wait()
		renderer.open(1) // the B render finishes last among the first two
		synctest.Wait()
		renderer.open(2) // the third save's own render, when it has one
		wg.Wait()
		synctest.Wait()

		for i := range codes {
			require.NoError(t, errs[i])
			require.Equal(t, http.StatusOK, codes[i])
		}
		require.Equal(t, "# A", content.current)
		require.Equal(t, "%PDF-1.4 # A", storedPDF(t, pdfStore), "the stored PDF must match the latest content")
	})
}

// TestPDFServiceAbandonedSaveDoesNotOverwriteNewerPDF verifies a save whose client left cannot upload its PDF over
// a newer save that finished first.
func TestPDFServiceAbandonedSaveDoesNotOverwriteNewerPDF(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newGatedPDFRenderer()
		defer renderer.openAll()
		content := &versionedContentStore{}
		service, pdfStore, _ := newFlightTestPDFService(t, renderer, content)
		h := &handler{store: content, pdfStore: pdfStore, pdfService: service}

		callerCtx, cancelCaller := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		var errA, errB error
		var codeB int
		wg.Go(func() { _, errA = saveCVContent(callerCtx, h, nil, "# A") })
		synctest.Wait()
		cancelCaller()
		synctest.Wait()
		wg.Go(func() { codeB, errB = saveCVContent(context.Background(), h, nil, "# B") })
		synctest.Wait()
		renderer.open(1)
		synctest.Wait()
		require.Equal(t, "%PDF-1.4 # B", storedPDF(t, pdfStore))
		renderer.open(0)
		wg.Wait()
		synctest.Wait()

		require.NoError(t, errA)
		require.NoError(t, errB)
		require.Equal(t, http.StatusOK, codeB)
		require.Equal(t, "%PDF-1.4 # B", storedPDF(t, pdfStore), "an abandoned older save must not overwrite the newer PDF")
	})
}

// TestPDFServiceAbandonedPreviewsReleaseRenderSlots verifies previews stop once every caller has left, so they no
// longer hold render slots that a later save needs.
func TestPDFServiceAbandonedPreviewsReleaseRenderSlots(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newGatedPDFRenderer()
		defer renderer.openAll()
		service, _, _ := newFlightTestPDFService(t, renderer, &versionedContentStore{current: "# S"})

		var wg sync.WaitGroup
		cancels := make([]context.CancelFunc, 2)
		previewErrs := make([]error, 2)
		for i, text := range []string{"# P1", "# P2"} {
			previewCtx, cancel := context.WithCancel(context.Background())
			cancels[i] = cancel
			wg.Go(func() { _, previewErrs[i] = service.Render(previewCtx, text) })
			synctest.Wait()
		}
		var saveErr error
		wg.Go(func() { saveErr = renderAndStoreErr(service, "# S") })
		synctest.Wait()
		require.Equal(t, []string{"# P1", "# P2"}, renderer.startedContents(), "the save waits for a render slot")

		cancels[0]()
		cancels[1]()
		synctest.Wait()
		require.ElementsMatch(t, []string{"# P1", "# P2"}, renderer.canceledContents())
		require.Equal(t, []string{"# P1", "# P2", "# S"}, renderer.startedContents(), "the save must get a freed slot")
		renderer.open(2)
		wg.Wait()
		require.NoError(t, saveErr)
		require.ErrorIs(t, previewErrs[0], context.Canceled)
		require.ErrorIs(t, previewErrs[1], context.Canceled)
	})
}

// TestPDFServiceAbandonedQueuedPreviewNeverRenders verifies a preview whose caller left while it waited for a
// render slot is not rendered later.
func TestPDFServiceAbandonedQueuedPreviewNeverRenders(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newGatedPDFRenderer()
		defer renderer.openAll()
		service, _, _ := newFlightTestPDFService(t, renderer, &versionedContentStore{})

		var wg sync.WaitGroup
		errs := make([]error, 3)
		for i, text := range []string{"# P1", "# P2"} {
			wg.Go(func() { _, errs[i] = service.Render(context.Background(), text) })
			synctest.Wait()
		}
		queuedCtx, cancelQueued := context.WithCancel(context.Background())
		wg.Go(func() { _, errs[2] = service.Render(queuedCtx, "# Queued") })
		synctest.Wait()
		cancelQueued()
		synctest.Wait()
		renderer.open(0)
		renderer.open(1)
		wg.Wait()
		synctest.Wait()
		require.Equal(t, []string{"# P1", "# P2"}, renderer.startedContents(), "an abandoned preview must not render")
		require.NoError(t, errs[0])
		require.NoError(t, errs[1])
		require.ErrorIs(t, errs[2], context.Canceled)
	})
}

// TestPDFServiceAbandonedSaveFailureLoggedOnce verifies a save keeps running after its caller left and that its
// failure, which no caller can receive any more, is logged exactly once.
func TestPDFServiceAbandonedSaveFailureLoggedOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newGatedPDFRenderer()
		renderer.fail = true
		defer renderer.openAll()
		logger, entries := newObservedLogger(t)
		service, _, _ := newFlightTestPDFService(t, renderer, &versionedContentStore{current: "# F"})

		callerCtx, cancelCaller := context.WithCancel(gmw.SetLogger(context.Background(), logger))
		var saveErr error
		var wg sync.WaitGroup
		wg.Go(func() { _, saveErr = service.RenderAndStore(callerCtx, "# F") })
		synctest.Wait()
		cancelCaller()
		wg.Wait()
		require.ErrorIs(t, saveErr, context.Canceled)
		require.Zero(t, entriesMentioning(entries, "gated renderer failed"))

		renderer.open(0)
		synctest.Wait()
		require.Empty(t, renderer.canceledContents(), "a save keeps running after its caller left")
		require.Equal(t, 1, entriesMentioning(entries, "gated renderer failed"), "the orphaned failure must be logged once")
	})
}

// TestDownloadPDFRefreshFailureLoggedOnce verifies concurrent background refreshes that share one failing render
// log the failure once, and that keep the public 404 responses unchanged.
func TestDownloadPDFRefreshFailureLoggedOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newGatedPDFRenderer()
		renderer.fail = true
		defer renderer.openAll()
		logger, entries := newObservedLogger(t)
		content := &versionedContentStore{current: "# CV"}
		service, pdfStore, _ := newFlightTestPDFService(t, renderer, content)
		h := &handler{store: content, pdfStore: pdfStore, pdfService: service}

		for range 3 {
			ginCtx, recorder := newCVTestContext(http.MethodGet, "/cv/pdf")
			gmw.SetLogger(ginCtx, logger)
			h.downloadPDF(ginCtx)
			require.Equal(t, http.StatusNotFound, recorder.Code)
		}
		synctest.Wait()
		renderer.open(0)
		synctest.Wait()
		require.Len(t, renderer.startedContents(), 1)
		require.Equal(t, 1, entriesMentioning(entries, "gated renderer failed"))
	})
}

// TestDownloadPDFSlowRefreshSucceedsWithoutTimeoutNoise verifies a background refresh slower than one minute but
// within the shared cap stores the PDF and logs no failure.
func TestDownloadPDFSlowRefreshSucceedsWithoutTimeoutNoise(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newGatedPDFRenderer()
		defer renderer.openAll()
		logger, entries := newObservedLogger(t)
		content := &versionedContentStore{current: "# CV"}
		service, pdfStore, _ := newFlightTestPDFService(t, renderer, content)
		h := &handler{store: content, pdfStore: pdfStore, pdfService: service}

		ginCtx, recorder := newCVTestContext(http.MethodGet, "/cv/pdf")
		gmw.SetLogger(ginCtx, logger)
		h.downloadPDF(ginCtx)
		require.Equal(t, http.StatusNotFound, recorder.Code)
		synctest.Wait()
		time.Sleep(80 * time.Second)
		renderer.open(0)
		synctest.Wait()
		require.Equal(t, "%PDF-1.4 # CV", storedPDF(t, pdfStore))
		require.Zero(t, entriesMentioning(entries, "deadline exceeded"))
		require.Zero(t, entriesMentioning(entries, "pdf async refresh render failed"))
		require.Zero(t, entriesMentioning(entries, "shared cv pdf work failed"))
	})
}

// TestPDFServicesWithDifferentStoresDoNotShareWrites verifies two services that store into different PDF objects
// never join each other's work, even for identical content.
func TestPDFServicesWithDifferentStoresDoNotShareWrites(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newGatedPDFRenderer()
		defer renderer.openAll()
		first, firstStore, _ := newFlightTestPDFService(t, renderer, &versionedContentStore{current: "# X"})
		second, secondStore, _ := newFlightTestPDFService(t, renderer, &versionedContentStore{current: "# X"})
		shareProcessRenderLimit(first, second)

		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i, service := range []*PDFService{first, second} {
			wg.Go(func() { errs[i] = renderAndStoreErr(service, "# X") })
			synctest.Wait()
		}
		renderer.open(0)
		renderer.open(1)
		wg.Wait()
		require.NoError(t, errs[0])
		require.NoError(t, errs[1])
		require.Equal(t, "%PDF-1.4 # X", storedPDF(t, firstStore))
		require.Equal(t, "%PDF-1.4 # X", storedPDF(t, secondStore), "each service must write its own store")
	})
}

// inFlightWrites counts the store flights a service is currently running.
func inFlightWrites(service *PDFService) int {
	service.flights.mu.Lock()
	defer service.flights.mu.Unlock()
	count := 0
	for key := range service.flights.flights {
		if strings.HasPrefix(key, "store:") {
			count++
		}
	}
	return count
}

// TestPDFServiceWriteKeyGenerations verifies repeated saves of one content share a generation, while a save of
// content that another save superseded starts a newer generation, and that looking up a key records nothing.
func TestPDFServiceWriteKeyGenerations(t *testing.T) {
	t.Parallel()
	service, _, _ := newFlightTestPDFService(t, &trackingPDFRenderer{}, &fakeContentStore{})
	service.recordSave("# A")
	first := service.storeKey("# A")
	service.recordSave("# A")
	require.Equal(t, first, service.storeKey("# A"))
	service.recordSave("# B")
	second := service.storeKey("# B")
	require.NotEqual(t, first, second)
	require.False(t, service.isLatestSave("# A"))
	require.Equal(t, second, service.storeKey("# B"), "a key lookup for other content must not record a save")
	service.recordSave("# A")
	third := service.storeKey("# A")
	require.NotEqual(t, first, third, "a write must not join a generation that a newer save superseded")
	require.True(t, service.isLatestSave("# A"))
}
