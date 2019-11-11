package fluentd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/pkg/errors"
)

var (
	httpClient = &http.Client{
		Timeout: 5 * time.Second,
	}
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

type FluentdMonitorCfg struct {
	Name, IP, HealthCheckURL string
}

func loadFluentdSettings() []*FluentdMonitorCfg {
	settings := []*FluentdMonitorCfg{}
	if utils.Settings.GetBool("debug") {
		utils.Settings.Set("tasks.fluentd.interval", 3)
	}

	var configM map[string]interface{}
	for name, configI := range utils.Settings.Get("tasks.fluentd.configs").(map[string]interface{}) {
		configM = configI.(map[string]interface{})
		settings = append(settings, &FluentdMonitorCfg{
			Name:           name,
			IP:             configM["ip"].(string),
			HealthCheckURL: configM["health-check"].(string),
		})
	}

	return settings
}

func checkFluentdHealth(wg *sync.WaitGroup, cfg *FluentdMonitorCfg, metric *sync.Map) {
	utils.Logger.Debug("checkFluentdHealth", zap.String("name", cfg.Name))
	defer wg.Done()
	var (
		resp    *http.Response
		err     error
		isAlive = false
	)
	resp, err = httpClient.Get(cfg.HealthCheckURL)
	if err != nil {
		utils.Logger.Info("http get fluentd status error", zap.Error(err))
	} else if resp.StatusCode == 200 {
		isAlive = true
	}

	metric.Store(cfg, isAlive)
}

func pushResultToES(metric *fluentdMonitorMetric) (err error) {
	url := utils.Settings.GetString("tasks.fluentd.push")
	jsonBytes, err := json.Marshal(metric)
	if err != nil {
		return errors.Wrap(err, "parse json got error")
	}

	utils.Logger.Debug("push fluentd metric", zap.ByteString("metric", jsonBytes[:]))
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

	utils.Logger.Info("success to push fluentd metric to elasticsearch",
		zap.String("type", metric.MonitorType),
		zap.String("ts", metric.Timestamp))
	return nil
}
