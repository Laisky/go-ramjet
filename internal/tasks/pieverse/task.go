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
package pieverse

import (
	"context"
	"crypto/tls"
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

// httpClient has no Timeout of its own; the per-request deadline is supplied by
// the context in runTask so there is a single authoritative deadline.
var httpClient = &http.Client{
	Transport: &http.Transport{
		// The health endpoint is served with a self-signed / non-standard cert
		// (the manual check uses `curl -k`), so skip verification to mirror it.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // intended, mirrors `curl -k`
	},
}

// healthResp is the subset of the health response we care about.
type healthResp struct {
	State string `json:"state"`
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

// checkHealth performs the HTTP request and validates the response.
//
// It returns a non-nil error describing why the health check failed, or nil
// when the endpoint is healthy.
func checkHealth(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return errors.Wrap(err, "new request")
	}

	resp, err := httpClient.Do(req)
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

func runTask() {
	url := taskURL()
	log.Logger.Debug("run pieverse_alert", zap.String("url", url))

	ctx, cancel := context.WithTimeout(context.Background(), defaultHTTPTimeout)
	defer cancel()

	if err := checkHealth(ctx, url); err != nil {
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

	log.Logger.Info("bind pieverse_alert task...",
		zap.String("url", taskURL()),
		zap.Duration("interval", taskInterval()))
	go store.TaskStore.TickerAfterRun(taskInterval(), runTask)
}

func init() {
	store.TaskStore.Store(taskName, bindTask)
}
