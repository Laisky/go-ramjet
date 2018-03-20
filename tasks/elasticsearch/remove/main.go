// Package remove Some tasks to operate ES
package remove

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-ramjet/tasks/store"

	log "github.com/cihub/seelog"
	"github.com/go-ramjet/utils"
	"github.com/spf13/viper"
)

// Query json query to request elasticsearch
type Query struct {
	Range *Range                  `json:"query"`
	Size  int                     `json:"size"`
	Sort  []map[string]string     `json:"sort"`
	Term  *map[string]interface{} `json:"term,omitempty"`
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
	sync.Mutex
	Index  string
	Expire int
	Term   *map[string]interface{}
}

func getDateStringSecondsAgo(seconds int) (dateString string) {
	dateString = time.Now().Add(time.Second * time.Duration(-seconds)).Format(time.RFC3339)
	return
}

func getURLByIndexName(index string) (url string) {
	var baseURL bytes.Buffer
	baseURL.WriteString(viper.GetString("tasks.elasticsearch.url"))
	baseURL.WriteString(index)
	baseURL.WriteString("/_delete_by_query?conflicts=proceed")
	url = baseURL.String()
	return
}

// isRespInTrouble check whether response is really in trouble when status!=200
func isRespInTrouble(errMsg string) (isErr bool) {
	defer log.Flush()
	log.Debugf("isRespInTrouble for errMsg %v", errMsg)
	isErr = true
	if strings.Contains(errMsg, "No mapping found for [@timestamp]") {
		isErr = false
		return
	}
	return
}

func removeDocumentsByTaskSetting(task *MonitorTaskConfig) {
	defer log.Flush()
	task.Lock()
	defer task.Unlock()
	dateBefore := getDateStringSecondsAgo(task.Expire)
	log.Infof("removeDocumentsByTaskSetting for task %v, before %v", task.Index, dateBefore)
	requestBody := Query{
		Range: &Range{
			Range: map[string]interface{}{"@timestamp": map[string]string{
				"lte": dateBefore,
			}},
		},
		Size: viper.GetInt("tasks.elasticsearch.batch"),
		Sort: []map[string]string{
			map[string]string{"@timestamp": "asc"},
		},
		Term: task.Term,
	}

	var resp Resp
	url := getURLByIndexName(task.Index)
	requestData := utils.RequestData{
		Data: &requestBody,
	}

	// debug
	if viper.GetBool("debug") {
		b, _ := json.Marshal(requestData)
		log.Debugf("request %v", string(b[:]))
		return
	}

	if err := utils.RequestJSON("post", url, &requestData, &resp); err != nil {
		errMsg := err.Error()
		if isRespInTrouble(errMsg) {
			log.Errorf("delete documents error for task %v, url %v: %v", task.Index, url, errMsg)
			return
		}

		log.Debugf("http.RequestJSON got some innocent error: %v", errMsg)
		resp = Resp{
			Deleted: 0,
			Total:   0,
		}
	}

	log.Infof("deleted documents for %v: %v/%v", task.Index, resp.Deleted, resp.Total)
	if resp.Total >= viper.GetInt("tasks.elasticsearch.batch") { // continue to delete documents
		go removeDocumentsByTaskSetting(task)
	}
}

// BindRemoveCPLogs Tasks to remove documents in ES
func BindRemoveCPLogs() {
	defer log.Flush()
	log.Info("bind remove CP Logs...")
	go setNext(runTask)
}

func runTask() {
	defer log.Flush()
	// TOOD: reload settings before each loop
	taskSettings := loadDeleteTaskSettings()
	go setNext(runTask)
	for _, taskConfig := range taskSettings {
		go removeDocumentsByTaskSetting(taskConfig)
	}
}

func setNext(f func()) {
	time.AfterFunc(viper.GetDuration("tasks.elasticsearch.interval")*time.Second, func() {
		store.PutReadyTask(f)
	})
}

// loadDeleteTaskSettings load config for each subtask
func loadDeleteTaskSettings() (taskSettings []*MonitorTaskConfig) {
	var (
		config      map[interface{}]interface{}
		indexConfig *MonitorTaskConfig
		term        = new(map[string]interface{})
	)
	for _, configI := range viper.Get("tasks.elasticsearch.configs").([]interface{}) {
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
