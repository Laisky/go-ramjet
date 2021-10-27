package monitor

// OSMetric Node OS metrics
type OSMetric struct {
	*CPUMetric
	*MemMetric
}

// CPUMetric is the cpu metric for each node
type CPUMetric struct {
	CPUPercent int     `json:"os.cpu.percent"`
	CPULoad1M  float64 `json:"os.cpu.load.1m"`
	CPULoad5M  float64 `json:"os.cpu.load.5m"`
	CPULoad15M float64 `json:"os.cpu.load.15m"`
}

// MemMetric is the memory metric for each node
type MemMetric struct {
	MemPercent int `json:"os.mem.percent"`
}

func getOSMetric(nodeData map[string]interface{}) *OSMetric {
	os := nodeData["os"].(map[string]interface{})
	return &OSMetric{
		CPUMetric: getCPUMetric(os),
		MemMetric: getMemMetric(os),
	}
}

func getCPUMetric(os map[string]interface{}) (metric *CPUMetric) {
	cpu := os["cpu"].(map[string]interface{})
	load := cpu["load_average"].(map[string]interface{})
	metric = &CPUMetric{
		CPUPercent: int(cpu["percent"].(float64)),
		CPULoad1M:  load["1m"].(float64),
		CPULoad5M:  load["5m"].(float64),
		CPULoad15M: load["15m"].(float64),
	}
	return
}

func getMemMetric(os map[string]interface{}) (metric *MemMetric) {
	mem := os["mem"].(map[string]interface{})
	metric = &MemMetric{
		MemPercent: int(mem["used_percent"].(float64)),
	}
	return
}
