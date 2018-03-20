package store_test

import (
	"fmt"
	"time"

	"github.com/go-ramjet/tasks/store"
	"github.com/go-ramjet/utils"
)

func bindTask() {
	fmt.Println("bind task")
}

func setNext(f func()) {
	utils.LoadSettings()
	time.AfterFunc(1*time.Second, func() {
		store.PutReadyTask(f)
	})
}

func taskRunner() {
	fmt.Println("running task")
	setNext(taskRunner)
}

func Example() {
	// bind task binder
	store.Store(bindTask)

	// start task binder
	store.Start()

	// run task
	store.Run()
}
