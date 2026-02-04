package cv

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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

// fakePDFRenderer is a PDF renderer stub that embeds the content in a deterministic payload.
type fakePDFRenderer struct{}

// Render returns a deterministic PDF payload for ctx and content and returns an error when ctx is done.
func (f fakePDFRenderer) Render(ctx context.Context, content string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []byte("PDF:" + content), nil
}

// TestDownloadPDFFreshRender verifies the handler renders PDF on demand when a cache buster is present.
// It takes a testing.T and returns no values.
func TestDownloadPDFFreshRender(t *testing.T) {
	t.Parallel()

	content := "CV https://cv.laisky.com"
	payload := ContentPayload{Content: content, IsDefault: false}
	store := &fakeContentStore{payload: payload}

	client := newFakeS3Client()
	pdfStore, err := NewS3PDFStore(client, "bucket", "cv.pdf")
	require.NoError(t, err)

	pdfService, err := NewPDFService(fakePDFRenderer{}, pdfStore)
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
	require.Contains(t, string(body), content)
	require.Contains(t, string(body), "PDF:")
}
