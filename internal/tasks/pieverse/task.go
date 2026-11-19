// Package pieverse implements the pieverse health-check alert task.
//
// The task periodically requests a health endpoint and verifies that:
//   - the HTTP status code is 200;
//   - the response body is valid JSON;
//   - the JSON field `state` equals "master".
//
// When any of these checks fail, an ERROR log is emitted; the alert log pusher
// forwards ERROR logs automatically, so no email is sent from here.
//
// The task is DISABLED by default; it only runs when it is explicitly enabled
// via the CMD flag `-t pieverse_alert` (or the env `TASKS=pieverse_alert`).
//
// TLS is fully verified. The default endpoint is served by the purrfect-tee
// private PKI (leaf -> purrfect-tee-issuing-ca -> purrfect-tee-root-ca), so the
// task trusts the system roots plus the embedded purrfect-tee Root CA, which is
// the anchor purrfect-tee relying parties pin. Extra roots (e.g. both Roots
// during a Root rotation) can be added with `tasks.pieverse_alert.root_ca_file`,
// and `tasks.pieverse_alert.server_name` sets the name the certificate is
// verified against when `url` uses an IP literal or another host name.
package pieverse

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

const (
	taskName = "pieverse_alert"

	defaultInterval    = time.Minute
	defaultHTTPTimeout = 10 * time.Second
	defaultURL         = "https://blacknova-enclave-cipherforge-wallet-relay.pieverse.io:8443/health"
	expectedState      = "master"
)

// purrfectTEERootCAPEM is the public purrfect-tee Root CA (CN=purrfect-tee-root-ca,
// ECDSA P-256, valid 2026-05-22..2036-05-19, cert SHA-256
// d4e928312d8b6e4b1a0be8995a0ffab5b24b2adb0597d90be890e32796c1f074). It matches
// the production Root pin published by purrfect-claw-platform.
//
//go:embed purrfect_tee_root_ca.pem
var purrfectTEERootCAPEM []byte

// newRootPool returns the trust anchors for the health check: the system roots,
// the embedded purrfect-tee Root CA and, when extraCAFile is set, every
// certificate in that PEM file.
//
// Parameters:
//   - extraCAFile: optional path of a PEM bundle with additional root CAs.
//
// Returns:
//   - *x509.CertPool: the combined root pool.
//   - error: wrapped error when the embedded root or extraCAFile cannot be used.
func newRootPool(extraCAFile string) (*x509.CertPool, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		// No system roots (e.g. a scratch image): the private root still works.
		log.Logger.Debug("system cert pool unavailable, use private roots only", zap.Error(err))
		pool = x509.NewCertPool()
	}

	if !pool.AppendCertsFromPEM(purrfectTEERootCAPEM) {
		return nil, errors.New("parse embedded purrfect-tee root ca")
	}

	if extraCAFile != "" {
		pem, err := os.ReadFile(extraCAFile)
		if err != nil {
			return nil, errors.Wrapf(err, "read root ca file %q", extraCAFile)
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.Errorf("no PEM certificate found in root ca file %q", extraCAFile)
		}
	}

	return pool, nil
}

// newHTTPClient builds the health-check client with full TLS verification
// (chain and hostname) against newRootPool(extraCAFile).
//
// The client has no Timeout of its own; the per-request deadline is supplied by
// the context in runTask so there is a single authoritative deadline.
//
// Parameters:
//   - extraCAFile: optional path of a PEM bundle with additional root CAs.
//   - serverName: optional name to send as SNI and to verify the certificate
//     against instead of the URL host (e.g. when url uses an IP literal).
//
// Returns:
//   - *http.Client: the verifying client.
//   - error: wrapped error when the root pool cannot be built.
func newHTTPClient(extraCAFile, serverName string) (*http.Client, error) {
	pool, err := newRootPool(extraCAFile)
	if err != nil {
		return nil, errors.Wrap(err, "build root pool")
	}

	// Same bare transport as before (no proxy from env), now verifying TLS.
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: pool,
				// Empty means "use the URL host". Verification is never skipped.
				ServerName: serverName,
				MinVersion: tls.VersionTLS12,
			},
		},
	}, nil
}

// healthChecker runs health checks with a verifying client, or carries the
// configuration error that prevented building one.
type healthChecker struct {
	client    *http.Client
	configErr error
}

// newHealthChecker builds a healthChecker from the TLS trust settings.
//
// Parameters:
//   - extraCAFile: optional path of a PEM bundle with additional root CAs.
//   - serverName: optional TLS server name override.
//
// Returns:
//   - *healthChecker: a checker with a client, or with configErr set when the
//     settings are invalid. It never falls back to skipping verification.
func newHealthChecker(extraCAFile, serverName string) *healthChecker {
	client, err := newHTTPClient(extraCAFile, serverName)
	if err != nil {
		return &healthChecker{configErr: errors.Wrap(err, "invalid TLS trust settings")}
	}

	return &healthChecker{client: client}
}

// check runs one health check against url.
//
// Parameters:
//   - ctx: request deadline and cancellation.
//   - url: health endpoint.
//
// Returns:
//   - error: the configuration error when the checker has no client, otherwise
//     the checkHealth result.
func (h *healthChecker) check(ctx context.Context, url string) error {
	if h.configErr != nil {
		return errors.Wrap(h.configErr, "health check not performed")
	}

	return checkHealth(ctx, h.client, url)
}

// scheduleHealthCheck runs checker every interval. Tests replace it.
var scheduleHealthCheck = func(interval time.Duration, checker *healthChecker) {
	go store.TaskStore.TickerAfterRun(interval, func() { runTask(checker) })
}

// healthResp is the subset of the health response we care about.
type healthResp struct {
	State string `json:"state"`
}

// taskServerName returns the optional TLS server name override from settings.
//
// Returns:
//   - string: configured name, or "" to verify against the URL host.
func taskServerName() string {
	return gconfig.Shared.GetString("tasks.pieverse_alert.server_name")
}

// taskRootCAFile returns the optional extra root CA bundle path from settings.
//
// Returns:
//   - string: configured path, or "" when unset.
func taskRootCAFile() string {
	return gconfig.Shared.GetString("tasks.pieverse_alert.root_ca_file")
}

func taskURL() string {
	if v := gconfig.Shared.GetString("tasks.pieverse_alert.url"); v != "" {
		return v
	}

	return defaultURL
}

func taskInterval() time.Duration {
	// config value is an integer count of seconds, e.g. `interval: 60`.
	if v := gconfig.Shared.GetDuration("tasks.pieverse_alert.interval"); v > 0 {
		return v * time.Second
	}

	return defaultInterval
}

// checkHealth performs the HTTP request with client and validates the response.
//
// It returns a non-nil error describing why the health check failed, or nil
// when the endpoint is healthy.
func checkHealth(ctx context.Context, client *http.Client, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return errors.Wrap(err, "new request")
	}

	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "do request")
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger) //nolint:errcheck,gosec

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "read body")
	}

	// requirement 1: status code must be 200.
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("expect status 200, got %d, body: %s", resp.StatusCode, body)
	}

	// requirement 2: body must be valid JSON.
	if !json.Valid(body) {
		return errors.Errorf("body is not valid json: %s", body)
	}

	// requirement 3: `state` must be "master".
	var data healthResp
	if err := json.Unmarshal(body, &data); err != nil {
		return errors.Wrapf(err, "unexpected health response shape: %s", body)
	}
	if data.State != expectedState {
		return errors.Errorf("expect state %q, got %q", expectedState, data.State)
	}

	return nil
}

// runTask runs one health check with checker and logs the outcome.
func runTask(checker *healthChecker) {
	url := taskURL()
	log.Logger.Debug("run pieverse_alert", zap.String("url", url))

	ctx, cancel := context.WithTimeout(context.Background(), defaultHTTPTimeout)
	defer cancel()

	if err := checker.check(ctx, url); err != nil {
		// Emit ERROR so the alert log pusher forwards it; no email is sent here.
		log.Logger.Error("pieverse health check failed", zap.String("url", url), zap.Error(err))
		return
	}

	log.Logger.Debug("pieverse health check ok", zap.String("url", url))
}

// explicitlyEnabled reports whether the task is explicitly selected via the
// `-t` flag or the `TASKS` env. Unlike the framework default (which runs every
// task when nothing is selected), this task stays off unless it is named.
//
// It intentionally mirrors the `-t`/`TASKS` resolution in the unexported
// store.isTaskEnabled. The `-e`/`exclude` flag is not re-checked here because
// store.Start already enforces it upstream before bindTask runs.
func explicitlyEnabled() bool {
	tasks := gconfig.Shared.GetStringSlice("task")
	if len(tasks) == 0 {
		if env := os.Getenv("TASKS"); env != "" {
			tasks = strings.Split(env, ",")
		}
	}

	return slices.Contains(tasks, taskName)
}

// bindTask binds the pieverse_alert task.
func bindTask() {
	if !explicitlyEnabled() {
		log.Logger.Info("pieverse_alert task is disabled by default, " +
			"enable it with `-t pieverse_alert`")
		return
	}

	// Invalid TLS trust settings do not disable the task: the checker fails
	// closed and every scheduled run reports the error at ERROR level (which
	// the alert pusher forwards), the same channel as a failed health check,
	// so a misconfiguration stays visible each interval.
	checker := newHealthChecker(taskRootCAFile(), taskServerName())

	log.Logger.Info("bind pieverse_alert task...",
		zap.String("url", taskURL()),
		zap.String("server_name", taskServerName()),
		zap.Bool("tls_config_valid", checker.configErr == nil),
		zap.Duration("interval", taskInterval()))
	scheduleHealthCheck(taskInterval(), checker)
}

func init() {
	store.TaskStore.Store(taskName, bindTask)
}
