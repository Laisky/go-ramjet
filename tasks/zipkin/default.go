package zipkin

import (
	"github.com/Laisky/go-ramjet/tasks/store"
	"github.com/Laisky/go-ramjet/tasks/zipkin/dependencies"
)

func init() {
	store.Store("zipkin-dep", dependencies.BindTask)
}
