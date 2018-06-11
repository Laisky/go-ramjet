// Package store store all tasks
package store

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"

	"github.com/Laisky/go-utils"
)

type tasksStore struct {
	bindFuncs []*task
	runChan   chan func()
}

type task struct {
	f    func()
	name string
}

var (
	store = &tasksStore{
		[]*task{},
		make(chan func(), 20),
	}
	once = sync.Once{}
)

// Store store binding func into tasksStore
func Store(name string, f func()) {
	store.bindFuncs = append(store.bindFuncs, &task{
		f:    f,
		name: name,
	})
}

func isContians(s []string, n string) bool {
	if len(s) == 0 { // not set -t
		tse := os.Getenv("TASKS")
		if len(tse) == 0 { // not set env `TASKS`
			utils.Logger.Debug("start to run all tasks...")
			return true
		}

		s = strings.Split(tse, ",")
	}

	for _, v := range s {
		if v == n {
			return true
		}
	}
	return false
}

// Start start to run task binding
// only run once
func Start() {
	once.Do(func() {
		for _, t := range store.bindFuncs {
			if t == nil || !isContians(viper.GetStringSlice("task"), t.name) {
				continue
			}

			utils.Logger.Infof("start to running %v...", t.name)
			t.f()
		}
	})
}

var runner = func(f func()) {
	defer func() {
		if err := recover(); err != nil {
			utils.Logger.Errorf("running task error for %v: %+v", utils.GetFuncName(f), err)
			go time.AfterFunc(30*time.Second, func() {
				store.runChan <- f
			})
		}
	}()
	f()
}

// Run run all tasks forever
func Run() {
	// forever loop to run each task func
	for task := range store.runChan {
		if utils.Settings.GetBool("debug") {
			go task()
		} else {
			go runner(task)
		}
	}
}

// PutReadyTask put task func into channel
func PutReadyTask(f func()) {
	store.runChan <- f
}

// Ticker put task into run queue
func Ticker(interval time.Duration, f func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			PutReadyTask(f)
		}
	}
}

// TickerAfterRun run task before start ticker
func TickerAfterRun(interval time.Duration, f func()) {
	PutReadyTask(f)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			PutReadyTask(f)
		}
	}
}
