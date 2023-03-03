package alias

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v4"
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

func BindAliasesTask() {
	step := gconfig.Shared.GetDuration("tasks.elasticsearch-v2.aliases.interval")
	if gconfig.Shared.GetBool("debug") {
		step = 5
	}

	go store.TaskStore.TickerAfterRun(step*time.Second, runTask)
}

func runTask() {
	log.Logger.Info("run elasticsearch.alias")
	var (
		err   error
		alias string
	)
	aliases := gconfig.Shared.Get("tasks.elasticsearch-v2.aliases.aliases").(map[string]interface{})
	api := gconfig.Shared.GetString("tasks.elasticsearch-v2.aliases.api")
	for index, aliasI := range aliases {
		alias = aliasI.(string)
		if err = createAlias(api, index, alias); err != nil {
			log.Logger.Error("failed to refresh aliases",
				zap.String("api", maskAPI(api)),
				zap.String("index", index),
				zap.String("alias", alias),
				zap.Error(err))
		} else {
			log.Logger.Info("success refresh alias",
				zap.String("index", index),
				zap.String("alias", alias))
		}
	}
}

func createAlias(api, index, alias string) error {
	data := map[string]interface{}{
		"actions": []interface{}{
			map[string]interface{}{
				"add": map[string]interface{}{
					"index": index,
					"alias": alias,
				},
			},
		},
	}
	reqJB, err := json.Marshal(data)
	log.Logger.Debug("post", zap.ByteString("body", reqJB))
	if err != nil {
		log.Logger.Error("try to marshal json got error", zap.Error(err))
	}

	if gconfig.Shared.GetBool("dry") {
		log.Logger.Info("refresh aliases via post",
			zap.String("api", maskAPI(api)),
			zap.String("index", index),
			zap.String("alias", alias))
		return nil
	}

	resp, err := httpClient.Post(api, gutils.HTTPHeaderContentTypeValJSON, bytes.NewReader(reqJB))
	if err != nil {
		log.Logger.Error("try to request api got error",
			zap.String("api", maskAPI(api)),
			zap.String("index", index),
			zap.String("alias", alias),
			zap.Error(err))
		return err
	}
	defer gutils.SilentClose(resp.Body)
	log.Logger.Debug("got response code", zap.Int("code", resp.StatusCode))
	if err = gutils.CheckResp(resp); err != nil {
		log.Logger.Error("request api got error",
			zap.String("api", maskAPI(api)),
			zap.String("index", index),
			zap.String("alias", alias),
			zap.Error(err))
		return err
	}

	return nil
}

func maskAPI(api string) string {
	return strings.Join(strings.Split(api, "@")[1:], "")
}
