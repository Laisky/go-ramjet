package tasks

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/require"
)

// rendererBrowserTestTask creates a task bound to a controlled synthetic public origin.
func rendererBrowserTestTask(target string, subresources bool) *rlibs.HTMLCrawlerTask {
	return rlibs.NewHTMLCrawlerTaskWithEgress(target, "synthetic-secret-never-forward", false, &rlibs.CrawlerEgressPolicy{Host: "render.example", Addresses: []string{"93.184.216.34"}, MaxRedirects: 2, AllowSubresources: subresources})
}

// TestRendererBrowserHTTPS proves the parent verifies TLS identity before fulfilling a secure browser document.
func TestRendererBrowserHTTPS(t *testing.T) {
	opts := rendererBrowserTestOptions(t)
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "example.com", r.TLS.ServerName)
		require.Equal(t, "example.com", r.Host)
		w.Header().Set("Content-Type", "text/html")
		_, err := w.Write([]byte(`<html><body><div id="result">before</div><script>document.querySelector('#result').textContent=window.isSecureContext?'verified secure render':'insecure'</script></body></html>`))
		require.NoError(t, err)
	}))
	defer srv.Close()
	deps := rendererTestDependencies(t, srv, "93.184.216.34", &hits)
	deps.rootCAs = x509.NewCertPool()
	deps.rootCAs.AddCert(srv.Certificate())
	task := rlibs.NewHTMLCrawlerTaskWithEgress("https://example.com/", "synthetic-secret-never-forward", false, &rlibs.CrawlerEgressPolicy{Host: "example.com", Addresses: []string{"93.184.216.34"}, MaxRedirects: 2, AllowSubresources: false})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	html, receipt, err := renderVerifiedHTMLWithDependencies(ctx, task, deps, opts)
	require.NoError(t, err)
	require.Contains(t, html, "verified secure render")
	require.Len(t, receipt.Origins, 1)
	require.Equal(t, "93.184.216.34", receipt.Origins[0].PeerAddress)
	require.Greater(t, receipt.Origins[0].BodyBytes, int64(0))
}

// rendererBrowserTestOptions locates the installed browser and skips only if it is unavailable.
func rendererBrowserTestOptions(t *testing.T) []chromedp.ExecAllocatorOption {
	t.Helper()
	_, err := exec.LookPath("google-chrome")
	if err != nil {
		t.Skip("google-chrome unavailable for synthetic browser acceptance")
	}
	launcher := filepath.Join(t.TempDir(), "crawler-browser-launcher")
	build := exec.Command("go", "build", "-o", launcher, "../../../../cmd/crawler-browser-launcher")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	output, err := build.CombinedOutput()
	require.NoError(t, err, "build process-only browser launcher: %s", output)
	return []chromedp.ExecAllocatorOption{chromedp.ExecPath(launcher)}
}

// TestRendererBrowserCanonicalDocument proves dynamic rendering with pinned original DNS and normalized URLs.
func TestRendererBrowserCanonicalDocument(t *testing.T) {
	opts := rendererBrowserTestOptions(t)
	for _, target := range []string{"http://render.example#fragment", "http://RENDER.EXAMPLE.:80/"} {
		t.Run(target, func(t *testing.T) {
			var hits, resolutions atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Empty(t, r.Header.Get("Authorization"))
				require.Empty(t, r.Header.Get("Cookie"))
				w.Header().Set("Content-Type", "text/html")
				_, err := w.Write([]byte(`<html><body><div id="result">before</div><img src="http://127.0.0.1/private"><script>setTimeout(()=>document.querySelector('#result').textContent='rendered synthetic',650)</script></body></html>`))
				require.NoError(t, err)
			}))
			defer srv.Close()
			deps := rendererTestDependencies(t, srv, "93.184.216.34", &hits)
			deps.resolve = func(context.Context, string) ([]net.IPAddr, error) {
				resolutions.Add(1)
				return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			html, receipt, err := renderVerifiedHTMLWithDependencies(ctx, rendererBrowserTestTask(target, false), deps, opts)
			require.NoError(t, err)
			require.Contains(t, html, "rendered synthetic")
			require.Len(t, receipt.Origins, 1)
			require.EqualValues(t, 1, hits.Load())
			require.EqualValues(t, 0, resolutions.Load())
			require.Equal(t, target, receipt.TargetURL)
		})
	}
}

// TestRendererBrowserPublicRedirectAndResource verifies actual document and resource connections without automatic redirects.
func TestRendererBrowserPublicRedirectAndResource(t *testing.T) {
	opts := rendererBrowserTestOptions(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Location", "http://redirect.example/final")
			w.WriteHeader(302)
		case "/script.js":
			w.Header().Set("Content-Type", "application/javascript")
			_, err := w.Write([]byte(`document.querySelector('#result').textContent='resource rendered'`))
			require.NoError(t, err)
		case "/favicon.ico":
			w.WriteHeader(204)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, err := w.Write([]byte(`<html><body><div id="result">before</div><script src="http://cdn.example/script.js"></script></body></html>`))
			require.NoError(t, err)
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	html, receipt, err := renderVerifiedHTMLWithDependencies(ctx, rendererBrowserTestTask("http://render.example/", true), rendererTestDependencies(t, srv, "93.184.216.34", &hits), opts)
	require.NoError(t, err)
	require.Contains(t, html, "resource rendered")
	require.GreaterOrEqual(t, len(receipt.Origins), 3)
	require.Equal(t, crawleregress.OriginDocument, receipt.Origins[0].Kind)
	require.Equal(t, crawleregress.OriginDocument, receipt.Origins[1].Kind)
	require.Equal(t, "http://redirect.example/final", receipt.Origins[1].URL)
}

// TestRendererBrowserPrivateRequests checks redirects, iframes, scripts and HTTP sockets cannot contact loopback.
func TestRendererBrowserPrivateRequests(t *testing.T) {
	opts := rendererBrowserTestOptions(t)
	for _, mode := range []string{"redirect", "image", "iframe", "popup", "worker", "websocket", "preconnect", "webtransport"} {
		t.Run(mode, func(t *testing.T) {
			var hits, privateHits atomic.Int32
			private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { privateHits.Add(1); w.WriteHeader(200) }))
			defer private.Close()
			privateURL := private.URL + "/private"
			var body string
			switch mode {
			case "image":
				body = `<img src="` + privateURL + `">`
			case "iframe":
				body = `<iframe src="` + privateURL + `"></iframe>`
			case "popup":
				body = `<script>window.open('` + privateURL + `','_blank')</script>`
			case "worker":
				body = `<script>new Worker('/worker.js')</script>`
			case "websocket":
				body = `<script>new WebSocket('` + strings.Replace(privateURL, "http:", "ws:", 1) + `')</script>`
			case "preconnect":
				body = `<link rel="preconnect" href="` + privateURL + `"><div>safe</div>`
			case "webtransport":
				body = `<script>try {new WebTransport('` + strings.Replace(privateURL, "http:", "https:", 1) + `')}catch(e){}</script>`
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "redirect" {
					w.Header().Set("Location", privateURL)
					w.WriteHeader(302)
					return
				}
				if r.URL.Path == "/worker.js" {
					w.Header().Set("Content-Type", "application/javascript")
					_, err := w.Write([]byte(`fetch('` + privateURL + `')`))
					require.NoError(t, err)
					return
				}
				if r.URL.Path == "/favicon.ico" {
					w.WriteHeader(204)
					return
				}
				w.Header().Set("Content-Type", "text/html")
				_, err := w.Write([]byte("<html><body>" + body + "</body></html>"))
				require.NoError(t, err)
			}))
			defer srv.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _, err := renderVerifiedHTMLWithDependencies(ctx, rendererBrowserTestTask("http://render.example/", true), rendererTestDependencies(t, srv, "93.184.216.34", &hits), opts)
			if mode == "redirect" || mode == "image" || mode == "iframe" {
				require.Error(t, err)
			}
			require.GreaterOrEqual(t, hits.Load(), int32(1), "negative controls must reach and execute the admitted page")
			require.EqualValues(t, 0, privateHits.Load(), "browser route must never hit a private fixture")
		})
	}
}

// TestRendererBrowserReachableSentinelNegativeControl proves Chrome can reach the loopback sentinel when proxy bypass is allowed.
func TestRendererBrowserReachableSentinelNegativeControl(t *testing.T) {
	opts := rendererBrowserTestOptions(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/html")
		_, err := w.Write([]byte("<html><body>reachable sentinel</body></html>"))
		require.NoError(t, err)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	allocatorOptions := append(rendererBrowserOptions("127.0.0.1:9"), chromedp.Flag("proxy-bypass-list", "127.0.0.1"))
	allocatorOptions = append(allocatorOptions, opts...)
	// This deliberate local negative control omits the process sandbox and enables loopback bypass.
	allocatorOptions = append(allocatorOptions, chromedp.ExecPath("/usr/bin/google-chrome"))
	allocatorCtx, stopAllocator := chromedp.NewExecAllocator(ctx, allocatorOptions...)
	defer stopAllocator()
	browserCtx, stopBrowser := chromedp.NewContext(allocatorCtx)
	defer stopBrowser()
	require.NoError(t, chromedp.Do(browserCtx, chromedp.Navigate(srv.URL)))
	require.GreaterOrEqual(t, hits.Load(), int32(1))
}

// TestRendererBrowserDelayedBody ensures completion waits for admitted async response bodies before sealing.
func TestRendererBrowserDelayedBody(t *testing.T) {
	opts := rendererBrowserTestOptions(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			time.Sleep(400 * time.Millisecond)
			_, err := w.Write([]byte("delayed complete"))
			require.NoError(t, err)
			return
		}
		if r.URL.Path == "/favicon.ico" {
			w.WriteHeader(204)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, err := w.Write([]byte(`<html><body><div id="result">before</div><script>fetch('/slow').then(r=>r.text()).then(t=>document.querySelector('#result').textContent=t)</script></body></html>`))
		require.NoError(t, err)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	html, receipt, err := renderVerifiedHTMLWithDependencies(ctx, rendererBrowserTestTask("http://render.example/", true), rendererTestDependencies(t, srv, "93.184.216.34", &hits), opts)
	require.NoError(t, err)
	require.Contains(t, html, "delayed complete")
	var found bool
	for _, origin := range receipt.Origins {
		if strings.HasSuffix(origin.URL, "/slow") {
			found = true
			require.EqualValues(t, len("delayed complete"), origin.BodyBytes)
		}
	}
	require.True(t, found)
}

// TestRendererBrowserNonProxyUDP verifies browser WebRTC cannot send STUN directly to a local UDP sentinel.
func TestRendererBrowserNonProxyUDP(t *testing.T) {
	opts := rendererBrowserTestOptions(t)
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	defer udp.Close()
	port := strconv.Itoa(udp.LocalAddr().(*net.UDPAddr).Port)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, err := fmt.Fprintf(w, `<html><body>safe<script>let p=new RTCPeerConnection({iceServers:[{urls:'stun:127.0.0.1:%s'}]});p.createDataChannel('x');p.createOffer().then(o=>p.setLocalDescription(o));</script></body></html>`, port)
		require.NoError(t, err)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	html, _, err := renderVerifiedHTMLWithDependencies(ctx, rendererBrowserTestTask("http://render.example/", false), rendererTestDependencies(t, srv, "93.184.216.34", &hits), opts)
	require.NoError(t, err)
	require.Contains(t, html, "safe")
	require.GreaterOrEqual(t, hits.Load(), int32(1))
	require.NoError(t, udp.SetReadDeadline(time.Now().Add(100*time.Millisecond)))
	_, _, err = udp.ReadFromUDP(make([]byte, 2048))
	require.Error(t, err)
	var netErr net.Error
	require.ErrorAs(t, err, &netErr)
	require.True(t, netErr.Timeout())
}

// TestRendererBrowserUDPReachableNegativeControl shows the identical ICE fixture sends UDP without the process sandbox.
func TestRendererBrowserUDPReachableNegativeControl(t *testing.T) {
	opts := append(rendererBrowserTestOptions(t), chromedp.ExecPath("/usr/bin/google-chrome"))
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	defer udp.Close()
	port := strconv.Itoa(udp.LocalAddr().(*net.UDPAddr).Port)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, err := fmt.Fprintf(w, `<html><body>control<script>let p=new RTCPeerConnection({iceServers:[{urls:'stun:127.0.0.1:%s'}]});p.createDataChannel('x');p.createOffer().then(o=>p.setLocalDescription(o));</script></body></html>`, port)
		require.NoError(t, err)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	html, _, err := renderVerifiedHTMLWithDependencies(ctx, rendererBrowserTestTask("http://render.example/", false), rendererTestDependencies(t, srv, "93.184.216.34", &hits), opts)
	require.NoError(t, err)
	require.Contains(t, html, "control")
	require.NoError(t, udp.SetReadDeadline(time.Now().Add(time.Second)))
	count, _, err := udp.ReadFromUDP(make([]byte, 2048))
	require.NoError(t, err)
	require.Greater(t, count, 0)
}

// TestRendererBrowserNonProxyTCP checks a native TURN TCP route cannot contact an Internet socket sentinel.
func TestRendererBrowserNonProxyTCP(t *testing.T) {
	opts := rendererBrowserTestOptions(t)
	tcp, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	defer tcp.Close()
	port := strconv.Itoa(tcp.Addr().(*net.TCPAddr).Port)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, err := fmt.Fprintf(w, `<html><body>tcp control<script>let p=new RTCPeerConnection({iceServers:[{urls:'turn:127.0.0.1:%s?transport=tcp',username:'synthetic',credential:'synthetic'}]});p.createDataChannel('x');p.createOffer().then(o=>p.setLocalDescription(o));</script></body></html>`, port)
		require.NoError(t, err)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	html, _, err := renderVerifiedHTMLWithDependencies(ctx, rendererBrowserTestTask("http://render.example/", false), rendererTestDependencies(t, srv, "93.184.216.34", &hits), opts)
	require.NoError(t, err)
	require.Contains(t, html, "tcp control")
	require.GreaterOrEqual(t, hits.Load(), int32(1))
	require.NoError(t, tcp.SetDeadline(time.Now().Add(100*time.Millisecond)))
	connection, err := tcp.AcceptTCP()
	if connection != nil {
		require.NoError(t, connection.Close())
	}
	require.Error(t, err)
	var netErr net.Error
	require.ErrorAs(t, err, &netErr)
	require.True(t, netErr.Timeout())
}
