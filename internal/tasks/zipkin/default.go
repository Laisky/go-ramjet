// Package zipkin implements zipkin tasks.
package zipkin

import (
	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/internal/tasks/zipkin/dependencies"
)

func init() {
	store.TaskStore.Store("zipkin-dep", dependencies.BindTask)
}
