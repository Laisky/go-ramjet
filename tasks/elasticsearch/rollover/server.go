package rollover

import (
	"encoding/json"
	"fmt"

	"github.com/Laisky/go-ramjet"
	"github.com/kataras/iris"

	utils "github.com/Laisky/go-utils"
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

	ramjet.Server.Get("/es/rollover", func(ctx iris.Context) {
		jb, err := json.Marshal(details)
		if err != nil {
			utils.Logger.Errorf("parse es-rollover details got error: %+v", err)
			ctx.WriteString("parse es-rollover details got error")
			return
		}
		ctx.Write(jb)
	})
}
