package monitor

// Monitor ElasticSearch's metrics

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/go-utils"
	"github.com/Laisky/go-ramjet/tasks/store"
)

var (
	isIndicesFirstRun = sync.Map{}
	httpClient        = http.Client{
		Timeout: time.Second * 30,
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
	return c.Push + "monitor-stats/stats/"
}

// St is monitor task settings
type St struct {
	Sts      []*ClusterSt
	Interval int
}

func loadESStats(wg *sync.WaitGroup, url string, esStats interface{}) {
	utils.Logger.Debugf("load es stats for url %v", strings.Split(url, "@")[1])
	defer wg.Done()
	resp, err := httpClient.Get(url)
	if err != nil {
		utils.Logger.Errorf("try to get es stats got error for url %v: %+v", url, err)
		esStats = nil
		return
	}
	defer resp.Body.Close()
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		utils.Logger.Errorf("try to read es stat body got error for url %v: %+v", url, err)
		esStats = nil
		return
	}
	err = json.Unmarshal(respBytes, esStats)
	if err != nil {
		utils.Logger.Errorf("try to parse es stat got error for url %v: %+v", url, err)
		esStats = nil
		return
	}
}

// NodeMetric is the whole metric for each node
type NodeMetric struct {
	ClusterName string `json:"cluster_name"`
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

// ESEvent wrap the node metric to push to ES
type ESEvent struct {
	Index  string      `json:"_index"`
	Type   string      `json:"_type"`
	Source *NodeMetric `json:"_source"`
}

func pushMetricToES(c *ClusterSt, metric interface{}) {
	utils.Logger.Infof("push es metric to elasticsearch for node %v", c.Name)
	jsonBytes, err := json.Marshal(metric)
	if err != nil {
		utils.Logger.Error(err.Error())
		return
	}

	utils.Logger.Debugf("push es metric %v", string(jsonBytes[:]))
	if utils.Settings.GetBool("dry") {
		return
	}
	resp, err := httpClient.Post(c.GetPushMetricAPI(), utils.HTTPJSONHeader, bytes.NewBuffer(jsonBytes))
	if err != nil {
		utils.Logger.Error(err.Error())
		return
	}

	err = utils.CheckResp(resp)
	if err != nil {
		utils.Logger.Error(err.Error())
		return
	}

	utils.Logger.Debugf("success to push es metric to elasticsearch for node %v", metric)
}

// BindMonitorTask start monitor tasks
func BindMonitorTask() {
	utils.Logger.Info("bind ES monitors...")

	st := LoadSettings()
	interval := st.Interval

	if utils.Settings.GetBool("debug") { // set for debug
		interval = 3
	}

	go store.TickerAfterRun(time.Duration(interval)*time.Second, runTask)
}

func runTask() {
	st := LoadSettings()
	for _, cst := range st.Sts {
		go RunClusterMonitorTask(cst)
	}
}

// RunClusterMonitorTask run monitor task for each cluster
func RunClusterMonitorTask(st *ClusterSt) {
	utils.Logger.Infof("run cluster monitor for %v...", st.Name)

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
		for _, metric := range metrics {
			go pushMetricToES(st, metric)
		}
	}

	// index metrics
	// extract metric to compare without push
	indexMetric := extractStatsToMetricForEachIndex(esIndexStats)
	indexMetric["cluster_name"] = st.Name
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
	for _, itemI = range utils.Settings.Get("tasks.elasticsearch-v2.configs").([]interface{}) {
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

	return
}
