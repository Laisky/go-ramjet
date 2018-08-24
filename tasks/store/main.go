// Package store store all tasks
package store

import (
	"os"
	"sync"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"

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

func isContians(tasks map[string]interface{}, n string) bool {
	if len(tasks) == 0 { // not set -t
		tse := os.Getenv("TASKS")
		if len(tse) == 0 { // not set env `TASKS`
			utils.Logger.Debug("start to run all tasks...")
			return true
		}
	}

	for k := range tasks {
		if k == n {
			return true
		}
	}
	return false
}

// Start start to run task binding
// only run once
func Start() {
	once.Do(func() {
		tasks := viper.Get("tasks").(map[string]interface{})
		for _, t := range store.bindFuncs {
			if t == nil || !isContians(tasks, t.name) {
				continue
			}

			utils.Logger.Info("start to running...", zap.String("name", t.name))
			t.f()
		}
	})
}

var runner = func(f func()) {
	defer func() {
		if err := recover(); err != nil {
			utils.Logger.Error("running task error", zap.String("func", utils.GetFuncName(f)), zap.Error(err.(error)))
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
	utils.Logger.Info("TickerAfterRun", zap.Duration("interval", interval))
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
