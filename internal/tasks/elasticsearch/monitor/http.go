package monitor

import "sync"

// HTTPMetric is http metric for each node
type HTTPMetric struct {
	HTTPTotalAscend int `json:"os.net.http_conn.ascend.1m"`
	HTTPOpen        int `json:"os.net.http_conn.now"`
}

var nodeHTTPStats = &sync.Map{}

func getNodeHTTPStatChange(nodeName string, newMetric *HTTPMetric) *HTTPMetric {
	var currentMetric *HTTPMetric
	if d, ok := nodeHTTPStats.Load(nodeName); ok {
		lastHTTPMetrics := d.(*HTTPMetric)
		currentMetric = lastHTTPMetrics
		currentMetric.HTTPOpen = newMetric.HTTPOpen
		currentMetric.HTTPTotalAscend = newMetric.HTTPTotalAscend - lastHTTPMetrics.HTTPTotalAscend
	} else {
		currentMetric = &HTTPMetric{
			HTTPOpen:        newMetric.HTTPOpen,
			HTTPTotalAscend: 0,
		}
	}
	nodeHTTPStats.Store(nodeName, newMetric)
	return currentMetric
}

func getHTTPMetric(nodeData map[string]interface{}) *HTTPMetric {
	nodeName := nodeData["name"].(string)
	httpMetric := nodeData["http"].(map[string]interface{})
	newMetric := &HTTPMetric{
		HTTPOpen:        int(httpMetric["current_open"].(float64)),
		HTTPTotalAscend: int(httpMetric["total_opened"].(float64)),
	}
	return getNodeHTTPStatChange(nodeName, newMetric)
}
