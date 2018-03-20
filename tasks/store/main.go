// Package store store all tasks
package store

import (
	"sync"
	"time"

	log "github.com/cihub/seelog"
	"github.com/spf13/viper"

	"github.com/go-ramjet/utils"
)

type tasksStore struct {
	bindFuncs []func()
	runChan   chan func()
}

var (
	store = &tasksStore{
		[]func(){},
		make(chan func(), 20),
	}
	once = sync.Once{}
)

// Store store binding func into tasksStore
func Store(f func()) {
	store.bindFuncs = append(store.bindFuncs, f)
}

// Start start to run task binding
// only run once
func Start() {
	once.Do(func() {
		for _, f := range store.bindFuncs {
			if f == nil {
				continue
			}
			f()
		}
	})
}

var runner = func(f func()) {
	defer func() {
		defer log.Flush()
		if err := recover(); err != nil {
			log.Errorf("running task error for %v: %+v", utils.GetFunctionName(f), err)
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
		if viper.GetBool("debug") {
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
