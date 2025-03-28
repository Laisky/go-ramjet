// Package remove Some tasks to operate ES
package remove

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/go-utils/v5"
	"github.com/Laisky/go-utils/v5/json"
	"github.com/Laisky/zap"
	"golang.org/x/sync/semaphore"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

var (
	sem       *semaphore.Weighted // concurrent to delete documents
	ctx       = context.Background()
	indexLock = map[string]*sync.Mutex{}
)

// Query json query to request elasticsearch
type Query struct {
	Range *Range `json:"query"`
	Size  int    `json:"size"`
	// Sort  []map[string]string     `json:"sort"`
	Term *map[string]interface{} `json:"term,omitempty"`
}

// Range range query in Query
type Range struct {
	Range map[string]interface{} `json:"range"`
}

// Resp json response from elasticsearch
type Resp struct {
	Deleted int `json:"deleted"`
	Total   int `json:"total"`
}

// MonitorTaskConfig config for each ES index
type MonitorTaskConfig struct {
	l      *sync.Mutex
	Index  string
	Expire int
	Term   *map[string]interface{}
}

func (c *MonitorTaskConfig) Lock() {
	c.l.Lock()
}

func (c *MonitorTaskConfig) Unlock() {
	c.l.Unlock()
}

func (c *MonitorTaskConfig) SetLock(l *sync.Mutex) {
	c.l = l
}

func getDateStringSecondsAgo(seconds int) (dateString string) {
	dateString = time.Now().Add(time.Second * time.Duration(-seconds)).Format(time.RFC3339)
	return
}

func getURLByIndexName(index string) (url string) {
	var baseURL bytes.Buffer
	baseURL.WriteString(gconfig.Shared.GetString("tasks.elasticsearch.url"))
	baseURL.WriteString(index)
	baseURL.WriteString("/_delete_by_query?conflicts=proceed")
	url = baseURL.String()
	return
}

// isRespInTrouble check whether response is really in trouble when status!=200
func isRespInTrouble(errMsg string) (isErr bool) {
	log.Logger.Debug("isRespInTrouble", zap.String("err", errMsg))
	isErr = true
	if strings.Contains(errMsg, "No mapping found for [@timestamp]") {
		isErr = false
		return
	}
	return
}

func removeDocumentsByTaskSetting(task *MonitorTaskConfig) {
	task.Lock() // do not parallel to remove same index
	defer task.Unlock()

	if err := sem.Acquire(ctx, 1); err != nil {
		log.Logger.Error("Failed to acquire semaphore", zap.Error(err))
		return
	}
	defer sem.Release(1)

	dateBefore := getDateStringSecondsAgo(task.Expire)
	log.Logger.Info("removeDocumentsByTaskSetting",
		zap.String("task", task.Index),
		zap.String("dateBefore", dateBefore))
	requestBody := Query{
		Range: &Range{
			Range: map[string]interface{}{"@timestamp": map[string]string{
				"lte": dateBefore,
			}},
		},
		Size: gconfig.Shared.GetInt("tasks.elasticsearch.batch"),
		// Sort: []map[string]string{
		// 	map[string]string{"@timestamp": "asc"},
		// },
		Term: task.Term,
	}

	var resp Resp
	url := getURLByIndexName(task.Index)
	requestData := utils.RequestData{
		Data: &requestBody,
	}

	// dry
	if gconfig.Shared.GetBool("dry") {
		b, _ := json.Marshal(requestData)
		log.Logger.Info("request ", zap.ByteString("data", b))
		return
	}

	if err := utils.RequestJSON("post", url, &requestData, &resp); err != nil {
		errMsg := err.Error()
		if isRespInTrouble(errMsg) {
			log.Logger.Error("delete documents error",
				zap.String("index", task.Index),
				zap.String("url", url),
				zap.Error(err))
			resp = Resp{
				Deleted: 0,
				Total:   gconfig.Shared.GetInt("tasks.elasticsearch.batch"),
			}
		} else {
			log.Logger.Debug("http.RequestJSON got some innocent error",
				zap.Error(err))
			resp = Resp{
				Deleted: 0,
				Total:   0,
			}
		}
	}

	log.Logger.Info("deleted documents",
		zap.String("index", task.Index),
		zap.Int("deleted", resp.Deleted),
		zap.Int("total", resp.Total))
	if resp.Total >= gconfig.Shared.GetInt("tasks.elasticsearch.batch") { // continue to delete documents
		go removeDocumentsByTaskSetting(task)
	}
}

// BindRemoveCPLogs Tasks to remove documents in ES
func BindRemoveCPLogs() {
	log.Logger.Info("bind remove ES Logs...")

	if gconfig.Shared.GetBool("debug") { // set for debug
		gconfig.Shared.Set("tasks.elasticsearch.interval", 1)
		gconfig.Shared.Set("tasks.elasticsearch.batch", 1)
	}

	sem = semaphore.NewWeighted(gconfig.Shared.GetInt64("tasks.elasticsearch.concurrent"))
	go store.TaskStore.Ticker(gconfig.Shared.GetDuration("tasks.elasticsearch.interval")*time.Second, runTask)
}

func runTask() {
	taskSettings := loadDeleteTaskSettings()
	for _, taskConfig := range taskSettings {
		if _, ok := indexLock[taskConfig.Index]; !ok {
			indexLock[taskConfig.Index] = &sync.Mutex{}
		}
		taskConfig.SetLock(indexLock[taskConfig.Index])
		go removeDocumentsByTaskSetting(taskConfig)
	}
}

// loadDeleteTaskSettings load config for each subtask
func loadDeleteTaskSettings() (taskSettings []*MonitorTaskConfig) {
	log.Logger.Debug("loadDeleteTaskSettings...")

	var (
		config      map[interface{}]interface{}
		indexConfig *MonitorTaskConfig
		term        = new(map[string]interface{})
	)
	for _, configI := range gconfig.Shared.Get("tasks.elasticsearch.configs").([]interface{}) {
		log.Logger.Debug("load delete tasks settings")
		config = configI.(map[interface{}]interface{})
		indexConfig = &MonitorTaskConfig{
			Index: config["index"].(string),
		}
		if val, ok := config["expire"]; ok {
			indexConfig.Expire = val.(int)
		}
		if val, ok := config["term"]; ok {
			if err := json.Unmarshal([]byte(val.(string)), term); err != nil {
				panic(fmt.Sprintf("load delete settings error: %+v", err))
			}
			indexConfig.Term = term
		}

		taskSettings = append(taskSettings, indexConfig)
	}
	return
}
