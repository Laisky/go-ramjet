package cv

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
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

// countingS3Client counts PDF uploads while delegating storage to the in-memory fake client.
type countingS3Client struct {
	*fakeS3Client
	puts atomic.Int64
}

// PutObject counts the upload and stores it through the fake client.
func (c *countingS3Client) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	c.puts.Add(1)
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
			callerErr = service.RenderAndStore(callerCtx, "# Hung CV")
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
		service, _, client := newFlightTestPDFService(t, renderer, &fakeContentStore{})

		const callers = 4
		errs := make([]error, callers)
		var wg sync.WaitGroup
		for i := range callers {
			wg.Go(func() {
				errs[i] = service.RenderAndStore(context.Background(), "# Broken CV")
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

		require.ErrorContains(t, service.RenderAndStore(context.Background(), "# Broken CV"), "renderer exploded")
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
