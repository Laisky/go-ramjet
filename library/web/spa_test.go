package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
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
		require.Contains(t, w.Body.String(), "<html>v1</html>")
		require.Contains(t, w.Body.String(), `name="ramjet-site" content="default"`)
	})

	t.Run("serve root index with cache bust", func(t *testing.T) {
		// Update index.html
		err = os.WriteFile(indexPath, []byte("<html>v2</html>"), 0644)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "<html>v2</html>")
		require.Contains(t, w.Body.String(), `name="ramjet-site" content="default"`)
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
		require.Contains(t, w.Body.String(), "<html>v2</html>")
		require.Contains(t, w.Body.String(), `name="ramjet-site" content="default"`)
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
<body><div id="root"></div></body>
</html>`
	err = os.WriteFile(indexPath, []byte(htmlContent), 0644)
	require.NoError(t, err)

	r := gin.New()
	err = RegisterSPA(r, distDir)
	require.NoError(t, err)

	// Register metadata
	RegisterSiteMetadata([]string{"chat.example.com", "/gptchat"}, SiteMetadata{
		ID:      "chat",
		Theme:   "chat",
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
		require.Contains(t, w.Body.String(), `name="ramjet-site" content="default"`)
		require.Contains(t, w.Body.String(), `name="ramjet-theme" content="default"`)
	})

	t.Run("host match", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		req.Host = "chat.example.com"
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "<title>Custom Chat</title>")
		require.Contains(t, w.Body.String(), `href="/custom.ico"`)
		require.Contains(t, w.Body.String(), `name="ramjet-site" content="chat"`)
		require.Contains(t, w.Body.String(), `name="ramjet-theme" content="chat"`)
	})

	t.Run("path match", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/gptchat/some/page", nil)
		req.Header.Set("Accept", "text/html")
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "<title>Custom Chat</title>")
		require.Contains(t, w.Body.String(), `href="/custom.ico"`)
		require.Contains(t, w.Body.String(), `name="ramjet-site" content="chat"`)
		require.Contains(t, w.Body.String(), `name="ramjet-theme" content="chat"`)
	})

	t.Run("chat2 host match", func(t *testing.T) {
		// Register chat2
		RegisterSiteMetadata([]string{"chat2.example.com"}, SiteMetadata{
			ID:    "chat2",
			Theme: "chat",
			Title: "Chat 2",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		req.Host = "chat2.example.com"
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "<title>Chat 2</title>")
		require.Contains(t, w.Body.String(), `name="ramjet-site" content="chat2"`)
	})

	t.Run("head and root fallback", func(t *testing.T) {
		RegisterSiteMetadata([]string{"fallback.example.com"}, SiteMetadata{
			ID:               "fallback",
			Theme:            "fallback",
			Title:            "Fallback Site",
			HeadHTML:         `<link rel="canonical" href="https://fallback.example.com/">`,
			RootFallbackHTML: `<main><h1>Fallback content</h1></main>`,
		})
		RegisterSiteMetadata([]string{"fallback.example.com"}, SiteMetadata{
			ID:      "fallback",
			Theme:   "fallback",
			Title:   "Configured Fallback Site",
			Favicon: "/configured.ico",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		req.Host = "fallback.example.com"
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "<title>Configured Fallback Site</title>")
		require.Contains(t, w.Body.String(), `href="/configured.ico"`)
		require.Contains(t, w.Body.String(), `<link rel="canonical" href="https://fallback.example.com/">`)
		require.Contains(t, w.Body.String(), `<div id="root"><main><h1>Fallback content</h1></main></div>`)
	})

	t.Run("later metadata preserves fallback", func(t *testing.T) {
		RegisterSiteMetadata([]string{"merged-fallback.example.com"}, SiteMetadata{
			ID:               "fallback",
			Theme:            "fallback",
			Title:            "Fallback Site",
			RootFallbackHTML: `<main><h1>Fallback content</h1></main>`,
		})
		RegisterSiteMetadata([]string{"merged-fallback.example.com"}, SiteMetadata{
			Title: "Config Title",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/", nil)
		req.Host = "merged-fallback.example.com"
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "<title>Config Title</title>")
		require.Contains(t, w.Body.String(), `<div id="root"><main><h1>Fallback content</h1></main></div>`)
	})
}

// newSPATraversalFixture lays out <tmp>/dist (the served SPA) next to a
// sibling secret file and a symlink inside dist that points at it.
//
// Parameters:
//   - t: test handle.
//
// Returns:
//   - http.Handler: the production pipeline (normalizeHandler wrapping gin).
//   - *gin.Engine: the bare gin engine, to probe paths normalizeHandler rewrites.
//   - string: secret content that must never be served.
func newSPATraversalFixture(t *testing.T) (http.Handler, *gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	const secret = "TOP-SECRET-OUTSIDE-DIST"
	base := t.TempDir()
	distDir := filepath.Join(base, "dist")
	require.NoError(t, os.MkdirAll(filepath.Join(distDir, "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<html>index</html>"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "vite.svg"), []byte("<svg>logo</svg>"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "assets", "app.js"), []byte("console.log(1)"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(base, "secret.txt"), []byte(secret), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(base, "secret.txt"), filepath.Join(distDir, "leak.txt")))
	require.NoError(t, os.Symlink(filepath.Join("..", "secret.txt"), filepath.Join(distDir, "leak-rel.txt")))
	// os.Root follows relative symlinks that stay inside distDir.
	require.NoError(t, os.Symlink("vite.svg", filepath.Join(distDir, "logo-link.svg")))

	r := gin.New()
	require.NoError(t, RegisterSPA(r, distDir))
	return &normalizeHandler{handler: r}, r, secret
}

// serveRawPath sends a GET whose request-target is rawTarget verbatim, the way
// a client on the wire can, and returns the recorded response.
//
// Parameters:
//   - t: test handle.
//   - h: handler under test.
//   - rawTarget: request-target, possibly percent-encoded or non-canonical.
//   - accept: Accept header value.
//
// Returns:
//   - *httptest.ResponseRecorder: recorded response.
func serveRawPath(t *testing.T, h http.Handler, rawTarget, accept string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	u, err := url.ParseRequestURI(rawTarget)
	require.NoError(t, err)
	req.URL = u
	req.RequestURI = rawTarget
	req.Header.Set("Accept", accept)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// TestRegisterSPARejectsPathTraversal reproduces CodeQL go/path-injection
// #75-#78: the prefix-stripping fallback turned `/gptchat../x` into `../x`,
// which escaped distDir. Every payload must stay confined to distDir, both
// through the production normalizeHandler pipeline and against bare gin.
func TestRegisterSPARejectsPathTraversal(t *testing.T) {
	pipeline, engine, secret := newSPATraversalFixture(t)

	payloads := []string{
		"/gptchat../secret.txt",
		"//gptchat../secret.txt",
		"/./gptchat../secret.txt",
		"/%2e/gptchat%2e%2e/secret.txt",
		"/x/../gptchat../secret.txt",
		"/x/..%2fgptchat..%2fsecret.txt",
		"/../secret.txt",
		"/%2e%2e/secret.txt",
		"/%2e%2e%2fsecret.txt",
		"/%252e%252e%252fsecret.txt",
		"/..%5csecret.txt",
		"/gptchat/..%5c..%5csecret.txt",
		"/x/../../secret.txt",
		"/gptchat/../../secret.txt",
		"/leak.txt",
		"/leak-rel.txt",
		"/gptchat/leak.txt",
		"/x/leak.txt",
	}
	for _, h := range []struct {
		name    string
		handler http.Handler
	}{
		{name: "pipeline", handler: pipeline},
		{name: "gin", handler: engine},
	} {
		for _, payload := range payloads {
			for _, accept := range []string{"*/*", "text/html"} {
				t.Run(h.name+" "+payload+" "+accept, func(t *testing.T) {
					w := serveRawPath(t, h.handler, payload, accept)
					require.NotContains(t, w.Body.String(), secret)
					require.NotEqual(t, http.StatusInternalServerError, w.Code)
					if accept == "*/*" {
						require.Contains(t, []int{http.StatusNotFound, http.StatusBadRequest}, w.Code,
							"non-navigation request must not be served: %d", w.Code)
					}
				})
			}
		}
	}
}

// TestRegisterSPAPrefixFallback pins the legitimate behaviors of the
// prefix-stripping fallback so the traversal fix keeps them working.
func TestRegisterSPAPrefixFallback(t *testing.T) {
	pipeline, engine, _ := newSPATraversalFixture(t)

	for _, tc := range []struct {
		name     string
		handler  http.Handler
		target   string
		wantCode int
		wantBody string
		wantLoc  string
	}{
		{name: "root static file", handler: pipeline, target: "/vite.svg", wantCode: http.StatusOK, wantBody: "<svg>logo</svg>"},
		{name: "gptchat prefix", handler: pipeline, target: "/gptchat/vite.svg", wantCode: http.StatusOK, wantBody: "<svg>logo</svg>"},
		{name: "merged gptchat prefix", handler: engine, target: "/gptchatvite.svg", wantCode: http.StatusOK, wantBody: "<svg>logo</svg>"},
		{name: "merged gptchat assets", handler: engine, target: "/gptchatassets/app.js", wantCode: http.StatusOK, wantBody: "console.log(1)"},
		{name: "gptchat assets", handler: pipeline, target: "/gptchat/assets/app.js", wantCode: http.StatusOK, wantBody: "console.log(1)"},
		{name: "other prefix", handler: pipeline, target: "/cv/vite.svg", wantCode: http.StatusOK, wantBody: "<svg>logo</svg>"},
		{name: "symlink inside dist", handler: pipeline, target: "/logo-link.svg", wantCode: http.StatusOK, wantBody: "<svg>logo</svg>"},
		{name: "index.html redirect", handler: pipeline, target: "/index.html", wantCode: http.StatusMovedPermanently, wantLoc: "./"},
		{name: "prefixed index.html redirect", handler: pipeline, target: "/gptchat/index.html", wantCode: http.StatusMovedPermanently, wantLoc: "./"},
		{name: "missing asset", handler: pipeline, target: "/gptchat/missing.js", wantCode: http.StatusNotFound},
		{name: "directory is not served", handler: pipeline, target: "/gptchat/assets", wantCode: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := serveRawPath(t, tc.handler, tc.target, "*/*")
			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			if tc.wantBody != "" {
				require.Equal(t, tc.wantBody, w.Body.String())
			}
			if tc.wantLoc != "" {
				require.Equal(t, tc.wantLoc, w.Header().Get("Location"))
			}
		})
	}

	t.Run("navigation falls back to index", func(t *testing.T) {
		w := serveRawPath(t, pipeline, "/gptchat/some/route", "text/html")
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "<html>index</html>")
	})
}
