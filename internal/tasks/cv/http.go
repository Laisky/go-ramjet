package cv

import (
	"net/http"
	"strings"

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
		zap.Int("bytes", len(payload.Content)))
	c.JSON(http.StatusOK, payload)
}

// downloadPDF streams the CV PDF file if configured.
func (h *handler) downloadPDF(c *gin.Context) {
	logger := gmw.GetLogger(c)

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

	logger.Debug("cv pdf download", zap.String("source", "s3"))
	c.Header("Content-Disposition", "attachment; filename=\"cv.pdf\"")
	c.DataFromReader(http.StatusOK, size, "application/pdf", reader, nil)
}
