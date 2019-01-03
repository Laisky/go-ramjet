package fluentd

import (
	"fmt"
	"time"

	"github.com/Laisky/go-ramjet"
	"github.com/Laisky/go-utils"
	"go.uber.org/zap"
)

func checkForAlert(m *fluentdMonitorMetric) (err error) {
	if !(m.IsSITAlive &&
		m.IsUATAlive &&
		m.IsPERFAlive &&
		m.IsPROD1Alive &&
		m.IsPROD2Alive) {
		msg := fmt.Sprintf(`
[%v]some fluentd server got error:

testd from: %v

sit: %v
uat: %v
perf: %v
prod-1: %v
prod-2: %v`,
			time.Now().Format(time.RFC3339),
			utils.Settings.GetString("host"),
			m.IsSITAlive, m.IsUATAlive, m.IsPERFAlive, m.IsPROD1Alive, m.IsPROD2Alive)

		if utils.Settings.GetBool("dry") {
			utils.Logger.Info("send fluentd alert email", zap.String("msg", msg))
			return nil
		}

		err = ramjet.Email.Send(
			"ppcelery@gmail.com",
			"Laisky Cai",
			"[pateo]fluentd got problem",
			msg,
		)
	}

	return err
}
