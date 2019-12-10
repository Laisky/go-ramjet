// Package store store all tasks
package store

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
)

const (
	defaultRetryWaitSec = 30
	defaultRunChanSize  = 100
	defaultEvtChanSize  = 100
)

var (
	// TaskStore global tasks store
	TaskStore = &taskStoreType{
		bindFuncs:          []*task{},
		evtListeners:       &sync.Map{},
		tobeUnregisterTask: &sync.Map{},
		runChan:            make(chan func(), defaultRunChanSize),
		evtChan:            make(chan *Event, defaultEvtChanSize),
	}
	once = sync.Once{}
)

// Event can trigger registered handler
type Event struct {
	Ts         time.Time
	Name, Type string
	Err        error
	Meta       map[string]interface{}
	Result     interface{}
}

type taskStoreType struct {
	sync.Mutex
	bindFuncs []*task
	runChan   chan func()

	// events
	evtChan            chan *Event
	evtListeners       *sync.Map // {evt_type: {func_name: EventListener }}
	tobeUnregisterTask *sync.Map // {evt_type: {func_name: struct{}{} }}
}

// EventListener is the func to handle Event
type EventListener func(*Event)

type task struct {
	f    func()
	name string
}

/*Store store task func into taskStoreType

stored funcs may not always run, it also depends settings `--task, --exclude`
*/
func (s *taskStoreType) Store(name string, f func()) {
	s.Lock()
	defer s.Unlock()
	utils.Logger.Info("store task", zap.String("name", name))
	s.bindFuncs = append(s.bindFuncs, &task{
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
		}
		tasks = strings.Split(tse, ",")
		utils.Logger.Debug("get tasks list from env", zap.Strings("tasks", tasks))
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
func (s *taskStoreType) Start(ctx context.Context) {
	once.Do(func() {
		for _, t := range s.bindFuncs {
			if t == nil || !isTaskEnabled(t.name) {
				utils.Logger.Info("ignore task", zap.String("task", t.name))
				continue
			}

			utils.Logger.Info("enable task", zap.String("name", t.name))
			t.f()
		}

		go s.runTrigger(ctx)
		go s.runEvtListener(ctx)
	})
}

// runTrigger run all tasks forever
func (s *taskStoreType) runTrigger(ctx context.Context) {
	defer utils.Logger.Info("runTrigger exit")
	var runner = func(f func()) {
		defer func() {
			if err := recover(); err != nil {
				utils.Logger.Error("running task error", zap.String("func", utils.GetFuncName(f)), zap.Error(err.(error)))
				go time.AfterFunc(defaultRetryWaitSec*time.Second, func() {
					s.runChan <- f
				})
			}
		}()
		f()
	}

	// forever loop to run each task func
	var task func()
	for {
		select {
		case <-ctx.Done():
			return
		case task = <-s.runChan:
			if utils.Settings.GetBool("debug") {
				go task()
			} else {
				go runner(task)
			}
		}
	}
}

// Trigger trigger custom event
func (s *taskStoreType) Trigger(evtName string, meta map[string]interface{}, ret interface{}, err error) {
	utils.Logger.Debug("trigger event", zap.String("name", evtName))
	s.evtChan <- &Event{
		Ts:     utils.Clock.GetUTCNow(),
		Name:   evtName,
		Meta:   meta,
		Result: ret,
		Err:    err,
	}
}

func (s *taskStoreType) runEvtListener(ctx context.Context) {
	defer utils.Logger.Info("runEvtListener exit")
	var (
		fmi,
		tobeUnregisterTMi interface{} // *sync.Map
		ok  bool
		evt *Event
	)
	for {
		select {
		case <-ctx.Done():
			return
		case evt = <-s.evtChan:
		}

		tobeUnregisterTMi, _ = s.tobeUnregisterTask.Load(evt.Name)
		if fmi, ok = s.evtListeners.Load(evt.Name); ok {
			fmi.(*sync.Map).Range(func(funcName, fi interface{}) bool {
				if tobeUnregisterTMi != nil {
					if _, ok = tobeUnregisterTMi.(*sync.Map).Load(funcName); ok { // need delete listener
						utils.Logger.Info("remove listener",
							zap.String("func", funcName.(string)),
							zap.String("evt", evt.Name))
						fmi.(*sync.Map).Delete(funcName)
						tobeUnregisterTMi.(*sync.Map).Delete(funcName)
					}
				}

				utils.Logger.Debug("trigger evt listener",
					zap.String("func", funcName.(string)),
					zap.String("evt", evt.Name))
				fi.(EventListener)(evt)
				return true
			})
		}
	}
}

// PutFunc2RunChan put task func into channel
func (s *taskStoreType) PutFunc2RunChan(f func()) {
	s.runChan <- f
}

// Ticker put task into run queue
func (s *taskStoreType) Ticker(interval time.Duration, f func()) {
	utils.Logger.Info("Ticker", zap.Duration("interval", interval))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		s.PutFunc2RunChan(f)
	}
}

// TickerAfterRun run task before start ticker
func (s *taskStoreType) TickerAfterRun(interval time.Duration, f func()) {
	utils.Logger.Info("TickerAfterRun", zap.Duration("interval", interval))
	s.PutFunc2RunChan(f)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		s.PutFunc2RunChan(f)
	}
}

// RegisterListener register new evt handler to specific event
func (s *taskStoreType) RegisterListener(evtType, funcName string, f EventListener) {
	utils.Logger.Info("register new listener", zap.String("event", evtType), zap.String("listener", funcName))
	newMap := &sync.Map{}
	newMap.Store(funcName, f)
	if sm, ok := s.evtListeners.LoadOrStore(evtType, newMap); ok {
		sm.(*sync.Map).LoadOrStore(funcName, f)
	}
}

// UnregisterListener mark func not listen to specific event
func (s *taskStoreType) UnregisterListener(evtType, funcName string) {
	utils.Logger.Info("unregister new listener", zap.String("event", evtType), zap.String("listener", funcName))
	newMap := &sync.Map{}
	newMap.Store(funcName, struct{}{})
	if sm, ok := s.tobeUnregisterTask.LoadOrStore(evtType, newMap); ok {
		sm.(*sync.Map).LoadOrStore(funcName, struct{}{})
	}
}
