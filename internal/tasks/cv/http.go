package cv

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/laisky-blog-graphql/library/auth"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/web"
)

const pdfAsyncRenderTimeout = 60 * time.Second

// contentRequest represents the payload for updating CV content.
type contentRequest struct {
	Content string `json:"content"`
}

// handler provides HTTP handlers for the CV task.
type handler struct {
	store      ContentRepository
	pdfStore   *S3PDFStore
	pdfService *PDFService
}

// bindHTTP registers CV routes and metadata.
func bindHTTP(store ContentRepository, pdfStore *S3PDFStore, pdfService *PDFService) {
	web.RegisterSiteMetadata([]string{"/cv"}, web.SiteMetadata{
		ID:      "cv",
		Theme:   "cv",
		Title:   "Zhonghua (Laisky) Cai | CV",
		Favicon: "https://s3.laisky.com/uploads/2025/12/favicon.ico",
	})

	h := &handler{
		store:      store,
		pdfStore:   pdfStore,
		pdfService: pdfService,
	}

	grp := web.Server.Group("/cv")
	grp.GET("/content", h.getContent)
	grp.PUT("/content", auth.AuthMw, h.saveContent)
	grp.GET("/pdf", h.downloadPDF)
}

// getContent returns the stored CV markdown content.
func (h *handler) getContent(c *gin.Context) {
	logger := gmw.GetLogger(c)

	payload, err := h.store.Load(gmw.Ctx(c))
	if web.AbortErr(c, err) {
		return
	}

	logger.Debug("cv content loaded",
		zap.Int("bytes", len(payload.Content)),
		zap.Bool("is_default", payload.IsDefault))
	c.JSON(http.StatusOK, payload)
}

// saveContent updates the stored CV markdown content.
func (h *handler) saveContent(c *gin.Context) {
	logger := gmw.GetLogger(c)

	var req contentRequest
	if err := c.ShouldBindJSON(&req); web.AbortErr(c, err) {
		return
	}

	if strings.TrimSpace(req.Content) == "" {
		web.AbortErr(c, errors.WithStack(errors.New("content is empty")))
		return
	}

	payload, err := h.store.Save(gmw.Ctx(c), req.Content)
	if web.AbortErr(c, err) {
		return
	}

	if h.pdfService != nil {
		if err := h.pdfService.RenderAndStore(gmw.Ctx(c), payload.Content); web.AbortErr(c, err) {
			return
		}
		logger.Debug("cv pdf uploaded", zap.Int("content_bytes", len(payload.Content)))
	}

	logger.Debug("cv content saved",
		zap.Int("bytes", len(payload.Content)),
		zap.String("updated_at", formatUpdatedAt(payload.UpdatedAt)))
	c.JSON(http.StatusOK, payload)
}

// downloadPDF streams the CV PDF file if configured.
func (h *handler) downloadPDF(c *gin.Context) {
	logger := gmw.GetLogger(c)

	cacheBuster := strings.TrimSpace(c.Query("ts"))
	if h.pdfStore == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	reader, size, err := h.pdfStore.Open(gmw.Ctx(c))
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			logger.Debug("cv pdf unavailable, trigger async refresh",
				zap.String("cache_buster", cacheBuster))
			h.triggerAsyncPDFRefresh(c, cacheBuster)
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		logger.Debug("cv pdf open failed",
			zap.String("cache_buster", cacheBuster),
			zap.Error(err))
		web.AbortErr(c, err)
		return
	}
	defer func() {
		if cerr := reader.Close(); cerr != nil {
			logger.Error("close cv pdf reader", zap.Error(cerr))
		}
	}()

	logger.Debug("cv pdf download",
		zap.String("source", "s3"),
		zap.String("cache_buster", cacheBuster))
	c.Header("Cache-Control", cvPDFCacheControl)
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.Header("Surrogate-Control", "no-store")
	c.Header("Content-Disposition", "attachment; filename=\"cv.pdf\"")
	c.DataFromReader(http.StatusOK, size, "application/pdf", reader, nil)
}

// triggerAsyncPDFRefresh renders and stores the latest CV PDF in the background.
// It takes the request context and cache buster string and returns no values.
func (h *handler) triggerAsyncPDFRefresh(c *gin.Context, cacheBuster string) {
	if h == nil || h.pdfService == nil || h.store == nil || c == nil {
		return
	}

	logger := gmw.GetLogger(c)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), pdfAsyncRenderTimeout)
		defer cancel()

		payload, err := h.store.Load(ctx)
		if err != nil {
			logger.Warn("cv pdf async refresh load failed", zap.Error(err))
			return
		}

		if err := h.pdfService.RenderAndStore(ctx, payload.Content); err != nil {
			logger.Warn("cv pdf async refresh render failed", zap.Error(err))
			return
		}

		logger.Debug("cv pdf async refresh completed",
			zap.String("cache_buster", cacheBuster),
			zap.Int("content_bytes", len(payload.Content)),
			zap.String("updated_at", formatUpdatedAt(payload.UpdatedAt)))
	}()
}

// formatUpdatedAt formats updatedAt as RFC3339, returning an empty string when nil.
func formatUpdatedAt(updatedAt *time.Time) string {
	if updatedAt == nil {
		return ""
	}
	return updatedAt.UTC().Format(time.RFC3339)
}
