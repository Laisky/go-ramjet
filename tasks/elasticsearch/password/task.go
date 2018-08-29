package password

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/go-ramjet/tasks/store"
	utils "github.com/Laisky/go-utils"
	"go.uber.org/zap"
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
	step := utils.Settings.GetDuration("tasks.elasticsearch-v2.password.interval")
	if utils.Settings.GetBool("debug") {
		step = 5
	}

	go store.TickerAfterRun(step*time.Second, runTask)
	bindHTTP()
}

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func runTask() {
	newpasswd := GeneratePasswdByDate(utils.UTCNow(), utils.Settings.GetString("tasks.elasticsearch-v2.password.secret"))
	for _, api := range utils.Settings.GetStringSlice("tasks.elasticsearch-v2.password.apis") {
		utils.Logger.Debug("try to change password", zap.String("api", maskAPI(api)))
		user := &User{
			Username: utils.Settings.GetString("tasks.elasticsearch-v2.password.username"),
			Password: newpasswd,
		}
		jb, err := json.Marshal(user)
		if err != nil {
			utils.Logger.Error("try to marshal json got error", zap.Error(err))
			continue
		}

		if utils.Settings.GetBool("dry") {
			utils.Logger.Info("change password via post", zap.String("api", maskAPI(api)), zap.String("password", newpasswd))
			continue
		}

		resp, err := httpClient.Post(api, utils.HTTPJSONHeaderVal, bytes.NewBuffer(jb))
		if err != nil {
			utils.Logger.Error("try to request api got error", zap.String("api", maskAPI(api)), zap.Error(err))
			continue
		}
		if err = utils.CheckResp(resp); err != nil {
			utils.Logger.Error("request api got error", zap.String("api", maskAPI(api)), zap.Error(err))
			continue
		}
	}

	utils.Logger.Info("success changed password")
}

func maskAPI(api string) string {
	return strings.Join(strings.Split(api, "@")[1:], "")
}
