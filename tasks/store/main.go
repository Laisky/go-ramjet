// Package store store all tasks
package store

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
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
	utils.Logger.Info("store task", zap.String("name", name))
	store.bindFuncs = append(store.bindFuncs, &task{
		f:    f,
		name: name,
	})
}

func isTaskEnabled(task string) bool {
	utils.Logger.Debug("isTaskEnabled", zap.String("task", task))
	tasks := utils.Settings.GetStringSlice("task")
	extasks := strings.Split(utils.Settings.GetString("exclude"), ",")

	if len(tasks) == 0 { // not set -t
		tse := os.Getenv("TASKS")
		if len(tse) == 0 { // not set env `TASKS`
			utils.Logger.Info("start to run all tasks...")
			return true
		} else {
			tasks = strings.Split(tse, ",")
			utils.Logger.Debug("get tasks list from env", zap.Strings("tasks", tasks))
		}
	}

	for _, k := range extasks {
		if k == task {
			utils.Logger.Debug("ignored by `exclude`")
			return false
		}
	}

	for _, k := range tasks {
		if k == task {
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
			if t == nil || !isTaskEnabled(t.name) {
				utils.Logger.Info("ignore task", zap.String("task", t.name))
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
	utils.Logger.Info("Ticker", zap.Duration("interval", interval))
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
