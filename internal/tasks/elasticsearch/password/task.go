package password

import (
	"bytes"
	"context"
	"net/http"
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
	httpClient = http.Client{
		Timeout: time.Second * 30,
		Transport: &http.Transport{
			MaxIdleConns:        3,
			MaxIdleConnsPerHost: 3,
		},
	}
)

// BindPasswordTask schedules runTask at the configured interval in seconds and
// registers the HTTP endpoint that serves the current password.
func BindPasswordTask() {
	step := gconfig.Shared.GetDuration("tasks.elasticsearch-v2.password.interval")
	if gconfig.Shared.GetBool("debug") {
		step = 5
	}

	if step == 0 { // no config
		step = 1
	}

	go store.TaskStore.TickerAfterRun(step*time.Second, runTask)
	bindHTTP()
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
// when every API accepted the change.
func runTask() {
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
	failed := 0
	for i, api := range apis {
		// MaskURLPassword also rejects URLs that net/url cannot parse. Its error
		// never quotes the URL, unlike the parse error an HTTP request would
		// return, so the admin credentials stay out of the logs.
		maskedAPI, err := gutils.MaskURLPassword(api, apiPasswordMask)
		if err != nil {
			log.Logger.Error("skip invalid elasticsearch api",
				zap.Int("index", i),
				zap.Error(err))
			failed++
			continue
		}
		log.Logger.Debug("try to change password", zap.String("api", maskedAPI))

		if gconfig.Shared.GetBool("dry") {
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

	if failed > 0 {
		log.Logger.Error("failed to change password on some apis",
			zap.Int("failed", failed),
			zap.Int("total", len(apis)))
		return
	}
	log.Logger.Info("success changed password")
}

// postPassword posts body, the JSON user with its new password, to api and
// returns an error unless Elasticsearch accepted it. api must already have
// passed gutils.MaskURLPassword. Transport errors are safe to return because
// net/http masks the URL password in them.
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
