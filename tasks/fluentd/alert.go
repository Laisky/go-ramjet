package fluentd

import (
	"fmt"

	"github.com/Laisky/go-ramjet"
)

func checkForAlert(m *fluentdMonitorMetric) (err error) {
	if !(m.IsSITAlive &&
		m.IsUATAlive &&
		m.IsPERFAlive &&
		m.IsPROD1Alive &&
		m.IsPROD2Alive) {
		msg := fmt.Sprintf(`
some fluentd server got error:

sit: %v
uat: %v
perf: %v
prod-1: %v
prod-2: %v`,
			m.IsSITAlive, m.IsUATAlive, m.IsPERFAlive, m.IsPROD1Alive, m.IsPROD2Alive)

		err = ramjet.Email.Send(
			"ppcelery@gmail.com",
			"Laisky Cai",
			"[pateo]fluentd got problem",
			msg,
		)
	}

	return err
}
