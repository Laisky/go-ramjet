package fluentd

import (
	"net/http"
	"sync"
	"time"

	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v5"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/library/log"
)

var (
	httpClient = &http.Client{
		Timeout: 5 * time.Second,
	}
)

// type monitorMetric struct {
// 	MonitorType  string `json:"monitor_type"`
// 	Timestamp    string `json:"@timestamp"`
// 	IsSITAlive   bool   `json:"fluentd.aggregator.health.sit"`
// 	IsUATAlive   bool   `json:"fluentd.aggregator.health.uat"`
// 	IsPERFAlive  bool   `json:"fluentd.aggregator.health.perf"`
// 	IsPROD1Alive bool   `json:"fluentd.aggregator.health.prod-1"`
// 	IsPROD2Alive bool   `json:"fluentd.aggregator.health.prod-2"`
// }

type MonitorCfg struct {
	Name, IP, HealthCheckURL string
}

func loadFluentdSettings() []*MonitorCfg {
	settings := []*MonitorCfg{}
	if gconfig.Shared.GetBool("debug") {
		gconfig.Shared.Set("tasks.fluentd.interval", 3)
	}

	var configM map[string]interface{}
	for name, configI := range gconfig.Shared.Get("tasks.fluentd.configs").(map[string]interface{}) {
		configM = configI.(map[string]interface{})
		settings = append(settings, &MonitorCfg{
			Name:           name,
			IP:             configM["ip"].(string),
			HealthCheckURL: configM["health-check"].(string),
		})
	}

	return settings
}

func checkFluentdHealth(wg *sync.WaitGroup, cfg *MonitorCfg, metric *sync.Map) {
	log.Logger.Debug("checkFluentdHealth", zap.String("name", cfg.Name))
	defer wg.Done()
	var (
		resp    *http.Response
		err     error
		isAlive = false
	)
	resp, err = httpClient.Get(cfg.HealthCheckURL)
	if err != nil {
		log.Logger.Error("http get fluentd status error", zap.Error(err))
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger) // nolint: errcheck,gosec

	if resp.StatusCode == http.StatusOK {
		isAlive = true
	}

	metric.Store(cfg, isAlive)
}

// func pushResultToES(metric *monitorMetric) (err error) {
// 	url := gconfig.Shared.GetString("tasks.fluentd.push")
// 	jsonBytes, err := json.Marshal(metric)
// 	if err != nil {
// 		return errors.Wrap(err, "parse json got error")
// 	}

// 	log.Logger.Debug("push fluentd metric", zap.ByteString("metric", jsonBytes[:]))
// 	if gconfig.Shared.GetBool("dry") {
// 		return nil
// 	}

// 	resp, err := httpClient.Post(url, "application/json", bytes.NewBuffer(jsonBytes))
// 	if err != nil {
// 		return errors.Wrap(err, "http post got error")
// 	}
//  defer gutils.LogErr(resp.Body.Close, log.Logger)

// 	err = utils.CheckResp(resp)
// 	if err != nil {
// 		return err
// 	}

// 	log.Logger.Info("success to push fluentd metric to elasticsearch",
// 		zap.String("type", metric.MonitorType),
// 		zap.String("ts", metric.Timestamp))
// 	return nil
// }
