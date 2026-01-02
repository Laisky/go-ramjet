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

func TestSiteMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create temporary dist directory
	distDir, err := os.MkdirTemp("", "spa_metadata_test")
	require.NoError(t, err)
	defer os.RemoveAll(distDir)

	// Create index.html with placeholders
	indexPath := filepath.Join(distDir, "index.html")
	htmlContent := `<!DOCTYPE html>
<html>
<head>
	<title>Default Title</title>
	<link rel="icon" href="/default.ico">
</head>
<body></body>
</html>`
	err = os.WriteFile(indexPath, []byte(htmlContent), 0644)
	require.NoError(t, err)

	r := gin.New()
	err = RegisterSPA(r, distDir)
	require.NoError(t, err)

	// Register metadata
	RegisterSiteMetadata([]string{"chat.example.com", "/gptchat"}, SiteMetadata{
		Title:   "Custom Chat",
		Favicon: "/custom.ico",
	})

	t.Run("default metadata", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		req.Host = "www.example.com"
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "<title>Laisky</title>")
		require.Contains(t, w.Body.String(), `href="https://s3.laisky.com/uploads/2025/12/favicon.ico"`)
	})

	t.Run("host match", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		req.Host = "chat.example.com"
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "<title>Custom Chat</title>")
		require.Contains(t, w.Body.String(), `href="/custom.ico"`)
	})

	t.Run("path match", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/gptchat/some/page", nil)
		req.Header.Set("Accept", "text/html")
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "<title>Custom Chat</title>")
		require.Contains(t, w.Body.String(), `href="/custom.ico"`)
	})

	t.Run("chat2 host match", func(t *testing.T) {
		// Register chat2
		RegisterSiteMetadata([]string{"chat2.example.com"}, SiteMetadata{
			Title: "Chat 2",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		req.Host = "chat2.example.com"
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "<title>Chat 2</title>")
	})
}
