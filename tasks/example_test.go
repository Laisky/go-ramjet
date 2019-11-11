package tasks_test

import (
	"time"

	"github.com/Laisky/go-ramjet/tasks/store"
)

func runTask() {
	// do some heavy works here
	// ...
}

// bindTask setup tasks
func bindTask() {
	go store.TaskStore.Ticker(10*time.Second, runTask)
}

func Example() {
	store.TaskStore.Store("es", bindTask)
}
