package tasks_test

import (
	"time"

	"github.com/go-ramjet/tasks/store"
	"github.com/go-ramjet/utils"
)

func setNext(f func()) {
	utils.LoadSettings()
	time.AfterFunc(10*time.Second, func() {
		store.PutReadyTask(f)
	})
}

func runTask() {
	// set next task
	go setNext(runTask)

	// do some heavy works here
	// ...
}

// bindTask setup tasks
func bindTask() {
	go setNext(runTask)
}

func Example() {
	store.Store(bindTask)
}
