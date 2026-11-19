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
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/json"
	"encoding/pem"
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
		data, err := os.ReadFile(extraCAFile)
		if err != nil {
			return nil, errors.Wrapf(err, "read root ca file %q", extraCAFile)
		}
		if err = appendRootCAs(pool, data); err != nil {
			return nil, errors.Wrapf(err, "root ca file %q", extraCAFile)
		}
	}

	return pool, nil
}

// appendRootCAs adds every CERTIFICATE block of the PEM bundle data to pool.
//
// Unlike x509.CertPool.AppendCertsFromPEM, which succeeds as soon as one
// certificate parses, it rejects a bundle with an unparsable certificate or a
// malformed or incomplete block (pem.Decode silently skips those), e.g. one
// read while it was being rewritten, so a damaged bundle fails closed instead
// of silently trusting fewer roots. Text outside blocks and non-certificate
// blocks are ignored, as in system bundles.
//
// Parameters:
//   - pool: pool to add the certificates to.
//   - data: PEM bundle.
//
// Returns:
//   - error: why the bundle is unusable, or nil when at least one certificate
//     was added and nothing was damaged.
func appendRootCAs(pool *x509.CertPool, data []byte) error {
	added, decoded := 0, 0
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		decoded++
		if block.Type != "CERTIFICATE" {
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return errors.Wrapf(err, "parse certificate %d", added+1)
		}
		pool.AddCert(cert)
		added++
	}

	if begins := bytes.Count(data, []byte("-----BEGIN ")); begins != decoded {
		return errors.Errorf("malformed or incomplete PEM block: %d of %d decoded (file partially written?)",
			decoded, begins)
	}
	if added == 0 {
		return errors.New("no PEM certificate found")
	}

	return nil
}

// newHTTPClient builds the health-check client with full TLS verification
// (chain and hostname) against newRootPool(extraCAFile).
//
// The client has no Timeout of its own; the per-request deadline is supplied by
// the context in runTask so there is a single authoritative deadline.
// Keep-alives are off, so every request performs (and verifies) its own TLS
// handshake and the client leaves no idle connection behind.
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
			DisableKeepAlives: true,
			TLSClientConfig: &tls.Config{
				RootCAs: pool,
				// Empty means "use the URL host". Verification is never skipped.
				ServerName: serverName,
				MinVersion: tls.VersionTLS12,
			},
		},
	}, nil
}

// healthChecker runs health checks that verify TLS against its trust
// settings. It is immutable, so concurrent runs cannot race on it.
type healthChecker struct {
	extraCAFile string
	serverName  string
}

// newHealthChecker builds a healthChecker from the TLS trust settings.
//
// Parameters:
//   - extraCAFile: optional path of a PEM bundle with additional root CAs.
//   - serverName: optional TLS server name override.
//
// Returns:
//   - *healthChecker: the checker; settings are validated on every check.
func newHealthChecker(extraCAFile, serverName string) *healthChecker {
	return &healthChecker{extraCAFile: extraCAFile, serverName: serverName}
}

// check builds a verifying client and runs one health check against url.
//
// The client is rebuilt on every run, so extraCAFile is re-read: a fixed or
// rotated bundle applies at the next run without a restart, while an unusable
// one fails this run closed and never falls back to skipping verification.
//
// Parameters:
//   - ctx: request deadline and cancellation.
//   - url: health endpoint.
//
// Returns:
//   - error: the trust-settings error when no client can be built, otherwise
//     the checkHealth result.
func (h *healthChecker) check(ctx context.Context, url string) error {
	client, err := newHTTPClient(h.extraCAFile, h.serverName)
	if err != nil {
		return errors.Wrap(err, "health check not performed: invalid TLS trust settings")
	}

	return checkHealth(ctx, client, url)
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

	// Invalid TLS trust settings do not disable the task: every scheduled run
	// (the first one immediately) fails closed and reports the error at ERROR
	// level (which the alert pusher forwards), the same channel as a failed
	// health check, until the root_ca_file is fixed.
	checker := newHealthChecker(taskRootCAFile(), taskServerName())

	log.Logger.Info("bind pieverse_alert task...",
		zap.String("url", taskURL()),
		zap.String("server_name", checker.serverName),
		zap.String("root_ca_file", checker.extraCAFile),
		zap.Duration("interval", taskInterval()))
	scheduleHealthCheck(taskInterval(), checker)
}

func init() {
	store.TaskStore.Store(taskName, bindTask)
}
