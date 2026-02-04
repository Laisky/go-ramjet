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

// newCVTestContext builds a gin context for CV handler tests.
func newCVTestContext(method string, url string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(method, url, nil)
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = req
	return ctx, recorder
}

// TestDownloadPDFSetsNoCacheHeaders verifies PDF responses disable caching.
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
