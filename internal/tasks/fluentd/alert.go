package fluentd

import (
	"fmt"
	"sync"
	"time"

	"github.com/Laisky/go-ramjet/library/log"

	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/library/alert"
)

func checkForAlert(m *sync.Map) (err error) {
	cnt := ""
	m.Range(func(ki, vi interface{}) bool {
		if vi.(bool) {
			return true
		}

		if cnt == "" {
			cnt = fmt.Sprintf("[%v]some fluentd server got error:\ntestd from: %v\n",
				time.Now().Format(time.RFC3339),
				utils.Settings.GetString("host"))
		}

		k := ki.(*MonitorCfg)
		cnt += fmt.Sprintf("%v(%v) got error\n", k.Name, k.IP)
		return true
	})

	if utils.Settings.GetBool("dry") {
		log.Logger.Info("send fluentd alert email", zap.String("msg", cnt))
		return nil
	}

	if cnt != "" {
		err = alert.Manager.Send(
			"ppcelery@gmail.com",
			"Laisky Cai",
			"[google]fluentd got problem",
			cnt,
		)
	}

	return err
}
