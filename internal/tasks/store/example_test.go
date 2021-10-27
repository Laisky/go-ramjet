package store_test

import (
	"context"
	"fmt"
	"time"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
)

func bindTask() {
	fmt.Println("bind task")
	go store.TaskStore.Ticker(1*time.Second, taskRunner)
}

func taskRunner() {
	fmt.Println("running task")
}

func Example() {
	// bind task binder
	store.TaskStore.Store("demo", bindTask)

	// start task binder
	go store.TaskStore.Start(context.Background())
}
