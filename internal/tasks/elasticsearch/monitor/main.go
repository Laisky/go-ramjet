package monitor

// Monitor ElasticSearch's metrics

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/go-utils/v6/json"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/log"
)

var (
	isIndicesFirstRun = sync.Map{}
	httpClient        = http.Client{
		Timeout: time.Second * 30,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 20,
		},
	}
)

// ClusterSt settings for cluster
type ClusterSt struct {
	URL  string
	Name string
	Push string
}

// GetNodeStatAPI return the API to fetch node stats
func (c *ClusterSt) GetNodeStatAPI() string {
	return c.URL + "_nodes/stats"
}

// GetIdxStatAPI return the API to fetch index stats
func (c *ClusterSt) GetIdxStatAPI() string {
	return c.URL + "_cat/indices/?h=index,store.size&bytes=m&format=json"
}

// GetPushMetricAPI return the API to push metric data
func (c *ClusterSt) GetPushMetricAPI() string {
	return c.Push + "monitor-stats-write/stats/"
}

type AlertSt struct {
	Receivers  map[string]string
	Conditions map[string]interface{}
}

// St is monitor task settings
type St struct {
	Sts      []*ClusterSt
	Interval int
	*AlertSt
}

func loadESStats(wg *sync.WaitGroup, url string, esStats interface{}) {
	log.Logger.Debug("load es stats for url", zap.String("url", strings.Split(url, "@")[1]))
	defer wg.Done()
	resp, err := httpClient.Get(url)
	if err != nil {
		log.Logger.Error("try to get es stats got error", zap.String("url", url), zap.Error(err))
		return
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger) // nolint: errcheck,gosec

	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Logger.Error("try to read es stat body got error", zap.String("url", url), zap.Error(err))
		return
	}
	err = json.Unmarshal(respBytes, esStats)
	if err != nil {
		log.Logger.Error("try to parse es stat got error", zap.String("url", url), zap.Error(err))
		return
	}
}

// NodeMetric is the whole metric for each node
type NodeMetric struct {
	ClusterName string `json:"cluster_name"`
	NodeName    string `json:"node_name"`
	MonitorType string `json:"monitor_type"`
	Timestamp   string `json:"@timestamp"`
	Message     string `json:"message"`
	*OSMetric
	*OperatorsMetric
	*FSMetric
	*JVMMetric
	*HTTPMetric
	*ThreadMetric
}

func extractStatsToMetricForEachNode(clusterName string, stats map[string]interface{}) (metrics []*NodeMetric) {
	metrics = []*NodeMetric{}
	sv, ok := stats["nodes"].(map[string]interface{})
	if !ok {
		return
	}

	for _, nodeData := range sv {
		data := nodeData.(map[string]interface{})
		metrics = append(metrics, &NodeMetric{
			ClusterName:     clusterName,
			NodeName:        data["name"].(string),
			MonitorType:     "node",
			Timestamp:       gutils.UTCNow().Format(time.RFC3339),
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

// ESEvent wrap the node metric to push to ES
type ESEvent struct {
	Index  string      `json:"_index"`
	Type   string      `json:"_type"`
	Source *NodeMetric `json:"_source"`
}

func pushMetricToES(c *ClusterSt, metric interface{}) {
	log.Logger.Info("push es metric to elasticsearch", zap.String("node", c.Name))
	jsonBytes, err := json.Marshal(metric)
	if err != nil {
		log.Logger.Error("try to marshal es metric got error", zap.Error(err))
		return
	}

	log.Logger.Debug("push es metric",
		zap.String("api", c.GetPushMetricAPI()),
		zap.ByteString("body", jsonBytes[:]))
	if gconfig.Shared.GetBool("dry") {
		return
	}
	resp, err := httpClient.Post(c.GetPushMetricAPI(), gutils.HTTPHeaderContentTypeValJSON, bytes.NewBuffer(jsonBytes))
	if err != nil {
		log.Logger.Error("try to push es metric got error", zap.Error(err))
		return
	}
	defer gutils.LogErr(resp.Body.Close, log.Logger) // nolint: errcheck,gosec

	err = gutils.CheckResp(resp)
	if err != nil {
		log.Logger.Error("got error after push", zap.Error(err))
		return
	}
	log.Logger.Debug("success to push es metric to elasticsearch for node")
}

// BindMonitorTask start monitor tasks
func BindMonitorTask() {
	log.Logger.Info("bind ES monitors...")

	st := LoadSettings()
	if st == nil {
		return
	}

	interval := st.Interval
	if gconfig.Shared.GetBool("debug") { // set for debug
		interval = 3
	}

	go store.TaskStore.TickerAfterRun(time.Duration(interval)*time.Second, runTask)
}

func runTask() {
	st := LoadSettings()
	for _, cst := range st.Sts {
		go RunClusterMonitorTask(cst, st.AlertSt)
	}
}

// RunClusterMonitorTask run monitor task for each cluster
func RunClusterMonitorTask(st *ClusterSt, alert *AlertSt) {
	log.Logger.Info("run cluster monitor", zap.String("node", st.Name))

	var (
		esStats       = make(map[string]interface{})
		esIndexStats  = []map[string]string{}
		wg            = &sync.WaitGroup{}
		isNotFirstRun bool
	)
	wg.Add(2)
	go loadESStats(wg, st.GetNodeStatAPI(), &esStats)
	go loadESStats(wg, st.GetIdxStatAPI(), &esIndexStats)
	wg.Wait()

	if len(esStats) == 0 {
		return
	}

	// node metrics
	// extract metric to compare without push
	metrics := extractStatsToMetricForEachNode(st.Name, esStats)
	if _, isNotFirstRun = isIndicesFirstRun.Load(st.Name); isNotFirstRun {
		go monitorNodeMetrics(st, alert, metrics) // check if need to throw alert
		for _, metric := range metrics {
			go pushMetricToES(st, metric)
		}
	}

	// index metrics
	// extract metric to compare without push
	indexMetric := extractStatsToMetricForEachIndex(esIndexStats)
	indexMetric["cluster_name"] = st.Name
	indexMetric["message"] = ""
	if _, isNotFirstRun = isIndicesFirstRun.Load(st.Name); isNotFirstRun {
		go pushMetricToES(st, indexMetric)
	}

	if _, isNotFirstRun = isIndicesFirstRun.Load(st.Name); !isNotFirstRun {
		isIndicesFirstRun.Store(st.Name, 0)
	}
}

// LoadSettings load task settings
func LoadSettings() (monitorSt *St) {
	var (
		itemI interface{}
		item  map[interface{}]interface{}
	)
	st, ok := gconfig.Shared.Get("tasks.elasticsearch-v2.configs").([]interface{})
	if !ok {
		log.Logger.Info("no elasticsearch monitor settings found")
		return
	}

	for _, itemI = range st {
		item = itemI.(map[interface{}]interface{})
		switch item["action"].(string) {
		case "monitor":
			monitorSt = ParseMonitorSettings(item)
		case "monitor-storage":
			continue
		}
	}

	return
}

// ParseMonitorSettings parse monitor task settings to struct
func ParseMonitorSettings(item map[interface{}]interface{}) (monitorSt *St) {
	var (
		itemI  interface{}
		cluStI map[interface{}]interface{}
		name   string
	)
	monitorSt = &St{
		Sts: []*ClusterSt{},
		AlertSt: &AlertSt{
			Receivers:  map[string]string{},
			Conditions: map[string]interface{}{},
		},
	}
	monitorSt.Interval = item["interval"].(int)
	for _, itemI = range item["urls"].([]interface{}) {
		cluStI = itemI.(map[interface{}]interface{})
		name = cluStI["name"].(string)
		monitorSt.Sts = append(monitorSt.Sts, &ClusterSt{
			URL:  cluStI["url"].(string),
			Push: cluStI["push"].(string),
			Name: name,
		})
	}

	for namei, addri := range item["alert"].(map[interface{}]interface{})["receivers"].(map[interface{}]interface{}) {
		monitorSt.Receivers[namei.(string)] = addri.(string)
	}

	for namei, val := range item["alert"].(map[interface{}]interface{})["conditions"].(map[interface{}]interface{}) {
		monitorSt.Conditions[namei.(string)] = val
	}

	return
}
