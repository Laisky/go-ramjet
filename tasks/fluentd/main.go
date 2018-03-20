package fluentd

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	log "github.com/cihub/seelog"
	"pateo.com/go-ramjet/tasks/store"
	"pateo.com/go-ramjet/utils"

	"github.com/spf13/viper"
)

var (
	httpClient = &http.Client{
		Timeout: 5 * time.Second,
	}
	settings map[string]*config
)

type fluentdMonitorMetric struct {
	MonitorType  string `json:"monitor_type"`
	Timestamp    string `json:"@timestamp"`
	IsSITAlive   bool   `json:"fluentd.aggregator.health.sit"`
	IsUATAlive   bool   `json:"fluentd.aggregator.health.uat"`
	IsPERFAlive  bool   `json:"fluentd.aggregator.health.perf"`
	IsPROD1Alive bool   `json:"fluentd.aggregator.health.prod-1"`
	IsPROD2Alive bool   `json:"fluentd.aggregator.health.prod-2"`
}

type config struct {
	IP             string
	HealthCheckURL string
}

func loadFluentdSettings() (settings map[string]*config) {
	var (
		configM map[string]interface{}
	)
	settings = map[string]*config{}
	if viper.GetBool("debug") {
		viper.Set("tasks.fluentd.interval", 1)
	}
	for name, configI := range viper.Get("tasks.fluentd.configs").(map[string]interface{}) {
		configM = configI.(map[string]interface{})
		settings[name] = &config{
			IP:             configM["ip"].(string),
			HealthCheckURL: configM["health-check"].(string),
		}
	}

	return
}

func checkFluentdHealth(wg *sync.WaitGroup, name, url string, metric *fluentdMonitorMetric) {
	defer wg.Done()
	var (
		resp    *http.Response
		err     error
		isAlive = false
	)
	resp, err = httpClient.Head(url)
	if err != nil {
		log.Errorf("http get fluentd status error: %v", err)
		return
	}
	if resp.StatusCode == 200 {
		isAlive = true
	}

	switch name {
	case "sit":
		metric.IsSITAlive = isAlive
	case "uat":
		metric.IsUATAlive = isAlive
	case "perf":
		metric.IsPERFAlive = isAlive
	case "prod-1":
		metric.IsPROD1Alive = isAlive
	case "prod-2":
		metric.IsPROD2Alive = isAlive
	}
}

func pushResultToES(metric *fluentdMonitorMetric) {
	defer log.Flush()
	url := viper.GetString("tasks.elasticsearch.url") + "monitor-stats/stats/"
	jsonBytes, err := json.Marshal(metric)
	if err != nil {
		log.Error(err.Error())
		return
	}

	if viper.GetBool("debug") {
		log.Debugf("push fluentd metric %+v", string(jsonBytes[:]))
	} else {
		resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(jsonBytes))
		if err != nil {
			log.Error(err.Error())
			return
		}
		defer resp.Body.Close()
		if utils.FloorDivision(resp.StatusCode, 100) != 2 {
			respBytes, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Error(err.Error())
				return
			}
			log.Error(string(respBytes[:]))
			return
		}
		if err != nil {
			log.Error(err.Error())
			return
		}
	}
	log.Infof("success to push fluentd metric to elasticsearch for node %v", metric)
}

func setNext(f func()) {
	time.AfterFunc(viper.GetDuration("tasks.fluentd.interval")*time.Second, func() {
		store.PutReadyTask(f)
	})
}

func runTask() {
	go setNext(runTask)
	var (
		wg     = &sync.WaitGroup{}
		metric = &fluentdMonitorMetric{
			MonitorType: "fluentd",
			Timestamp:   utils.UTCNow().Format(time.RFC3339),
		}
	)
	for name, config := range settings {
		wg.Add(1)
		go checkFluentdHealth(wg, name, config.HealthCheckURL, metric)
	}
	wg.Wait()
	pushResultToES(metric)
}

func bindTask() {
	defer log.Flush()
	log.Info("bind fluentd monitor...")
	settings = loadFluentdSettings()
	go setNext(runTask)
}

func init() {
	store.Store(bindTask)
}
