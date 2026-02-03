package password

import (
	"bytes"
	"net/http"
	"strings"
	"time"

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

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func runTask() {
	log.Logger.Info("run elasticsearch.password")
	newpasswd := GeneratePasswdByDate(
		gutils.UTCNow(),
		gconfig.Shared.GetString("tasks.elasticsearch-v2.password.secret"))
	for _, api := range gconfig.Shared.GetStringSlice("tasks.elasticsearch-v2.password.apis") {
		log.Logger.Debug("try to change password", zap.String("api", maskAPI(api)))
		user := &User{
			Username: gconfig.Shared.GetString("tasks.elasticsearch-v2.password.username"),
			Password: newpasswd,
		}
		jb, err := json.Marshal(user)
		if err != nil {
			log.Logger.Error("try to marshal json got error",
				zap.Error(err))
			continue
		}

		if gconfig.Shared.GetBool("dry") {
			log.Logger.Info("change password via post",
				zap.String("api", maskAPI(api)),
				zap.String("password", newpasswd))
			continue
		}

		resp, err := httpClient.Post(api,
			gutils.HTTPHeaderContentTypeValJSON,
			bytes.NewReader(jb))
		if err != nil {
			log.Logger.Error("try to request api got error",
				zap.String("api", maskAPI(api)),
				zap.Error(err))
			continue
		}
		defer gutils.LogErr(resp.Body.Close, log.Logger) // nolint: errcheck,gosec

		if err = gutils.CheckResp(resp); err != nil {
			log.Logger.Error("request api got error",
				zap.String("api", maskAPI(api)),
				zap.Error(err))
			continue
		}
	}

	log.Logger.Info("success changed password")
}

func maskAPI(api string) string {
	return strings.Join(strings.Split(api, "@")[1:], "")
}
