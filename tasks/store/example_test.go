package store_test

import (
	"fmt"
	"time"

	"github.com/Laisky/go-ramjet/tasks/store"
)

func bindTask() {
	fmt.Println("bind task")
	go store.Ticker(1*time.Second, taskRunner)
}

func taskRunner() {
	fmt.Println("running task")
}

func Example() {
	// bind task binder
	store.Store("demo", bindTask)

	// start task binder
	store.Start()

	// run task
	store.Run()
}
