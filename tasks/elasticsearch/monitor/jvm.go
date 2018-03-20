package monitor

import "sync"

type JVMMetric struct {
	GCPauseTotal int `json:"es.jvm.gc.pause.total.1m"`
	HeapUsage    int `json:"es.jvm.heap.usage.1m"`
}

var nodeJVMStats = &sync.Map{}

func getJVMStatChange(nodeName string, newMetric *JVMMetric) *JVMMetric {
	var currentMetric *JVMMetric
	if n, ok := nodeJVMStats.Load(nodeName); ok {
		lastJVMStateMetric := n.(*JVMMetric)
		currentMetric = lastJVMStateMetric
		currentMetric.HeapUsage = newMetric.HeapUsage
		currentMetric.GCPauseTotal = newMetric.GCPauseTotal - lastJVMStateMetric.GCPauseTotal
	} else {
		currentMetric = &JVMMetric{
			GCPauseTotal: 0,
			HeapUsage:    0,
		}
	}
	nodeJVMStats.Store(nodeName, newMetric)
	return currentMetric
}

func getJVMMetric(nodeData map[string]interface{}) *JVMMetric {
	nodeName := nodeData["name"].(string)
	jvm := nodeData["jvm"].(map[string]interface{})
	newMetric := &JVMMetric{
		HeapUsage:    getHeapUsage(jvm),
		GCPauseTotal: getJVMGCPause(jvm),
	}
	return getJVMStatChange(nodeName, newMetric)
}

func getJVMGCPause(jvm map[string]interface{}) int {
	gc := jvm["gc"].(map[string]interface{})
	collectors := gc["collectors"].(map[string]interface{})
	young := collectors["young"].(map[string]interface{})
	old := collectors["old"].(map[string]interface{})
	return int(young["collection_time_in_millis"].(float64) + old["collection_time_in_millis"].(float64))
}

func getHeapUsage(jvm map[string]interface{}) int {
	mem := jvm["mem"].(map[string]interface{})
	return int(mem["heap_used_percent"].(float64))
}
