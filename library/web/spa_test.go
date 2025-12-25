package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterSPA(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create temporary dist directory
	distDir, err := os.MkdirTemp("", "spa_test")
	require.NoError(t, err)
	defer os.RemoveAll(distDir)

	// Create initial index.html
	indexPath := filepath.Join(distDir, "index.html")
	err = os.WriteFile(indexPath, []byte("<html>v1</html>"), 0644)
	require.NoError(t, err)

	// Create assets directory
	assetsDir := filepath.Join(distDir, "assets")
	err = os.Mkdir(assetsDir, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(assetsDir, "style.css"), []byte("body {}"), 0644)
	require.NoError(t, err)

	// Create root static file
	err = os.WriteFile(filepath.Join(distDir, "vite.svg"), []byte("<svg>v1</svg>"), 0644)
	require.NoError(t, err)

	r := gin.New()
	err = RegisterSPA(r, distDir)
	require.NoError(t, err)

	t.Run("serve root index", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "<html>v1</html>", w.Body.String())
	})

	t.Run("serve root index with cache bust", func(t *testing.T) {
		// Update index.html
		err = os.WriteFile(indexPath, []byte("<html>v2</html>"), 0644)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "<html>v2</html>", w.Body.String())
	})

	t.Run("serve assets", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/assets/style.css", nil)
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "body {}", w.Body.String())
	})

	t.Run("serve root static file", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/vite.svg", nil)
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "<svg>v1</svg>", w.Body.String())
	})

	t.Run("spa fallback", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/some/route", nil)
		req.Header.Set("Accept", "text/html")
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		// Should serve current index.html
		require.Equal(t, "<html>v2</html>", w.Body.String())
	})

	t.Run("404 for non-html", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/some/api", nil)
		req.Header.Set("Accept", "application/json")
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code)
	})
}
