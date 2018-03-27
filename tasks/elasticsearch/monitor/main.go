package monitor

// Monitor ElasticSearch's metrics

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	log "github.com/cihub/seelog"
	"github.com/spf13/viper"
	"github.com/go-ramjet/tasks/store"
	"github.com/go-ramjet/utils"
)

var (
	monitorLock = &sync.Mutex{}
	httpClient  = http.Client{
		Timeout: time.Second * 10,
	}
	esNodeStatAPI  string
	esIndexStatAPI string
	isFirstRun     = true
)

func loadESStats(wg *sync.WaitGroup, url string, esStats interface{}) {
	defer wg.Done()
	resp, err := httpClient.Get(url)
	if err != nil {
		log.Errorf("try to get es stats got error for url %v: %+v", url, err)
		esStats = nil
		return
	}
	defer resp.Body.Close()
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("try to read es stat body got error for url %v: %+v", url, err)
		esStats = nil
		return
	}
	err = json.Unmarshal(respBytes, esStats)
	if err != nil {
		log.Errorf("try to parse es stat got error for url %v: %+v", url, err)
		esStats = nil
		return
	}
}

type MonitorMetric struct {
	NodeName    string `json:"node_name"`
	MonitorType string `json:"monitor_type"`
	Timestamp   string `json:"@timestamp"`
	*OSMetric
	*OperatorsMetric
	*FSMetric
	*JVMMetric
	*HTTPMetric
	*ThreadMetric
}

func extractStatsToMetricForEachNode(stats map[string]interface{}) (metrics []*MonitorMetric) {
	for _, nodeData := range stats["nodes"].(map[string]interface{}) {
		data := nodeData.(map[string]interface{})
		metrics = append(metrics, &MonitorMetric{
			NodeName:        data["name"].(string),
			MonitorType:     "node",
			Timestamp:       utils.UTCNow().Format(time.RFC3339),
			OSMetric:        getOSMetric(data),
			OperatorsMetric: getOperatorsMetric(data),
			FSMetric:        getFSMetric(data),
			JVMMetric:       getJVMMetric(data),
			HTTPMetric:      getHTTPMetric(data),
			ThreadMetric:    getThreadMetric(data),
		})
	}
	return
}

type ESEvent struct {
	Index  string         `json:"_index"`
	Type   string         `json:"_type"`
	Source *MonitorMetric `json:"_source"`
}

func pushMetricToES(metric interface{}) {
	defer log.Flush()
	url := viper.GetString("tasks.elasticsearch.url") + "monitor-stats/stats/"
	jsonBytes, err := json.Marshal(metric)
	if err != nil {
		log.Error(err.Error())
		return
	}

	if viper.GetBool("debug") {
		log.Debugf("push es metric %v", string(jsonBytes[:]))
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
	log.Infof("success to push es metric to elasticsearch for node %v", metric)
}

// BindMonitorTask start monitor tasks
func BindMonitorTask() {
	defer log.Flush()
	log.Info("bind ES monitors...")

	esNodeStatAPI = viper.GetString("tasks.elasticsearch.url") + "_nodes/stats"
	esIndexStatAPI = viper.GetString("tasks.elasticsearch.url") + "_cat/indices/?h=index,store.size&bytes=m&format=json"

	go store.RunThenTicker(viper.GetDuration("tasks.elasticsearch.interval")*time.Second, runTask)
}

func runTask() {
	monitorLock.Lock()
	defer monitorLock.Unlock()

	var (
		esStats      = make(map[string]interface{})
		esIndexStats = []map[string]string{}
		wg           = &sync.WaitGroup{}
	)
	wg.Add(2)
	go loadESStats(wg, esNodeStatAPI, &esStats)
	go loadESStats(wg, esIndexStatAPI, &esIndexStats)
	wg.Wait()

	if len(esStats) == 0 {
		return
	}

	// node metrics
	// extract metric to compare without push
	metrics := extractStatsToMetricForEachNode(esStats)
	if !isFirstRun {
		for _, metric := range metrics {
			go pushMetricToES(metric)
		}
	}

	// index metrics
	// extract metric to compare without push
	indexMetric := extractStatsToMetricForEachIndex(esIndexStats)
	if !isFirstRun {
		go pushMetricToES(indexMetric)
	}

	if isFirstRun {
		isFirstRun = false
	}
}
