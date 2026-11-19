package cv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/semaphore"
)

// blockingPDFRenderer is a slow PDF renderer that holds every render until release is closed or ctx ends.
type blockingPDFRenderer struct {
	release     chan struct{}
	releaseOnce sync.Once
	calls       atomic.Int64
	inFlight    atomic.Int64
	maxInFlight atomic.Int64
	canceled    atomic.Int64
}

// newBlockingPDFRenderer returns a renderer whose renders wait for an explicit release.
func newBlockingPDFRenderer() *blockingPDFRenderer {
	return &blockingPDFRenderer{release: make(chan struct{})}
}

// releaseAll lets every pending and future render finish; it is safe to call more than once.
func (r *blockingPDFRenderer) releaseAll() {
	r.releaseOnce.Do(func() { close(r.release) })
}

// Render records call and concurrency counts, waits for release or ctx, and returns a deterministic PDF payload.
func (r *blockingPDFRenderer) Render(ctx context.Context, content string) ([]byte, error) {
	r.calls.Add(1)
	current := r.inFlight.Add(1)
	defer r.inFlight.Add(-1)
	for {
		seen := r.maxInFlight.Load()
		if current <= seen || r.maxInFlight.CompareAndSwap(seen, current) {
			break
		}
	}
	select {
	case <-r.release:
		return []byte("%PDF-1.4 " + content), nil
	case <-ctx.Done():
		r.canceled.Add(1)
		return nil, errors.Wrap(ctx.Err(), "blocking renderer stopped")
	}
}

// countingS3Client counts PDF uploads while delegating storage to the in-memory fake client, and fails uploads
// while failPuts is set.
type countingS3Client struct {
	*fakeS3Client
	puts     atomic.Int64
	failPuts atomic.Bool
}

// PutObject counts the upload and stores it through the fake client, or fails it while failPuts is set.
func (c *countingS3Client) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	c.puts.Add(1)
	if c.failPuts.Load() {
		return minio.UploadInfo{}, errors.New("pdf upload outage")
	}
	return c.fakeS3Client.PutObject(ctx, bucketName, objectName, reader, objectSize, opts)
}

// newFlightTestPDFService builds a PDF service around renderer and an in-memory store for render-flight tests,
// with its own production-sized coordinator so concurrent tests never share render slots or flights.
func newFlightTestPDFService(t *testing.T, renderer PDFRenderer, content ContentRepository) (*PDFService, *S3PDFStore, *countingS3Client) {
	t.Helper()
	client := &countingS3Client{fakeS3Client: newFakeS3Client()}
	pdfStore, err := NewS3PDFStore(client, "bucket", "cv.pdf")
	require.NoError(t, err)
	service, err := NewPDFService(renderer, pdfStore, content)
	require.NoError(t, err)
	require.Same(t, pdfRenderSlots, service.slots, "production services share the process-wide render limit")
	// A test-local limit of the production size keeps concurrent tests from sharing render slots.
	service.slots = semaphore.NewWeighted(pdfMaxConcurrentRenders)
	return service, pdfStore, client
}

// TestDownloadPDFMissingDeduplicatesConcurrentRefreshes verifies concurrent public downloads of a missing PDF keep
// their 404 responses but start exactly one render and one upload for the stored document.
func TestDownloadPDFMissingDeduplicatesConcurrentRefreshes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newBlockingPDFRenderer()
		defer renderer.releaseAll()
		store := &fakeContentStore{payload: ContentPayload{Content: "# CV\nbody"}}
		service, pdfStore, client := newFlightTestPDFService(t, renderer, store)
		h := &handler{store: store, pdfStore: pdfStore, pdfService: service}

		const requests = 8
		codes := make([]int, requests)
		bodies := make([]int, requests)
		var wg sync.WaitGroup
		for i := range requests {
			wg.Go(func() {
				ctx, recorder := newCVTestContext(http.MethodGet, "/cv/pdf?ts=1")
				h.downloadPDF(ctx)
				codes[i], bodies[i] = recorder.Code, recorder.Body.Len()
			})
		}
		wg.Wait()
		for i := range requests {
			require.Equal(t, http.StatusNotFound, codes[i])
			require.Zero(t, bodies[i])
		}
		synctest.Wait()
		require.Equal(t, int64(1), renderer.calls.Load(), "concurrent refreshes of one document must share one render")

		renderer.releaseAll()
		synctest.Wait()
		require.Equal(t, int64(1), renderer.calls.Load())
		require.Equal(t, int64(1), client.puts.Load(), "the shared render must be stored once")
		reader, _, err := pdfStore.Open(context.Background())
		require.NoError(t, err)
		stored, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		require.Equal(t, "%PDF-1.4 # CV\nbody", string(stored))
	})
}

// TestPDFServiceBoundsConcurrentRenders verifies distinct documents still render once each while at most two
// renders run at the same time.
func TestPDFServiceBoundsConcurrentRenders(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newBlockingPDFRenderer()
		defer renderer.releaseAll()
		service, _, _ := newFlightTestPDFService(t, renderer, &fakeContentStore{})

		const documents = 6
		payloads := make([][]byte, documents)
		errs := make([]error, documents)
		var wg sync.WaitGroup
		for i := range documents {
			wg.Go(func() {
				payloads[i], errs[i] = service.Render(context.Background(), fmt.Sprintf("# CV %d", i))
			})
		}
		synctest.Wait()
		require.Equal(t, int64(pdfMaxConcurrentRenders), renderer.inFlight.Load(), "only two renders may hold the browser at once")

		renderer.releaseAll()
		wg.Wait()
		require.Equal(t, int64(pdfMaxConcurrentRenders), renderer.maxInFlight.Load())
		require.Equal(t, int64(documents), renderer.calls.Load())
		for i := range documents {
			require.NoError(t, errs[i])
			require.Equal(t, fmt.Sprintf("%%PDF-1.4 # CV %d", i), string(payloads[i]))
		}
	})
}

// TestPDFServiceSharedRenderSurvivesCallerCancellation verifies callers that join an in-flight render receive the
// same bytes, and that the first caller's cancellation returns only to that caller without stopping the render.
func TestPDFServiceSharedRenderSurvivesCallerCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newBlockingPDFRenderer()
		defer renderer.releaseAll()
		service, _, _ := newFlightTestPDFService(t, renderer, &fakeContentStore{})
		const content = "# Shared CV"

		leaderCtx, cancelLeader := context.WithCancel(context.Background())
		var leaderErr error
		var wg sync.WaitGroup
		wg.Go(func() {
			_, leaderErr = service.Render(leaderCtx, content)
		})
		synctest.Wait()

		const followers = 3
		payloads := make([][]byte, followers)
		errs := make([]error, followers)
		for i := range followers {
			wg.Go(func() {
				payloads[i], errs[i] = service.Render(context.Background(), content)
			})
		}
		synctest.Wait()
		cancelLeader()
		synctest.Wait()
		require.ErrorIs(t, leaderErr, context.Canceled)
		require.Equal(t, int64(0), renderer.canceled.Load(), "a caller's cancellation must not stop the shared render")

		renderer.releaseAll()
		wg.Wait()
		require.Equal(t, int64(1), renderer.calls.Load())
		for i := range followers {
			require.NoError(t, errs[i])
			require.Equal(t, "%PDF-1.4 "+content, string(payloads[i]))
		}
		require.False(t, &payloads[0][0] == &payloads[1][0], "joined callers must not share one mutable slice")
		require.True(t, bytes.Equal(payloads[0], payloads[1]))
	})
}

// TestPDFServiceSharedRenderIsBounded verifies a save that keeps running after its caller left is stopped at the
// shared timeout, so detached work cannot run forever, and that the timeout is logged once.
func TestPDFServiceSharedRenderIsBounded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newBlockingPDFRenderer()
		defer renderer.releaseAll()
		logger, entries := newObservedLogger(t)
		service, _, _ := newFlightTestPDFService(t, renderer, &fakeContentStore{payload: ContentPayload{Content: "# Hung CV"}})

		started := time.Now()
		callerCtx, cancelCaller := context.WithCancel(gmw.SetLogger(context.Background(), logger))
		var callerErr error
		var wg sync.WaitGroup
		wg.Go(func() {
			_, callerErr = service.RenderAndStore(callerCtx, "# Hung CV")
		})
		synctest.Wait()
		cancelCaller()
		wg.Wait()
		require.ErrorIs(t, callerErr, context.Canceled)

		synctest.Wait()
		require.Equal(t, int64(0), renderer.canceled.Load(), "a save keeps running after its caller left")
		time.Sleep(pdfSharedRenderTimeout)
		synctest.Wait()
		require.Equal(t, int64(1), renderer.canceled.Load(), "the detached save must stop at the shared timeout")
		require.Equal(t, pdfSharedRenderTimeout, time.Since(started))
		require.Equal(t, 1, entriesMentioning(entries, "shared cv pdf work failed after every caller left"))
	})
}

// TestPDFServiceRenderAndStoreSharesFailure verifies callers joining a failing render-and-store all receive the
// shared failure and that the failed flight is not cached for later callers.
func TestPDFServiceRenderAndStoreSharesFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := &failingPDFRenderer{release: make(chan struct{})}
		service, _, client := newFlightTestPDFService(t, renderer, &fakeContentStore{payload: ContentPayload{Content: "# Broken CV"}})

		const callers = 4
		errs := make([]error, callers)
		var wg sync.WaitGroup
		for i := range callers {
			wg.Go(func() {
				errs[i] = renderAndStoreErr(service, "# Broken CV")
			})
		}
		synctest.Wait()
		close(renderer.release)
		wg.Wait()
		require.Equal(t, int64(1), renderer.calls.Load())
		for i := range callers {
			require.ErrorContains(t, errs[i], "renderer exploded")
		}
		require.Zero(t, client.puts.Load())

		require.ErrorContains(t, renderAndStoreErr(service, "# Broken CV"), "renderer exploded")
		require.Equal(t, int64(2), renderer.calls.Load(), "a failed flight must not be cached")
	})
}

// failingPDFRenderer fails every render once release is closed.
type failingPDFRenderer struct {
	release chan struct{}
	calls   atomic.Int64
}

// Render records the call, waits for release or ctx, and returns a fixed failure.
func (r *failingPDFRenderer) Render(ctx context.Context, _ string) ([]byte, error) {
	r.calls.Add(1)
	select {
	case <-r.release:
	case <-ctx.Done():
	}
	return nil, errors.New("renderer exploded")
}

// shareProcessRenderLimit makes services share one render limit the way production services share pdfRenderSlots.
func shareProcessRenderLimit(first *PDFService, others ...*PDFService) {
	for _, other := range others {
		other.slots = first.slots
	}
}

// previewCVPDF performs POST /cv/pdf/preview through the handler with ctx as the request context.
// It returns the response status code, or an error when the request body cannot be encoded.
func previewCVPDF(ctx context.Context, h *handler, content string) (int, error) {
	body, err := json.Marshal(contentRequest{Content: content})
	if err != nil {
		return 0, errors.Wrap(err, "encode preview request")
	}
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/cv/pdf/preview", bytes.NewReader(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = request
	h.renderPDFPreview(ginCtx)
	return recorder.Code, nil
}

// TestPDFHandlersWithHungRenderAreBounded verifies the preview and save handlers, which have no deadline of their
// own, answer 504 exactly when the shared render timeout ends a render that never finishes, even for a client that
// keeps waiting, and that the hung render is stopped.
func TestPDFHandlersWithHungRenderAreBounded(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		call func(ctx context.Context, h *handler) (int, error)
	}{
		{name: "preview", call: func(ctx context.Context, h *handler) (int, error) { return previewCVPDF(ctx, h, "# Hung") }},
		{name: "save", call: func(ctx context.Context, h *handler) (int, error) { return saveCVContent(ctx, h, nil, "# Hung") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				renderer := newBlockingPDFRenderer()
				defer renderer.releaseAll()
				content := &versionedContentStore{current: "# Hung"}
				service, pdfStore, client := newFlightTestPDFService(t, renderer, content)
				h := &handler{store: content, pdfStore: pdfStore, pdfService: service}

				started := time.Now()
				code, err := tc.call(context.Background(), h)
				require.NoError(t, err)
				require.Equal(t, http.StatusGatewayTimeout, code)
				require.Equal(t, pdfSharedRenderTimeout, time.Since(started), "the shared timeout is the request's upper bound")
				synctest.Wait()
				require.Equal(t, int64(1), renderer.canceled.Load(), "the hung render must be stopped")
				require.Zero(t, client.puts.Load())
			})
		})
	}
}

// countingLoadContentStore is a versioned content store that counts reads of the latest content.
type countingLoadContentStore struct {
	versionedContentStore
	loads atomic.Int64
}

// Load counts the read and returns the current content.
func (s *countingLoadContentStore) Load(ctx context.Context) (ContentPayload, error) {
	s.loads.Add(1)
	return s.versionedContentStore.Load(ctx)
}

// TestDownloadPDFMissingSharesOneContentLoad verifies that while a background refresh of a missing PDF runs, a
// burst of further public downloads keeps its 404 responses without reading the CV content again or starting
// another render, so the public route cannot multiply content reads.
func TestDownloadPDFMissingSharesOneContentLoad(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newBlockingPDFRenderer()
		defer renderer.releaseAll()
		content := &countingLoadContentStore{versionedContentStore: versionedContentStore{current: "# CV"}}
		service, pdfStore, client := newFlightTestPDFService(t, renderer, content)
		h := &handler{store: content, pdfStore: pdfStore, pdfService: service}
		download := func() int {
			ginCtx, recorder := newCVTestContext(http.MethodGet, "/cv/pdf")
			h.downloadPDF(ginCtx)
			return recorder.Code
		}

		require.Equal(t, http.StatusNotFound, download())
		synctest.Wait()
		require.Equal(t, int64(1), renderer.calls.Load())
		loadsWhileRendering := content.loads.Load()

		const burst = 50
		codes := make([]int, burst)
		var wg sync.WaitGroup
		for i := range burst {
			wg.Go(func() { codes[i] = download() })
		}
		wg.Wait()
		synctest.Wait()
		for i := range burst {
			require.Equal(t, http.StatusNotFound, codes[i])
		}
		require.Equal(t, loadsWhileRendering, content.loads.Load(), "downloads during a running refresh must not read the content again")
		require.Equal(t, int64(1), renderer.calls.Load())

		renderer.releaseAll()
		synctest.Wait()
		require.Equal(t, int64(1), client.puts.Load())
		require.Equal(t, "%PDF-1.4 # CV", storedPDF(t, pdfStore))
	})
}

// gatedFailingLoadStore is a content store whose reads wait for release and then fail, like a slow S3 outage.
type gatedFailingLoadStore struct {
	versionedContentStore
	release chan struct{}
	loads   atomic.Int64
}

// Load counts the read, waits for release or ctx, and fails.
func (s *gatedFailingLoadStore) Load(ctx context.Context) (ContentPayload, error) {
	s.loads.Add(1)
	select {
	case <-s.release:
	case <-ctx.Done():
	}
	return ContentPayload{}, errors.New("content read outage")
}

// TestDownloadPDFRefreshLoadFailureLoggedOnce verifies a burst of public downloads of a missing PDF during a content
// read outage shares one read, logs its failure once instead of once per request, and renders nothing.
func TestDownloadPDFRefreshLoadFailureLoggedOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newBlockingPDFRenderer()
		defer renderer.releaseAll()
		logger, entries := newObservedLogger(t)
		content := &gatedFailingLoadStore{release: make(chan struct{})}
		service, pdfStore, _ := newFlightTestPDFService(t, renderer, content)
		h := &handler{store: content, pdfStore: pdfStore, pdfService: service}

		const burst = 20
		codes := make([]int, burst)
		var wg sync.WaitGroup
		for i := range burst {
			wg.Go(func() {
				ginCtx, recorder := newCVTestContext(http.MethodGet, "/cv/pdf")
				gmw.SetLogger(ginCtx, logger)
				h.downloadPDF(ginCtx)
				codes[i] = recorder.Code
			})
		}
		wg.Wait()
		synctest.Wait()
		for i := range burst {
			require.Equal(t, http.StatusNotFound, codes[i])
		}
		close(content.release)
		synctest.Wait()
		require.Equal(t, int64(1), content.loads.Load(), "the burst must share one content read")
		require.Equal(t, 1, entriesMentioning(entries, "content read outage"), "the failure must be logged once")
		require.Zero(t, renderer.calls.Load())
	})
}

// TestDownloadPDFRefreshJoinsConcurrentSave verifies a background refresh started while a save of the same content
// renders joins that save's render and upload instead of rendering the document a second time.
func TestDownloadPDFRefreshJoinsConcurrentSave(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newBlockingPDFRenderer()
		defer renderer.releaseAll()
		content := &versionedContentStore{current: "# Old"}
		service, pdfStore, client := newFlightTestPDFService(t, renderer, content)
		h := &handler{store: content, pdfStore: pdfStore, pdfService: service}

		var saveCode int
		var saveErr error
		var wg sync.WaitGroup
		wg.Go(func() { saveCode, saveErr = saveCVContent(context.Background(), h, nil, "# New") })
		synctest.Wait()
		ginCtx, recorder := newCVTestContext(http.MethodGet, "/cv/pdf")
		h.downloadPDF(ginCtx)
		require.Equal(t, http.StatusNotFound, recorder.Code)
		synctest.Wait()
		require.Equal(t, int64(1), renderer.calls.Load(), "the refresh must join the save's render")

		renderer.releaseAll()
		wg.Wait()
		synctest.Wait()
		require.NoError(t, saveErr)
		require.Equal(t, http.StatusOK, saveCode)
		require.Equal(t, int64(1), renderer.calls.Load())
		require.Equal(t, int64(1), client.puts.Load())
		require.Equal(t, "%PDF-1.4 # New", storedPDF(t, pdfStore))
	})
}

// TestDownloadPDFHungRefreshLoggedOnce verifies a refresh whose render never finishes is stopped by the shared
// timeout and logged once, even though the refresh and the store work it holds on to both end there.
func TestDownloadPDFHungRefreshLoggedOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		renderer := newBlockingPDFRenderer()
		defer renderer.releaseAll()
		logger, entries := newObservedLogger(t)
		content := &versionedContentStore{current: "# CV"}
		service, pdfStore, client := newFlightTestPDFService(t, renderer, content)
		h := &handler{store: content, pdfStore: pdfStore, pdfService: service}

		ginCtx, recorder := newCVTestContext(http.MethodGet, "/cv/pdf")
		gmw.SetLogger(ginCtx, logger)
		h.downloadPDF(ginCtx)
		require.Equal(t, http.StatusNotFound, recorder.Code)
		synctest.Wait()
		time.Sleep(pdfSharedRenderTimeout + time.Second)
		synctest.Wait()
		require.Equal(t, int64(1), renderer.canceled.Load(), "the hung refresh render must be stopped")
		require.Equal(t, 1, entriesMentioning(entries, "shared cv pdf work failed after every caller left"))
		require.Zero(t, client.puts.Load())
		service.flights.mu.Lock()
		remaining := len(service.flights.flights)
		service.flights.mu.Unlock()
		require.Zero(t, remaining, "no refresh or store work may outlive the shared timeout")
	})
}

// TestDownloadPDFRefreshStoreFailureBacksOff verifies that after a refresh whose render or upload failed, public
// downloads start no new refresh until the backoff window ends, and that the first download after it retries.
func TestDownloadPDFRefreshStoreFailureBacksOff(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		failRender bool
	}{
		{name: "render fails", failRender: true},
		{name: "upload fails"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				renderer := newGatedPDFRenderer()
				renderer.fail = tc.failRender
				renderer.openAll()
				content := &scriptedContentStore{versionedContentStore: versionedContentStore{current: "# CV"}}
				service, pdfStore, client := newFlightTestPDFService(t, renderer, content)
				client.failPuts.Store(!tc.failRender)
				h := &handler{store: content, pdfStore: pdfStore, pdfService: service}

				require.Equal(t, http.StatusNotFound, requestCVPDF(h, nil))
				synctest.Wait()
				require.Len(t, renderer.startedContents(), 1)

				const downloads = 5
				for range downloads {
					time.Sleep(pdfRefreshFailureBackoff / (downloads + 1))
					require.Equal(t, http.StatusNotFound, requestCVPDF(h, nil))
					synctest.Wait()
				}
				require.Len(t, renderer.startedContents(), 1, "no refresh may start within the backoff window")

				time.Sleep(pdfRefreshFailureBackoff)
				require.Equal(t, http.StatusNotFound, requestCVPDF(h, nil))
				synctest.Wait()
				require.Len(t, renderer.startedContents(), 2, "a download after the backoff window must retry")
				if !tc.failRender {
					require.Equal(t, int64(2), client.puts.Load())
				}
			})
		})
	}
}
