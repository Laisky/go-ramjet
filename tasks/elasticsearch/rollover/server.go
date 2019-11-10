package rollover

import (
	"encoding/json"
	"fmt"

	"github.com/Laisky/go-ramjet"
	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/kataras/iris/v12"
)

type idxDetail struct {
	Name    string `json:"index-name"`
	Expires string `json:"index-expires"`
}

func bindHTTP() {
	var (
		stI     map[interface{}]interface{}
		details = []*idxDetail{}
	)
	for _, sts := range utils.Settings.Get("tasks.elasticsearch-v2.configs").([]interface{}) {
		stI = sts.(map[interface{}]interface{})
		if stI["action"].(string) != "rollover" {
			continue
		}

		details = append(details, &idxDetail{
			Name:    stI["index-alias"].(string),
			Expires: fmt.Sprintf("%vhrs", stI["expires"].(int)/3600),
		})
	}

	utils.Logger.Info("bind HTTP GET `/es/rollover`")
	ramjet.Server.Get("/es/rollover", func(ctx iris.Context) {
		jb, err := json.Marshal(details)
		if err != nil {
			utils.Logger.Error("parse es-rollover details got error", zap.Error(err))
			ctx.WriteString("parse es-rollover details got error")
			return
		}
		ctx.Write(jb)
	})
}
