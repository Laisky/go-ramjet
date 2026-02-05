package cv

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
)

// newCVTestContext builds a gin context for the provided method and URL and returns the context plus recorder.
func newCVTestContext(method string, url string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(method, url, nil)
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = req
	return ctx, recorder
}

// TestDownloadPDFSetsNoCacheHeaders verifies PDF responses disable caching; it takes a testing.T and returns no values.
func TestDownloadPDFSetsNoCacheHeaders(t *testing.T) {
	t.Parallel()

	client := newFakeS3Client()
	store, err := NewS3PDFStore(client, "bucket", "cv.pdf")
	require.NoError(t, err)

	err = store.Save(context.Background(), []byte("%PDF-1.4 mock"))
	require.NoError(t, err)

	h := &handler{pdfStore: store}
	ctx, recorder := newCVTestContext(http.MethodGet, "/cv/pdf?ts=123")

	h.downloadPDF(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, cvPDFCacheControl, resp.Header.Get("Cache-Control"))
	require.Equal(t, "no-cache", resp.Header.Get("Pragma"))
	require.Equal(t, "0", resp.Header.Get("Expires"))
	require.Equal(t, "no-store", resp.Header.Get("Surrogate-Control"))
	require.Equal(t, "attachment; filename=\"cv.pdf\"", resp.Header.Get("Content-Disposition"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NotEmpty(t, body)
}

// fakeContentStore is a content repository stub that returns predetermined content.
type fakeContentStore struct {
	payload ContentPayload
}

// Load returns the configured payload for ctx and returns an error when ctx is done.
func (f *fakeContentStore) Load(ctx context.Context) (ContentPayload, error) {
	if err := ctx.Err(); err != nil {
		return ContentPayload{}, err
	}
	return f.payload, nil
}

// Save returns the configured payload for ctx and returns an error when ctx is done.
func (f *fakeContentStore) Save(ctx context.Context, _ string) (ContentPayload, error) {
	if err := ctx.Err(); err != nil {
		return ContentPayload{}, err
	}
	return f.payload, nil
}

type trackingPDFRenderer struct {
	called bool
}

// Render marks the renderer as called and returns an error-free payload.
// It takes a context and content string and returns a PDF payload or an error.
func (tpr *trackingPDFRenderer) Render(ctx context.Context, content string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tpr.called = true
	return []byte("PDF:" + content), nil
}

type signalPDFRenderer struct {
	ch chan struct{}
}

// Render signals the channel when invoked and returns a deterministic PDF payload.
// It takes a context and content string and returns a PDF payload or an error.
func (spr *signalPDFRenderer) Render(ctx context.Context, content string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if spr.ch != nil {
		select {
		case spr.ch <- struct{}{}:
		default:
		}
	}
	return []byte("%PDF-1.4 async"), nil
}

type missingOnceS3Client struct {
	inner       *fakeS3Client
	missingLeft int
	statCalls   int
	lastObject  string
}

// GetObject returns the inner client object reader.
// It takes context, bucket, object name, and options and returns a reader or an error.
func (m *missingOnceS3Client) GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (io.ReadCloser, error) {
	return m.inner.GetObject(ctx, bucketName, objectName, opts)
}

// PutObject stores an object via the inner client.
// It takes context, bucket, object name, reader, size, and options and returns upload info or an error.
func (m *missingOnceS3Client) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	return m.inner.PutObject(ctx, bucketName, objectName, reader, objectSize, opts)
}

// RemoveObject delegates removal to the inner client.
// It takes context, bucket, object name, and options and returns an error.
func (m *missingOnceS3Client) RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error {
	return m.inner.RemoveObject(ctx, bucketName, objectName, opts)
}

// ListObjects delegates listing to the inner client.
// It takes context, bucket, and options and returns a channel of object info.
func (m *missingOnceS3Client) ListObjects(ctx context.Context, bucketName string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	return m.inner.ListObjects(ctx, bucketName, opts)
}

// StatObject returns a missing response once before delegating to the inner client.
// It takes context, bucket, object name, and options and returns object info or an error.
func (m *missingOnceS3Client) StatObject(ctx context.Context, bucketName, objectName string, opts minio.StatObjectOptions) (minio.ObjectInfo, error) {
	m.statCalls++
	m.lastObject = objectName
	if m.missingLeft > 0 {
		m.missingLeft--
		return minio.ObjectInfo{}, minio.ErrorResponse{Code: "NoSuchKey", StatusCode: 404}
	}
	return m.inner.StatObject(ctx, bucketName, objectName, opts)
}

// TestDownloadPDFUsesStoredPDF verifies the handler serves stored PDFs even with cache busters.
// It takes a testing.T and returns no values.
func TestDownloadPDFUsesStoredPDF(t *testing.T) {
	t.Parallel()

	content := "CV https://cv.laisky.com"
	payload := ContentPayload{Content: content, IsDefault: false}
	store := &fakeContentStore{payload: payload}

	client := newFakeS3Client()
	pdfStore, err := NewS3PDFStore(client, "bucket", "cv.pdf")
	require.NoError(t, err)
	require.Equal(t, "cv.pdf", pdfStore.key)

	err = pdfStore.Save(context.Background(), []byte("%PDF-1.4 mock"))
	require.NoError(t, err)

	renderer := &trackingPDFRenderer{}
	pdfService, err := NewPDFService(renderer, pdfStore)
	require.NoError(t, err)

	h := &handler{
		store:      store,
		pdfStore:   pdfStore,
		pdfService: pdfService,
	}
	ctx, recorder := newCVTestContext(http.MethodGet, "/cv/pdf?ts=1700000000000")

	h.downloadPDF(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, cvPDFCacheControl, resp.Header.Get("Cache-Control"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "%PDF-1.4")
	require.False(t, renderer.called)
}

// TestDownloadPDFMissingTriggersAsyncRender verifies missing PDFs trigger a background render.
// It takes a testing.T and returns no values.
func TestDownloadPDFMissingTriggersAsyncRender(t *testing.T) {
	t.Parallel()

	content := "CV async"
	payload := ContentPayload{Content: content, IsDefault: false}
	store := &fakeContentStore{payload: payload}

	baseClient := newFakeS3Client()
	client := &missingOnceS3Client{inner: baseClient, missingLeft: 1}
	pdfStore, err := NewS3PDFStore(client, "bucket", "cv.pdf")
	require.NoError(t, err)

	signal := make(chan struct{}, 1)
	renderer := &signalPDFRenderer{ch: signal}
	pdfService, err := NewPDFService(renderer, pdfStore)
	require.NoError(t, err)

	h := &handler{
		store:      store,
		pdfStore:   pdfStore,
		pdfService: pdfService,
	}
	ctx, recorder := newCVTestContext(http.MethodGet, "/cv/pdf?ts=1700000000001")

	h.downloadPDF(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	require.Equal(t, http.StatusNotFound, ctx.Writer.Status())
	require.Equalf(t, http.StatusNotFound, resp.StatusCode, "statCalls=%d lastObject=%s missingLeft=%d", client.statCalls, client.lastObject, client.missingLeft)
	require.GreaterOrEqual(t, client.statCalls, 1)
	require.Equal(t, "cv.pdf", client.lastObject)

	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for async pdf render")
	}

	waitForPDFStore(t, pdfStore, 2*time.Second)
}

// waitForPDFStore waits for the PDF to appear in the store within the timeout.
// It takes a testing.T, PDF store, and timeout duration and returns no values.
func waitForPDFStore(t *testing.T, store *S3PDFStore, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		reader, _, err := store.Open(context.Background())
		if err == nil {
			require.NoError(t, reader.Close())
			return
		}
		if errors.Is(err, ErrObjectNotFound) {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		require.NoError(t, err)
	}

	t.Fatalf("timeout waiting for pdf to be stored")
}
