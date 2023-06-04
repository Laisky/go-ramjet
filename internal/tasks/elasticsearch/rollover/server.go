// Package rollover implements elasticsearch rollover tasks.
package rollover

import (
	"fmt"
	"net/http"

	gconfig "github.com/Laisky/go-config/v2"
	utils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/go-utils/v4/json"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/log"
	web "github.com/Laisky/go-ramjet/library/web"
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
	for _, sts := range gconfig.Shared.Get("tasks.elasticsearch-v2.configs").([]interface{}) {
		stI = sts.(map[interface{}]interface{})
		if stI["action"].(string) != "rollover" {
			continue
		}

		details = append(details, &idxDetail{
			Name:    stI["index-alias"].(string),
			Expires: fmt.Sprintf("%vhrs", stI["expires"].(int)/3600),
		})
	}

	log.Logger.Info("bind HTTP GET `/es/rollover`")
	web.Server.GET("/es/rollover", func(ctx *gin.Context) {
		jb, err := json.Marshal(details)
		if err != nil {
			log.Logger.Error("parse es-rollover details got error", zap.Error(err))
			ctx.String(http.StatusOK, "parse es-rollover details got error")
			return
		}
		ctx.Data(http.StatusOK, utils.HTTPHeaderContentTypeValJSON, jb)
	})
}
