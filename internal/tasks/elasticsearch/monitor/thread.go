// Package monitor implements monitor task.
package monitor

type ThreadMetric struct {
	Index      int `json:"es.threadpool.queue.index.1m"`
	Search     int `json:"es.threadpool.queue.search.1m"`
	Get        int `json:"es.threadpool.queue.get.1m"`
	Bulk       int `json:"es.threadpool.queue.bulk.1m"`
	Management int `json:"es.threadpool.queue.management.1m"`
	Generic    int `json:"es.threadpool.queue.generic.1m"`
}

func getThreadMetric(nodeData map[string]interface{}) *ThreadMetric {
	ths := nodeData["thread_pool"].(map[string]interface{})
	newMetric := &ThreadMetric{}
	for name, d := range ths {
		opsstat := d.(map[string]interface{})
		switch name {
		case "index":
			newMetric.Index = int(opsstat["queue"].(float64))
		case "search":
			newMetric.Search = int(opsstat["queue"].(float64))
		case "get":
			newMetric.Get = int(opsstat["queue"].(float64))
		case "bulk":
			newMetric.Bulk = int(opsstat["queue"].(float64))
		case "management":
			newMetric.Management = int(opsstat["queue"].(float64))
		case "generic":
			newMetric.Generic = int(opsstat["queue"].(float64))
		}
	}
	return newMetric
}
