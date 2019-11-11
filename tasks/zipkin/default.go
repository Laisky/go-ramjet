package zipkin

import (
	"github.com/Laisky/go-ramjet/tasks/store"
	"github.com/Laisky/go-ramjet/tasks/zipkin/dependencies"
)

func init() {
	store.TaskStore.Store("zipkin-dep", dependencies.BindTask)
}
