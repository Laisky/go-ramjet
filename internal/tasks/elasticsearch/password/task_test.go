package password

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/go-utils/v6/json"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/log"
)

const (
	// testSecret seeds the generated password in tests.
	testSecret = "task-test-secret"
	// adminPassword is the Elasticsearch admin password carried in the API URL.
	// The raw '@' is accepted by net/url and used to split the old masking apart.
	adminPassword = "hunter@2-secret"
)

// setConfig sets key for the duration of t and restores its previous value.
func setConfig(t *testing.T, key string, value any) {
	t.Helper()
	previous := gconfig.Shared.Get(key)
	gconfig.Shared.Set(key, value)
	t.Cleanup(func() { gconfig.Shared.Set(key, previous) })
}

// configureTask points the password task at apis with the test secret and the
// given dry mode for the duration of t.
func configureTask(t *testing.T, dry bool, apis ...string) {
	t.Helper()
	setConfig(t, "dry", dry)
	setConfig(t, "tasks.elasticsearch-v2.password.secret", testSecret)
	setConfig(t, "tasks.elasticsearch-v2.password.username", "elastic")
	setConfig(t, "tasks.elasticsearch-v2.password.apis", apis)
}

// captureLogs replaces the package logger with an observer for the duration of
// t and returns the recorded entries.
func captureLogs(t *testing.T) *observer.ObservedLogs {
	t.Helper()
	core, entries := observer.New(zapcore.DebugLevel)
	logger, err := glog.New(glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
	require.NoError(t, err)
	old := log.Logger
	log.Logger = logger
	t.Cleanup(func() { log.Logger = old })
	return entries
}

// requireNoSecretsLogged fails t when any recorded entry carries a password
// field or contains one of secrets in its message or field values.
func requireNoSecretsLogged(t *testing.T, entries *observer.ObservedLogs, secrets ...string) {
	t.Helper()
	for _, entry := range entries.All() {
		for _, secret := range secrets {
			require.NotContains(t, entry.Message, secret)
		}
		for key, value := range entry.ContextMap() {
			require.NotContains(t, strings.ToLower(key), "password", "entry %q must not carry a password field", entry.Message)
			for _, secret := range secrets {
				require.NotContains(t, fmt.Sprint(value), secret, "entry %q field %q", entry.Message, key)
			}
		}
	}
}

// adminAPI returns the change-password API on server with the admin
// credentials embedded in the URL, the way the task is configured.
func adminAPI(server *httptest.Server) string {
	return strings.Replace(server.URL, "http://", "http://elastic:"+adminPassword+"@", 1) +
		"/_security/user/elastic/_password"
}

// TestRunTaskChangesPasswordWithoutLoggingSecrets runs the task against a
// loopback Elasticsearch stub. The admin credentials and the new password must
// reach the server, but neither may appear in any log entry.
func TestRunTaskChangesPasswordWithoutLoggingSecrets(t *testing.T) {
	var calls atomic.Int32
	var gotUser User
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		username, password, ok := r.BasicAuth()
		if !ok || username != "elastic" || password != adminPassword {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotUser); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	configureTask(t, false, adminAPI(server))
	entries := captureLogs(t)
	newPassword := GeneratePasswdByDate(gutils.UTCNow(), testSecret)

	runTask()

	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, User{Username: "elastic", Password: newPassword}, gotUser)
	require.NotEmpty(t, entries.FilterMessage("success changed password").All())
	requireNoSecretsLogged(t, entries, newPassword, "hunter", "2-secret")
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip calls f with req and returns its response and error.
func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// useTransport makes the task's HTTP client send through rt for the duration of t.
func useTransport(t *testing.T, rt http.RoundTripper) {
	t.Helper()
	old := httpClient.Transport
	httpClient.Transport = rt
	t.Cleanup(func() { httpClient.Transport = old })
}

// TestRunTaskNeverLogsCredentialsOfMalformedAPI guards API URLs whose
// credentials net/url cannot read as userinfo. A bad escape makes the parse
// error quote the whole URL. A raw '/', '?' or '#' in the password ends the
// authority early, so net/url parses no userinfo (or the wrong one) and puts
// the rest of the password in the host, path, query or fragment, where no
// masking applies. An API without a scheme keeps its credentials in the opaque
// part. The task must skip such an API before any request, and
// report it by index without echoing any part of it, in dry and real runs.
func TestRunTaskNeverLogsCredentialsOfMalformedAPI(t *testing.T) {
	const path = "/_security/user/elastic/_password"
	apis := []string{
		"http://elastic:bad%zzSECRETTAIL@127.0.0.1:1" + path,
		"https://elastic:12?SECRETTAIL@es.example" + path,
		"https://elastic:/SECRETTAIL@es.example" + path,
		"https://elastic:12#SECRETTAIL@es.example" + path,
		"https://elastic:p@ss/SECRETTAIL@es.example" + path,
		// Without a scheme, net/url reads "elastic" as the scheme and keeps
		// the credentials in the opaque part, where no masking applies.
		"elastic:SECRETTAIL@es.example:9200" + path,
	}
	for i, api := range apis {
		for _, dry := range []bool{true, false} {
			t.Run(fmt.Sprintf("api%d/dry=%t", i, dry), func(t *testing.T) {
				var sent atomic.Int32
				useTransport(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
					sent.Add(1)
					return nil, errors.New("offline test transport")
				}))
				configureTask(t, dry, api)
				entries := captureLogs(t)

				runTask()

				require.Zero(t, sent.Load(), "an API whose credentials did not parse must not be requested")
				require.Len(t, entries.FilterMessage("skip invalid elasticsearch api").All(), 1)
				require.Empty(t, entries.FilterMessage("success changed password").All())
				requireNoSecretsLogged(t, entries, "SECRETTAIL", "es.example", "127.0.0.1",
					GeneratePasswdByDate(gutils.UTCNow(), testSecret))
			})
		}
	}
}

// TestLogSafeAPIAcceptsEncodedAt pins that an '@' written as %40 outside the
// credentials is still accepted, so a username such as an email address can
// appear in the change-password path, while the password stays masked.
func TestLogSafeAPIAcceptsEncodedAt(t *testing.T) {
	masked, err := logSafeAPI("https://elastic:p@ss@es.example/_security/user/ops%40corp/_password")
	require.NoError(t, err)
	require.Equal(t, "https://elastic:*****@es.example/_security/user/ops%40corp/_password", masked)
}

// closeTracker reports the first Close of the wrapped response body.
type closeTracker struct {
	io.ReadCloser
	once    sync.Once
	onClose func()
}

// Close closes the wrapped body and calls onClose once.
func (c *closeTracker) Close() error {
	c.once.Do(c.onClose)
	return c.ReadCloser.Close()
}

// TestRunTaskClosesEachResponseBeforeTheNextAPI guards against holding every
// response body, and its connection, open until the whole run ends: each
// API's response must be closed before the next API is requested.
func TestRunTaskClosesEachResponseBeforeTheNextAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"acknowledged":true}`))
	}))
	t.Cleanup(server.Close)

	base := &http.Transport{}
	t.Cleanup(base.CloseIdleConnections)
	var mu sync.Mutex
	open := 0
	var openAtRequest []int
	useTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		openAtRequest = append(openAtRequest, open)
		mu.Unlock()
		resp, err := base.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		mu.Lock()
		open++
		mu.Unlock()
		resp.Body = &closeTracker{ReadCloser: resp.Body, onClose: func() {
			mu.Lock()
			open--
			mu.Unlock()
		}}
		return resp, nil
	}))
	configureTask(t, false, adminAPI(server), adminAPI(server), adminAPI(server))
	entries := captureLogs(t)

	runTask()

	require.NotEmpty(t, entries.FilterMessage("success changed password").All())
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []int{0, 0, 0}, openAtRequest, "open response bodies when each API was requested")
	require.Zero(t, open, "every response body must be closed by the end of the run")
}

// TestRunTaskDoesNotReportSuccessAfterFailure guards against announcing a
// successful rotation when Elasticsearch rejected the new password.
func TestRunTaskDoesNotReportSuccessAfterFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	configureTask(t, false, adminAPI(server))
	entries := captureLogs(t)

	runTask()

	require.Empty(t, entries.FilterMessage("success changed password").All())
	require.NotEmpty(t, entries.FilterLevelExact(zapcore.ErrorLevel).All(), "the rejected change must be reported")
	requireNoSecretsLogged(t, entries, "hunter", "2-secret")
}
