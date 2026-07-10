package cv

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/web"
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

// TestBuildCVSiteMetadata verifies base CV metadata keeps favicon inheriting from global site metadata.
// It takes a testing.T and returns no values.
func TestBuildCVSiteMetadata(t *testing.T) {
	t.Parallel()

	meta := buildCVSiteMetadata()
	require.Equal(t, cvSiteID, meta.ID)
	require.Equal(t, cvSiteTheme, meta.Theme)
	require.Equal(t, cvSiteTitle, meta.Title)
	require.Contains(t, meta.Description, "Senior Software Engineer")
	require.Equal(t, cvSiteTitle, meta.OGTitle)
	require.Empty(t, meta.Favicon)
}

// TestGetPageMeta verifies the CV metadata endpoint returns resolved favicon and og:image.
// It takes a testing.T and returns no values.
func TestGetPageMeta(t *testing.T) {
	web.RegisterSiteMetadata([]string{cvSitePathPrefix}, web.SiteMetadata{
		ID:      cvSiteID,
		Theme:   cvSiteTheme,
		Title:   cvSiteTitle,
		Favicon: "https://example.com/cv.ico",
		OGImage: "https://example.com/cv-og.png",
	})

	h := &handler{}
	ctx, recorder := newCVTestContext(http.MethodGet, "/cv/meta")
	ctx.Request.Host = "127.0.0.1:24456"

	h.getPageMeta(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload pageMetaResponse
	err := json.NewDecoder(resp.Body).Decode(&payload)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/cv.ico", payload.Favicon)
	require.Equal(t, "https://example.com/cv-og.png", payload.OGImage)
}

// TestServeCVRobots verifies robots.txt advertises crawl access and discovery files.
// It takes a testing.T and returns no values.
func TestServeCVRobots(t *testing.T) {
	t.Parallel()

	ctx, recorder := newCVTestContext(http.MethodGet, "/robots.txt")

	serveCVRobots(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/plain")
	require.Contains(t, string(body), "Allow: /")
	require.Contains(t, string(body), "https://cv.laisky.com/llms.txt")
	require.Contains(t, string(body), "https://cv.laisky.com/sitemap.xml")
}

// TestServeCVLLMs verifies llms.txt gives agents the public CV and API surface.
// It takes a testing.T and returns no values.
func TestServeCVLLMs(t *testing.T) {
	t.Parallel()

	ctx, recorder := newCVTestContext(http.MethodGet, "/llms.txt")

	serveCVLLMs(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body), "Zhonghua (Laisky) Cai CV")
	require.Contains(t, string(body), "https://cv.laisky.com/cv/content")
	require.Contains(t, string(body), "https://cv.laisky.com/openapi.json")
	require.Contains(t, string(body), "https://mcp.laisky.com")
}

// TestServeCVRootAgentSurfaceLetsBrowserSPAThrough verifies normal browser root requests reach the SPA handler.
// It takes a testing.T and returns no values.
func TestServeCVRootAgentSurfaceLetsBrowserSPAThrough(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(serveCVRootAgentSurface)
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "spa cv page")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "cv.laisky.com"
	req.Header.Set("Accept", "text/html")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "spa cv page", string(body))
}

// TestServeCVRootAgentSurfaceServesExplicitAgentMode verifies explicit agent mode remains crawlable.
// It takes a testing.T and returns no values.
func TestServeCVRootAgentSurfaceServesExplicitAgentMode(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(serveCVRootAgentSurface)
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "spa cv page")
	})

	req := httptest.NewRequest(http.MethodGet, "/?mode=agent", nil)
	req.Host = "cv.laisky.com"
	req.Header.Set("Accept", "text/html")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body), "Agent Mode Active")
	require.NotContains(t, string(body), "spa cv page")
}

// TestServeCVSitemap verifies the sitemap exposes machine-readable CV targets.
// It takes a testing.T and returns no values.
func TestServeCVSitemap(t *testing.T) {
	t.Parallel()

	ctx, recorder := newCVTestContext(http.MethodGet, "/sitemap.xml")

	serveCVSitemap(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/xml")
	require.Contains(t, string(body), "<loc>https://cv.laisky.com/</loc>")
	require.Contains(t, string(body), "<loc>https://cv.laisky.com/openapi.json</loc>")
}

// TestServeCVOpenAPI verifies the OpenAPI document exposes public CV endpoints.
// It takes a testing.T and returns no values.
func TestServeCVOpenAPI(t *testing.T) {
	t.Parallel()

	ctx, recorder := newCVTestContext(http.MethodGet, "/openapi.json")

	serveCVOpenAPI(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	var payload map[string]any
	err := json.NewDecoder(resp.Body).Decode(&payload)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "3.1.0", payload["openapi"])

	paths, ok := payload["paths"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, paths, "/cv/content")
	require.Contains(t, paths, "/cv/pdf")
	require.Contains(t, paths, "/cv/meta")
}

// TestServeCVAICatalog verifies agent catalog metadata links docs, APIs, and MCP.
// It takes a testing.T and returns no values.
func TestServeCVAICatalog(t *testing.T) {
	t.Parallel()

	ctx, recorder := newCVTestContext(http.MethodGet, "/.well-known/ai-catalog.json")

	serveCVAICatalog(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	var payload map[string]any
	err := json.NewDecoder(resp.Body).Decode(&payload)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "Zhonghua (Laisky) Cai CV", payload["name"])
	require.Equal(t, "0.1", payload["specVersion"])
	entries, ok := payload["entries"].([]any)
	require.True(t, ok)
	require.Len(t, entries, 3)
}

// TestServeCVMCPMetadata verifies MCP discovery metadata includes the public server.
// It takes a testing.T and returns no values.
func TestServeCVMCPMetadata(t *testing.T) {
	t.Parallel()

	ctx, recorder := newCVTestContext(http.MethodGet, "/.well-known/mcp.json")

	serveCVMCPMetadata(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	var payload map[string]any
	err := json.NewDecoder(resp.Body).Decode(&payload)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, cvPublicMCPServer, payload["url"])
	require.Equal(t, "streamable-http", payload["transport"])
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
	payload  ContentPayload
	history  []ContentHistoryEntry
	versions map[string]ContentPayload
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

// ListHistory returns the configured content history, truncated to the requested limit.
func (f *fakeContentStore) ListHistory(ctx context.Context, limit int) ([]ContentHistoryEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return []ContentHistoryEntry{}, nil
	}

	history := append([]ContentHistoryEntry(nil), f.history...)
	if len(history) > limit {
		history = history[:limit]
	}
	return history, nil
}

// LoadVersion returns the configured version payload or the current payload when no version is requested.
func (f *fakeContentStore) LoadVersion(ctx context.Context, versionID string) (ContentPayload, error) {
	if err := ctx.Err(); err != nil {
		return ContentPayload{}, err
	}
	if versionID == "" {
		return f.payload, nil
	}
	if payload, ok := f.versions[versionID]; ok {
		return payload, nil
	}
	return ContentPayload{}, ErrContentVersionNotFound
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

// TestListContentHistory verifies the history endpoint returns the latest persisted versions for the editor.
func TestListContentHistory(t *testing.T) {
	t.Parallel()

	history := []ContentHistoryEntry{
		{VersionID: "v3", UpdatedAt: time.Unix(300, 0).UTC(), IsLatest: true},
		{VersionID: "v2", UpdatedAt: time.Unix(200, 0).UTC(), IsLatest: false},
	}
	h := &handler{store: &fakeContentStore{history: history}}
	ctx, recorder := newCVTestContext(http.MethodGet, "/cv/content/history")

	h.listContentHistory(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload contentHistoryResponse
	err := json.NewDecoder(resp.Body).Decode(&payload)
	require.NoError(t, err)
	require.Equal(t, history, payload.Items)
}

// TestGetContentVersion verifies the version endpoint returns the selected persisted revision content.
func TestGetContentVersion(t *testing.T) {
	t.Parallel()

	updatedAt := time.Unix(200, 0).UTC()
	h := &handler{store: &fakeContentStore{
		versions: map[string]ContentPayload{
			"v2": {
				Content:   "revision-2",
				UpdatedAt: &updatedAt,
				IsDefault: false,
			},
		},
	}}
	ctx, recorder := newCVTestContext(http.MethodGet, "/cv/content/version?version_id=v2")

	h.getContentVersion(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload contentVersionResponse
	err := json.NewDecoder(resp.Body).Decode(&payload)
	require.NoError(t, err)
	require.Equal(t, "revision-2", payload.Content)
	require.Equal(t, "v2", payload.VersionID)
	require.NotNil(t, payload.UpdatedAt)
	require.Equal(t, updatedAt, payload.UpdatedAt.UTC())
}

// TestGetContentVersionNotFound verifies the version endpoint returns 404 for a missing persisted revision.
func TestGetContentVersionNotFound(t *testing.T) {
	t.Parallel()

	h := &handler{store: &fakeContentStore{versions: map[string]ContentPayload{}}}
	ctx, recorder := newCVTestContext(http.MethodGet, "/cv/content/version?version_id=missing")

	h.getContentVersion(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
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

// TestDownloadPDFAccessDeniedMissingTriggersAsyncRender verifies AccessDenied-missing PDFs trigger background render.
// It takes a testing.T and returns no values.
func TestDownloadPDFAccessDeniedMissingTriggersAsyncRender(t *testing.T) {
	t.Parallel()

	content := "CV async access denied"
	payload := ContentPayload{Content: content, IsDefault: false}
	store := &fakeContentStore{payload: payload}

	client := &statAccessDeniedS3Client{
		inner:              newFakeS3Client(),
		denyGetWhenMissing: true,
	}
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
	ctx, recorder := newCVTestContext(http.MethodGet, "/cv/pdf?ts=1700000000002")

	h.downloadPDF(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	require.Equal(t, http.StatusNotFound, ctx.Writer.Status())
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for async pdf render after access denied")
	}

	waitForPDFStore(t, pdfStore, 2*time.Second)
}

// TestRenderPDFPreviewReturnsPDFWithoutPersisting verifies the preview endpoint renders
// markdown to PDF bytes and never writes the result to the configured PDF store.
func TestRenderPDFPreviewReturnsPDFWithoutPersisting(t *testing.T) {
	t.Parallel()

	client := newFakeS3Client()
	pdfStore, err := NewS3PDFStore(client, "bucket", "cv.pdf")
	require.NoError(t, err)

	renderer := &trackingPDFRenderer{}
	pdfService, err := NewPDFService(renderer, pdfStore)
	require.NoError(t, err)

	h := &handler{pdfService: pdfService}
	body := strings.NewReader(`{"content":"# Tailored CV\nbody"}`)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cv/pdf/preview", body)
	req.Header.Set("Content-Type", "application/json")
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = req

	h.renderPDFPreview(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/pdf", resp.Header.Get("Content-Type"))
	require.Equal(t, cvPDFCacheControl, resp.Header.Get("Cache-Control"))
	require.True(t, renderer.called)

	payload, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "PDF:# Tailored CV\nbody", string(payload))

	_, _, openErr := pdfStore.Open(context.Background())
	require.ErrorIs(t, openErr, ErrObjectNotFound)
}

// TestRenderPDFPreviewRejectsEmptyContent verifies the preview endpoint refuses
// blank markdown payloads.
func TestRenderPDFPreviewRejectsEmptyContent(t *testing.T) {
	t.Parallel()

	client := newFakeS3Client()
	pdfStore, err := NewS3PDFStore(client, "bucket", "cv.pdf")
	require.NoError(t, err)

	renderer := &trackingPDFRenderer{}
	pdfService, err := NewPDFService(renderer, pdfStore)
	require.NoError(t, err)

	h := &handler{pdfService: pdfService}
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cv/pdf/preview", strings.NewReader(`{"content":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = req

	h.renderPDFPreview(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	require.NotEqual(t, http.StatusOK, resp.StatusCode)
	require.False(t, renderer.called)
}

// TestRenderPDFPreviewWithoutServiceReturns503 verifies the endpoint signals
// service unavailability when the PDF service is not configured.
func TestRenderPDFPreviewWithoutServiceReturns503(t *testing.T) {
	t.Parallel()

	h := &handler{}
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cv/pdf/preview", strings.NewReader(`{"content":"# CV"}`))
	req.Header.Set("Content-Type", "application/json")
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = req

	h.renderPDFPreview(ctx)

	resp := recorder.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
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
