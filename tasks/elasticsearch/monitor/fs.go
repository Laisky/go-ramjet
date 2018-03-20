package monitor

import (
	"sync"

	"github.com/go-ramjet/utils"
)

type FSMetric struct {
	UsageRate float64 `json:"os.fs.usage_rate"`
	*IOStat
}

type IOStat struct {
	ReadKB  int `json:"os.fs.read_kb.1m"`
	WriteKB int `json:"os.fs.write_kb.1m"`
}

var nodeFSStats = &sync.Map{}

func getNodeFSStatChange(nodeName string, newMetric *FSMetric) *FSMetric {
	var currentMetric *FSMetric
	if n, ok := nodeFSStats.Load(nodeName); ok {
		lastFSMetrics := n.(*FSMetric)
		currentMetric = lastFSMetrics
		currentMetric.UsageRate = newMetric.UsageRate
		currentMetric.IOStat.ReadKB = newMetric.IOStat.ReadKB - lastFSMetrics.IOStat.ReadKB
		currentMetric.IOStat.WriteKB = newMetric.IOStat.WriteKB - lastFSMetrics.IOStat.WriteKB
	} else {
		currentMetric = &FSMetric{
			IOStat: &IOStat{
				ReadKB:  0,
				WriteKB: 0,
			},
			UsageRate: 0.0,
		}
	}
	nodeFSStats.Store(nodeName, newMetric)
	return currentMetric
}

func getFSMetric(nodeData map[string]interface{}) *FSMetric {
	nodeName := nodeData["name"].(string)
	fs := nodeData["fs"].(map[string]interface{})
	newMetric := &FSMetric{
		IOStat:    getIOStatMetric(fs),
		UsageRate: getDevicesMetric(fs),
	}
	return getNodeFSStatChange(nodeName, newMetric)
}

func getDevicesMetric(fs map[string]interface{}) float64 {
	data := fs["total"].(map[string]interface{})
	available := data["available_in_bytes"].(float64)
	total := data["total_in_bytes"].(float64)
	return 100 - utils.Round(available/total, 0.5, 2)*100
}

func getIOStatMetric(fs map[string]interface{}) *IOStat {
	stats := fs["io_stats"].(map[string]interface{})
	total := stats["total"].(map[string]interface{})
	return &IOStat{
		ReadKB:  int(total["read_kilobytes"].(float64)),
		WriteKB: int(total["write_kilobytes"].(float64)),
	}
}
