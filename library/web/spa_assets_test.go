package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestRegisterSPAAssetsNoDirectoryListing verifies the /assets route serves
// files only: directories are never listed, for any Accept header.
func TestRegisterSPAAssetsNoDirectoryListing(t *testing.T) {
	pipeline, engine, _ := newSPATraversalFixture(t)

	for _, h := range []struct {
		name    string
		handler http.Handler
	}{
		{name: "pipeline", handler: pipeline},
		{name: "gin", handler: engine},
	} {
		for _, target := range []string{"/assets/", "/assets/sub/", "/gptchat/assets/", "/assets/./"} {
			t.Run(h.name+" "+target, func(t *testing.T) {
				w := serveRawPath(t, h.handler, target, "*/*")
				require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
				require.NotContains(t, w.Body.String(), "app.js", "directory must not be listed")
				require.NotContains(t, w.Body.String(), "nested.js", "directory must not be listed")
			})
		}
	}
}

// TestRegisterSPAAssetsHeadersUnchanged verifies the confined /assets route
// answers exactly like gin's StaticFS over http.Dir did for regular files:
// same status, body and every header, for GET, HEAD, ranges and revalidation.
func TestRegisterSPAAssetsHeadersUnchanged(t *testing.T) {
	_, engine, _ := newSPATraversalFixture(t)

	assetsDir := filepath.Join(spaFixtureDistDir(t, engine), "assets")
	reference := gin.New()
	reference.StaticFS("/assets", http.Dir(assetsDir))

	appInfo, err := os.Stat(filepath.Join(assetsDir, "app.js"))
	require.NoError(t, err)
	lastModified := appInfo.ModTime().UTC().Format(http.TimeFormat)

	for _, tc := range []struct {
		name   string
		method string
		target string
		header map[string]string
	}{
		{name: "get", method: http.MethodGet, target: "/assets/app.js"},
		{name: "nested", method: http.MethodGet, target: "/assets/sub/nested.js"},
		{name: "head", method: http.MethodHead, target: "/assets/app.js"},
		{name: "range", method: http.MethodGet, target: "/assets/app.js", header: map[string]string{"Range": "bytes=0-6"}},
		{name: "not modified", method: http.MethodGet, target: "/assets/app.js", header: map[string]string{"If-Modified-Since": lastModified}},
		{name: "trailing slash on file", method: http.MethodGet, target: "/assets/app.js/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			do := func(h http.Handler) *httptest.ResponseRecorder {
				req := httptest.NewRequest(tc.method, tc.target, nil)
				for k, v := range tc.header {
					req.Header.Set(k, v)
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, req)
				return w
			}

			want, got := do(reference), do(engine)
			require.Equal(t, want.Code, got.Code)
			require.Equal(t, want.Header(), got.Header())
			require.Equal(t, want.Body.String(), got.Body.String())
		})
	}
}

// spaFixtureDistDir returns the dist directory newSPATraversalFixture recorded
// for engine.
//
// Parameters:
//   - t: test handle.
//   - engine: engine returned by newSPATraversalFixture.
//
// Returns:
//   - string: the fixture's dist directory.
func spaFixtureDistDir(t *testing.T, engine *gin.Engine) string {
	t.Helper()

	dir, ok := spaFixtureDirs.Load(engine)
	require.True(t, ok, "engine was not built by newSPATraversalFixture")
	return dir.(string)
}

// spaFixtureDirs maps fixture engines to their dist directory.
var spaFixtureDirs sync.Map

// levelRecorder records the level of every log entry written through it.
type levelRecorder struct {
	mu     sync.Mutex
	levels []zapcore.Level
}

// hook records entry's level and returns no error.
func (r *levelRecorder) hook(entry zapcore.Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.levels = append(r.levels, entry.Level)
	return nil
}

// maxLevel returns the highest recorded level, or a level below DebugLevel when
// nothing was recorded.
func (r *levelRecorder) maxLevel() zapcore.Level {
	r.mu.Lock()
	defer r.mu.Unlock()
	maxLevel := zapcore.DebugLevel - 1
	for _, l := range r.levels {
		maxLevel = max(maxLevel, l)
	}
	return maxLevel
}

// TestRegisterSPAMissingDistIsQuiet verifies that when the dist directory
// disappears after startup, unknown non-navigation routes stay plain 404s and
// never log at ERROR, which the alert pusher would forward for every scan.
func TestRegisterSPAMissingDistIsQuiet(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := &levelRecorder{}
	logger, err := glog.NewConsoleWithName("spa-test", glog.LevelDebug, zap.Hooks(recorder.hook))
	require.NoError(t, err)

	distDir := filepath.Join(t.TempDir(), "dist")
	require.NoError(t, os.MkdirAll(filepath.Join(distDir, "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<html>index</html>"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "assets", "app.js"), []byte("console.log(1)"), 0o644))

	r := gin.New()
	r.Use(func(c *gin.Context) {
		gmw.SetLogger(c, logger)
		c.Next()
	})
	require.NoError(t, RegisterSPA(r, distDir))
	require.NoError(t, os.RemoveAll(distDir))

	for _, target := range []string{"/wp-login.php", "/gptchat/.env", "/assets/app.js", "/x/y/z.js"} {
		w := serveRawPath(t, r, target, "*/*")
		require.Equal(t, http.StatusNotFound, w.Code, target)
	}
	require.Less(t, recorder.maxLevel(), zapcore.ErrorLevel, "missing dist must not log at alert level")
}

// TestRootFileSystemRefusalsAreNotFound verifies rootFileSystem reports every
// path it refuses (a symlink escaping the root, a path through a regular
// file) as not found, like http.Dir does, so a bare http.FileServer over it
// answers 404 instead of 500. Gin's StaticFS pre-check hides the difference
// today, but only as long as nothing changes between its Open and the file
// server's own.
func TestRootFileSystemRefusalsAreNotFound(t *testing.T) {
	_, engine, secret := newSPATraversalFixture(t)
	assetsDir := filepath.Join(spaFixtureDistDir(t, engine), "assets")
	srv := http.FileServer(rootFileSystem{dir: assetsDir})

	for _, target := range []string{"/leak.js", "/leak-rel.js", "/app.js/x", "/missing.js"} {
		t.Run(target, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
			require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
			require.NotContains(t, w.Body.String(), secret)
		})
	}

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/app.js", nil))
	require.Equal(t, http.StatusOK, w.Code, "regular files are still served")

	if os.Geteuid() != 0 { // root ignores file modes
		locked := filepath.Join(assetsDir, "locked.js")
		require.NoError(t, os.WriteFile(locked, []byte("locked()"), 0o000))
		w = httptest.NewRecorder()
		srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/locked.js", nil))
		require.Equal(t, http.StatusForbidden, w.Code, "permission errors keep http.Dir's 403")
	}
}

// TestRootFileSystemSymlinkLoopIsNotFound verifies a symlink loop inside the
// root (a.js -> b.js -> a.js), which os.Root reports as ELOOP, is answered as
// not found rather than as a server error.
func TestRootFileSystemSymlinkLoopIsNotFound(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Symlink("b.js", filepath.Join(dir, "a.js")))
	require.NoError(t, os.Symlink("a.js", filepath.Join(dir, "b.js")))
	srv := http.FileServer(rootFileSystem{dir: dir})

	for _, target := range []string{"/a.js", "/b.js"} {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
		require.Equal(t, http.StatusNotFound, w.Code, target)
	}
}
