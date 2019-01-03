package zipkin

import (
	"github.com/Laisky/go-ramjet/tasks/store"
	"github.com/Laisky/go-ramjet/tasks/zipkin/dependencies"
	"github.com/Laisky/go-ramjet/tasks/zipkin/monitor"
)

func init() {
	store.Store("zipkin-dep", dependencies.BindTask)
	store.Store("zipkin-monitor", monitor.BindTask)
}
