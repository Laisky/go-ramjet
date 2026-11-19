package password

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

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

// TestRunTaskNeverLogsCredentialsOfMalformedAPI guards the error path for an
// API URL that net/url rejects. Its parse error quotes the whole URL, so the
// task must report the bad entry without echoing any part of its credentials.
func TestRunTaskNeverLogsCredentialsOfMalformedAPI(t *testing.T) {
	configureTask(t, false, "http://elastic:bad%zz-secret@127.0.0.1:1/_security/user/elastic/_password")
	entries := captureLogs(t)

	runTask()

	require.NotEmpty(t, entries.FilterLevelExact(zapcore.ErrorLevel).All(), "the malformed API must be reported")
	requireNoSecretsLogged(t, entries, "zz-secret", GeneratePasswdByDate(gutils.UTCNow(), testSecret))
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
