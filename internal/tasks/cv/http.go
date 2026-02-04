package cv

import (
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
	if shouldRenderFreshPDF(c) && h.pdfService != nil && h.store != nil {
		payload, err := h.store.Load(gmw.Ctx(c))
		if web.AbortErr(c, err) {
			return
		}

		pdfBytes, err := h.pdfService.Render(gmw.Ctx(c), payload.Content)
		if web.AbortErr(c, err) {
			return
		}

		if h.pdfStore != nil {
			if err := h.pdfStore.Save(gmw.Ctx(c), pdfBytes); err != nil {
				logger.Warn("cv pdf store failed", zap.Error(err))
			} else {
				logger.Debug("cv pdf stored", zap.Int("pdf_bytes", len(pdfBytes)))
			}
		}

		logger.Debug("cv pdf download",
			zap.String("source", "render"),
			zap.String("cache_buster", cacheBuster),
			zap.Int("content_bytes", len(payload.Content)),
			zap.String("updated_at", formatUpdatedAt(payload.UpdatedAt)))
		c.Header("Cache-Control", cvPDFCacheControl)
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Header("Surrogate-Control", "no-store")
		c.Header("Content-Disposition", "attachment; filename=\"cv.pdf\"")
		c.Data(http.StatusOK, "application/pdf", pdfBytes)
		return
	}

	if h.pdfStore == nil {
		c.Status(http.StatusNotFound)
		return
	}

	reader, size, err := h.pdfStore.Open(gmw.Ctx(c))
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			c.Status(http.StatusNotFound)
			return
		}
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

// shouldRenderFreshPDF reports whether the request context c asks for a freshly rendered PDF.
// It inspects c's query parameters and headers and returns true when a fresh PDF is requested.
func shouldRenderFreshPDF(c *gin.Context) bool {
	if c == nil {
		return false
	}

	if strings.TrimSpace(c.Query("ts")) != "" || strings.TrimSpace(c.Query("fresh")) != "" {
		return true
	}

	cacheControl := strings.ToLower(strings.TrimSpace(c.GetHeader("Cache-Control")))
	if strings.Contains(cacheControl, "no-cache") || strings.Contains(cacheControl, "no-store") {
		return true
	}

	pragma := strings.ToLower(strings.TrimSpace(c.GetHeader("Pragma")))
	return strings.Contains(pragma, "no-cache")
}

// formatUpdatedAt formats updatedAt as RFC3339, returning an empty string when nil.
func formatUpdatedAt(updatedAt *time.Time) string {
	if updatedAt == nil {
		return ""
	}
	return updatedAt.UTC().Format(time.RFC3339)
}
