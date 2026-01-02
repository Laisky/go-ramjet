package web

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/log"
)

var (
	reTitle   = regexp.MustCompile(`(?i)<title>.*?</title>`)
	reFavicon = regexp.MustCompile(`(?i)<link[^>]*?rel="icon"[^>]*?>`)
	reHref    = regexp.MustCompile(`(?i)href="[^"]*"`)
)

// SiteMetadata represents metadata for a specific site.
type SiteMetadata struct {
	Title   string
	Favicon string
}

var (
	siteMetadataMu     sync.RWMutex
	siteMetadataByHost = make(map[string]SiteMetadata)
	siteMetadataByPath = make(map[string]SiteMetadata)
)

// RegisterSiteMetadata registers metadata for a specific host or path prefix.
// If hostOrPath starts with "/", it is treated as a path prefix.
// Otherwise, it is treated as a host.
func RegisterSiteMetadata(hostsOrPaths []string, metadata SiteMetadata) {
	siteMetadataMu.Lock()
	defer siteMetadataMu.Unlock()

	for _, hop := range hostsOrPaths {
		if strings.HasPrefix(hop, "/") {
			siteMetadataByPath[hop] = metadata
		} else {
			siteMetadataByHost[hop] = metadata
		}
	}
}

func getSiteMetadata(host, path string) SiteMetadata {
	siteMetadataMu.RLock()
	defer siteMetadataMu.RUnlock()

	log.Logger.Debug("get site metadata", zap.String("host", host), zap.String("path", path))

	// Try host match first
	if meta, ok := siteMetadataByHost[host]; ok {
		log.Logger.Debug("host match", zap.String("host", host), zap.String("title", meta.Title))
		return meta
	}

	// Try path prefix match (longest match first)
	var bestMatch string
	var bestMeta SiteMetadata
	for p, meta := range siteMetadataByPath {
		if strings.HasPrefix(path, p) && len(p) > len(bestMatch) {
			bestMatch = p
			bestMeta = meta
		}
	}

	if bestMatch != "" {
		log.Logger.Debug("path match", zap.String("path", path), zap.String("bestMatch", bestMatch), zap.String("title", bestMeta.Title))
		return bestMeta
	}

	// Default
	log.Logger.Debug("use default metadata", zap.String("host", host), zap.String("path", path))
	return SiteMetadata{
		Title:   "Laisky",
		Favicon: "https://s3.laisky.com/uploads/2025/12/favicon.ico",
	}
}

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
	// Verify index.html exists but don't cache it
	if _, err := os.Stat(indexPath); err != nil {
		return errors.Wrapf(err, "stat spa index %q", indexPath)
	}

	assetsDir := filepath.Join(distDir, "assets")
	if _, statErr := os.Stat(assetsDir); statErr == nil {
		r.StaticFS("/assets", http.Dir(assetsDir))
	}

	indexHandler := func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		indexBytes, err := os.ReadFile(indexPath)
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, errors.Wrap(err, "read index.html"))
			return
		}

		meta := getSiteMetadata(c.Request.Host, c.Request.URL.Path)
		content := string(indexBytes)
		if meta.Title != "" {
			content = reTitle.ReplaceAllString(content, "<title>"+meta.Title+"</title>")
		}
		if meta.Favicon != "" {
			content = reFavicon.ReplaceAllStringFunc(content, func(s string) string {
				return reHref.ReplaceAllString(s, `href="`+meta.Favicon+`"`)
			})
		}

		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
	}

	r.GET("/", indexHandler)

	r.NoRoute(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusNotFound)
			return
		}

		// Try to serve static file from distDir
		// Clean path to prevent directory traversal
		cleanPath := filepath.Clean(c.Request.URL.Path)
		// Prevent serving root as it is handled by indexHandler
		if cleanPath == "/" || cleanPath == "." {
			indexHandler(c)
			return
		}

		fpath := filepath.Join(distDir, cleanPath)
		info, err := os.Stat(fpath)
		if err == nil && !info.IsDir() {
			c.File(fpath)
			return
		}

		// If not found, try stripping the prefix
		// This supports proxying a subpath to the SPA root.
		//
		// Nginx's proxy_pass might merge the prefix with the next component
		// if trailing slashes are mismatched.
		// e.g., /gptchat/assets/foo.js -> /assets/foo.js
		// e.g., /gptchatassets/foo.js -> /assets/foo.js
		newPath := cleanPath
		if strings.HasPrefix(cleanPath, "/gptchat") {
			newPath = strings.TrimPrefix(cleanPath, "/gptchat")
		} else {
			parts := strings.Split(strings.TrimPrefix(cleanPath, "/"), "/")
			if len(parts) > 1 {
				newPath = strings.Join(parts[1:], "/")
			}
		}

		if newPath != cleanPath {
			fpath = filepath.Join(distDir, strings.TrimPrefix(newPath, "/"))
			log.Logger.Debug("spa fallback", zap.String("path", cleanPath), zap.String("newPath", newPath), zap.String("fpath", fpath))
			info, err = os.Stat(fpath)
			if err == nil && !info.IsDir() {
				c.File(fpath)
				return
			}
		}

		// If not found or is directory, fall back to index.html for SPA routes
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
