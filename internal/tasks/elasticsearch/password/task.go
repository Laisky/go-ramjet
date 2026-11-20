package password

import (
	"bytes"
	"context"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/go-utils/v6/json"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

var (
	rotationMu sync.Mutex
	httpClient = http.Client{
		Timeout: time.Second * 30,
		Transport: &http.Transport{
			MaxIdleConns:        3,
			MaxIdleConnsPerHost: 3,
		},
	}
)

// defaultRotationInterval allows a missed month-boundary tick to retry within a minute.
const defaultRotationInterval = time.Minute

// BindPasswordTask starts the password rotation schedule when its APIs are configured.
func BindPasswordTask() {
	bindPasswordTask(func(interval time.Duration, rotate func()) {
		go store.TaskStore.TickerAfterRun(interval, rotate)
	})
}

// bindPasswordTask hands the rotation callback and interval to schedule only when APIs are configured.
func bindPasswordTask(schedule func(time.Duration, func())) {
	if len(gconfig.Shared.GetStringSlice("tasks.elasticsearch-v2.password.apis")) == 0 {
		log.Logger.Info("elasticsearch password rotation not scheduled: no APIs configured")
		return
	}

	// The existing configuration key is expressed as a number of seconds.
	seconds := gconfig.Shared.GetDuration("tasks.elasticsearch-v2.password.interval")
	interval := defaultRotationInterval
	switch {
	case gconfig.Shared.GetBool("debug"):
		interval = 5 * time.Second
	case seconds > 0 && seconds <= math.MaxInt64/time.Second:
		interval = seconds * time.Second
	}

	schedule(interval, runTask)
}

// User is the body of the Elasticsearch change-password request.
type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// apiPasswordMask replaces the admin password of an API URL in logs, matching
// the masking used by the rollover task.
const apiPasswordMask = "*****"

// runTask sets this month's generated password for the configured user on
// every configured Elasticsearch API. It logs each API only with its admin
// password masked, never logs the generated password, and reports success only
// when every API accepted the change. A dry run, or a run with no API
// configured, changes nothing and says so instead of reporting success.
func runTask() {
	// The task store dispatches asynchronously; cover the entire rotation, not just dispatch.
	if !rotationMu.TryLock() {
		log.Logger.Debug("skip elasticsearch password rotation: already running")
		return
	}
	defer rotationMu.Unlock()

	log.Logger.Info("run elasticsearch.password")
	newpasswd := GeneratePasswdByDate(
		gutils.UTCNow(),
		gconfig.Shared.GetString("tasks.elasticsearch-v2.password.secret"))
	ctx := context.Background()
	user := &User{
		Username: gconfig.Shared.GetString("tasks.elasticsearch-v2.password.username"),
		Password: newpasswd,
	}
	jb, err := json.Marshal(user)
	if err != nil {
		log.Logger.Error("try to marshal json got error", zap.Error(err))
		return
	}

	apis := gconfig.Shared.GetStringSlice("tasks.elasticsearch-v2.password.apis")
	dry := gconfig.Shared.GetBool("dry")
	failed := 0
	for i, api := range apis {
		// An API that logSafeAPI rejects is never requested, and is reported by
		// index only because any part of it may hold the admin credentials.
		maskedAPI, err := logSafeAPI(api)
		if err != nil {
			log.Logger.Error("skip invalid elasticsearch api",
				zap.Int("index", i),
				zap.Error(err))
			failed++
			continue
		}
		log.Logger.Debug("try to change password", zap.String("api", maskedAPI))

		if dry {
			// Never log the generated password, not even in a dry run.
			log.Logger.Info("change password via post",
				zap.String("api", maskedAPI),
				zap.String("username", user.Username))
			continue
		}

		if err = postPassword(ctx, api, jb); err != nil {
			log.Logger.Error("change elasticsearch password",
				zap.String("api", maskedAPI),
				zap.Error(err))
			failed++
			continue
		}
	}

	switch {
	case failed > 0:
		log.Logger.Error("failed to change password on some apis",
			zap.Int("failed", failed),
			zap.Int("total", len(apis)))
	case len(apis) == 0:
		log.Logger.Info("no elasticsearch api configured, no password changed")
	case dry:
		log.Logger.Info("dry run finished, no password changed",
			zap.Int("total", len(apis)))
	default:
		log.Logger.Info("success changed password")
	}
}

// logSafeAPI returns api with its password masked for logging. It returns an
// error that never quotes api when net/url cannot parse api, when api has no
// host, or when a raw '@' appears after the authority. The last case means a
// '/', '?' or '#' in the credentials ended the authority early, so net/url
// read no userinfo, or the wrong one, and left part of the password in the
// host, path, query or fragment, where no masking applies; such an API would
// also be requested on the wrong host. An '@' outside the credentials, such as
// in a username in the path, must be written as %40.
func logSafeAPI(api string) (string, error) {
	parsed, err := url.Parse(api)
	if err != nil {
		// The cause quotes api with its credentials, so it is deliberately not wrapped.
		return "", errors.New("api is not a valid URL")
	}
	if strings.Contains(parsed.EscapedPath()+parsed.RawQuery+parsed.EscapedFragment(), "@") {
		return "", errors.New("api has a raw '@' after its host; " +
			"percent-encode '/', '?', '#' and '@' in the credentials, and '@' elsewhere, as in RFC 3986")
	}
	masked, err := gutils.MaskURLPassword(api, apiPasswordMask)
	if err != nil {
		return "", errors.Wrap(err, "mask api password")
	}
	return masked, nil
}

// postPassword posts body, the JSON user with its new password, to api and
// returns an error unless Elasticsearch accepted it. api must already have
// passed logSafeAPI, so any credentials in it were parsed as userinfo, and
// net/http masks that password in the transport errors returned here. The
// response body is closed before returning, so each API's connection is
// released before the next API is requested.
func postPassword(ctx context.Context, api string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api, bytes.NewReader(body))
	if err != nil {
		// The cause quotes api with its credentials, so it is deliberately not wrapped.
		return errors.New("build change password request")
	}
	req.Header.Set(gutils.HTTPHeaderContentType, gutils.HTTPHeaderContentTypeValJSON)

	resp, err := httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "post new password")
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger) // nolint: errcheck,gosec

	if err = gutils.CheckResp(resp); err != nil {
		return errors.Wrap(err, "check change password response")
	}
	return nil
}
