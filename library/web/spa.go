package web

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
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
		return errors.WithStack(errors.New("router is nil"))
	}
	if strings.TrimSpace(distDir) == "" {
		return errors.WithStack(errors.New("distDir is empty"))
	}

	indexPath := filepath.Join(distDir, "index.html")
	// Verify index.html exists but don't cache it
	if _, err := os.Stat(indexPath); err != nil {
		return errors.Wrapf(err, "stat spa index %q", indexPath)
	}

	assetsDir := filepath.Join(distDir, "assets")
	if _, statErr := os.Stat(assetsDir); statErr == nil {
		// Same gin StaticFS handler and http.FileServer headers as http.Dir,
		// but confined to assetsDir by os.Root and without directory listing.
		r.StaticFS("/assets", rootFileSystem{dir: assetsDir})
	}

	// indexHandler serves the SPA index for request context c and returns no value.
	indexHandler := func(c *gin.Context) {
		logger := gmw.GetLogger(c)

		c.Header("Cache-Control", "no-store")
		indexBytes, err := os.ReadFile(indexPath)
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, errors.Wrap(err, "read index.html"))
			return
		}

		meta := getSiteMetadata(logger, c.Request.Host, c.Request.URL.Path)
		content := applySiteMetadataToHTML(string(indexBytes), meta)

		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
	}

	r.GET("/", indexHandler)

	r.NoRoute(func(c *gin.Context) {
		logger := gmw.GetLogger(c)

		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusNotFound)
			return
		}

		// Try to serve static file from distDir. URL paths are slash-separated,
		// so clean them with path (not filepath); confinement to distDir is
		// enforced by serveSPAFile, not by cleaning.
		cleanPath := path.Clean(c.Request.URL.Path)
		// Prevent serving root as it is handled by indexHandler
		if cleanPath == "/" || cleanPath == "." {
			indexHandler(c)
			return
		}

		if serveSPAFile(c, logger, distDir, cleanPath) {
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
			logger.Debug("spa fallback", zap.String("path", cleanPath), zap.String("newPath", newPath))
			if serveSPAFile(c, logger, distDir, newPath) {
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

// serveSPAFile serves the regular file name from distDir, confined to distDir.
//
// name comes from the request URL, so it is untrusted. It must be a local,
// slash-separated path (fs.ValidPath: no "..", ".", or empty elements once the
// leading slash is dropped), and it is resolved through an os.Root so neither
// ".." nor a symlink can reach outside distDir. The root is opened per request
// so a rebuilt dist directory is picked up without a restart, like index.html.
//
// Parameters:
//   - c: request context; the response is written to c.Writer when served.
//   - logger: request-scoped logger.
//   - distDir: SPA build directory that bounds every lookup.
//   - name: candidate file path taken from the request URL.
//
// Returns:
//   - bool: true when a file was served; false when the caller should fall back.
func serveSPAFile(c *gin.Context, logger glog.Logger, distDir, name string) bool {
	name = strings.TrimPrefix(name, "/")
	if name == "" || !fs.ValidPath(name) {
		logger.Debug("spa reject non-local path", zap.String("name", name))
		return false
	}

	root, err := os.OpenRoot(distDir)
	if err != nil {
		// Stay below the alert level (ERROR): with dist gone, every scanner hit
		// would otherwise page. The caller still answers 404 or index.html.
		logger.Warn("open spa dist root", zap.String("dir", distDir), zap.Error(err))
		return false
	}
	defer func() {
		if closeErr := root.Close(); closeErr != nil {
			logger.Warn("close spa dist root", zap.String("dir", distDir), zap.Error(closeErr))
		}
	}()

	fsys := root.FS()
	info, err := fs.Stat(fsys, name)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			// e.g. a symlink that escapes distDir.
			logger.Debug("spa stat rejected", zap.String("name", name), zap.Error(err))
		}
		return false
	}
	if info.IsDir() {
		return false
	}

	http.ServeFileFS(c.Writer, c.Request, fsys, name)
	return true
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
