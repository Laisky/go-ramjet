package fluentd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/Laisky/go-utils"
	"github.com/pkg/errors"
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
	if utils.Settings.GetBool("debug") {
		utils.Settings.Set("tasks.fluentd.interval", 1)
	}
	for name, configI := range utils.Settings.Get("tasks.fluentd.configs").(map[string]interface{}) {
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
		utils.Logger.Errorf("http get fluentd status error: %v", err)
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

func pushResultToES(metric *fluentdMonitorMetric) (err error) {
	url := utils.Settings.GetString("tasks.fluentd.push")
	jsonBytes, err := json.Marshal(metric)
	if err != nil {
		return errors.Wrap(err, "parse json got error")
	}

	utils.Logger.Debugf("push fluentd metric %+v", string(jsonBytes[:]))
	if utils.Settings.GetBool("dry") {
		return nil
	}

	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return errors.Wrap(err, "http post got error")
	}
	err = utils.CheckResp(resp)
	if err != nil {
		return err
	}

	utils.Logger.Infof("success to push fluentd metric to elasticsearch for node %v", metric)
	return nil
}
