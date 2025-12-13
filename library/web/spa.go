package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/log"
)

// RegisterSPA registers handlers that serve a built SPA (Vite output) from distDir.
//
// It serves static assets and returns distDir/index.html for unknown GET/HEAD routes
// that look like browser navigations (Accept includes text/html).
//
// Args:
//   - r: gin engine to register routes.
//   - distDir: filesystem directory that contains index.html and assets/.
//
// Returns:
//   - error: wrapped error when index.html cannot be read.
func RegisterSPA(r *gin.Engine, distDir string) error {
	if r == nil {
		return errors.New("router is nil")
	}
	if strings.TrimSpace(distDir) == "" {
		return errors.New("distDir is empty")
	}

	indexPath := filepath.Join(distDir, "index.html")
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		return errors.Wrapf(err, "read spa index %q", indexPath)
	}

	assetsDir := filepath.Join(distDir, "assets")
	if _, statErr := os.Stat(assetsDir); statErr == nil {
		r.StaticFS("/assets", http.Dir(assetsDir))
	}

	indexHandler := func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexBytes)
	}

	r.GET("/", indexHandler)

	r.NoRoute(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusNotFound)
			return
		}

		accept := c.GetHeader("Accept")
		if !strings.Contains(accept, "text/html") {
			c.Status(http.StatusNotFound)
			return
		}

		indexHandler(c)
	})

	return nil
}

// TryRegisterDefaultSPA tries to register the SPA served from the default dist path.
//
// Args:
//   - r: gin engine to register routes.
//
// Returns:
//   - bool: true if registration succeeded.
func TryRegisterDefaultSPA(r *gin.Engine) bool {
	logger := log.Logger.Named("spa")

	distDir := filepath.Join("web", "dist")
	if _, err := os.Stat(distDir); err != nil {
		logger.Info("spa dist dir not found, skip registering", zap.String("dir", distDir), zap.Error(err))
		return false
	}

	if err := RegisterSPA(r, distDir); err != nil {
		logger.Error("register spa", zap.Error(err))
		return false
	}

	logger.Info("spa registered", zap.String("dir", distDir))
	return true
}
