package password

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/go-ramjet/tasks/store"
	gutils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
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
	step := gutils.Settings.GetDuration("tasks.elasticsearch-v2.password.interval")
	if gutils.Settings.GetBool("debug") {
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
	gutils.Logger.Info("run elasticsearch.password")
	newpasswd := GeneratePasswdByDate(gutils.UTCNow(), gutils.Settings.GetString("tasks.elasticsearch-v2.password.secret"))
	for _, api := range gutils.Settings.GetStringSlice("tasks.elasticsearch-v2.password.apis") {
		gutils.Logger.Debug("try to change password", zap.String("api", maskAPI(api)))
		user := &User{
			Username: gutils.Settings.GetString("tasks.elasticsearch-v2.password.username"),
			Password: newpasswd,
		}
		jb, err := json.Marshal(user)
		if err != nil {
			gutils.Logger.Error("try to marshal json got error", zap.Error(err))
			continue
		}

		if gutils.Settings.GetBool("dry") {
			gutils.Logger.Info("change password via post", zap.String("api", maskAPI(api)), zap.String("password", newpasswd))
			continue
		}

		resp, err := httpClient.Post(api, gutils.HTTPHeaderContentTypeValJSON, bytes.NewReader(jb))
		if err != nil {
			gutils.Logger.Error("try to request api got error", zap.String("api", maskAPI(api)), zap.Error(err))
			continue
		}
		defer resp.Body.Close()
		if err = gutils.CheckResp(resp); err != nil {
			gutils.Logger.Error("request api got error", zap.String("api", maskAPI(api)), zap.Error(err))
			continue
		}
	}

	gutils.Logger.Info("success changed password")
}

func maskAPI(api string) string {
	return strings.Join(strings.Split(api, "@")[1:], "")
}
