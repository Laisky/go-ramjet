package monitor

import (
	"strconv"
	"time"

	"github.com/go-ramjet/utils"
)

type IndexStor struct {
	IndexName string `json:"index"`
	Size      string `json:"store.size"`
}

type IndexMetric struct {
	MonitorType string `json:"monitor_type"`
	Timestamp   string `json:"@timestamp"`
	// cp
	SITCPSize  int64 `json:"es.index.sit-cp-logs.size.mb"`
	UATCPSize  int64 `json:"es.index.uat-cp-logs.size.mb"`
	PERFCPSize int64 `json:"es.index.perf-cp-logs.size.mb"`
	PRODCPSize int64 `json:"es.index.prod-cp-logs.size.mb"`
	// spring
	SITSpringSize  int64 `json:"es.index.sit-spring-logs.size.mb"`
	UATSpringSize  int64 `json:"es.index.uat-spring-logs.size.mb"`
	PERFSpringSize int64 `json:"es.index.perf-spring-logs.size.mb"`
	PRODSpringSize int64 `json:"es.index.prod-spring-logs.size.mb"`
	// gateway
	SITGatewaySize  int64 `json:"es.index.sit-gateway-logs.size.mb"`
	UATGatewaySize  int64 `json:"es.index.uat-gateway-logs.size.mb"`
	PERFGatewaySize int64 `json:"es.index.perf-gateway-logs.size.mb"`
	PRODGatewaySize int64 `json:"es.index.prod-gateway-logs.size.mb"`
	// spark
	SITSparkSize  int64 `json:"es.index.sit-spark-logs.size.mb"`
	UATSparkSize  int64 `json:"es.index.uat-spark-logs.size.mb"`
	PERFSparkSize int64 `json:"es.index.perf-spark-logs.size.mb"`
	PRODSparkSize int64 `json:"es.index.prod-spark-logs.size.mb"`
	// geely
	SITGeelySize  int64 `json:"es.index.sit-geely-logs.size.mb"`
	UATGeelySize  int64 `json:"es.index.uat-geely-logs.size.mb"`
	PERFGeelySize int64 `json:"es.index.perf-geely-logs.size.mb"`
	PRODGeelySize int64 `json:"es.index.prod-geely-logs.size.mb"`
	//
}

func extractStatsToMetricForEachIndex(indexsStat []map[string]string) *IndexMetric {
	indexMetric := &IndexMetric{
		MonitorType: "index",
		Timestamp:   utils.UTCNow().Format(time.RFC3339),
	}
	for _, stat := range indexsStat {
		indexName := stat["index"]
		indexSize := stat["store.size"]
		switch indexName {
		// cp
		case "sit-cp-logs":
			indexMetric.SITCPSize, _ = strconv.ParseInt(indexSize, 10, 64)
		case "uat-cp-logs":
			indexMetric.UATCPSize, _ = strconv.ParseInt(indexSize, 10, 64)
		case "perf-cp-logs":
			indexMetric.PERFCPSize, _ = strconv.ParseInt(indexSize, 10, 64)
		case "prod-cp-logs":
			indexMetric.PRODCPSize, _ = strconv.ParseInt(indexSize, 10, 64)
		// spring
		case "sit-spring-logs":
			indexMetric.SITSpringSize, _ = strconv.ParseInt(indexSize, 10, 64)
		case "uat-spring-logs":
			indexMetric.UATSpringSize, _ = strconv.ParseInt(indexSize, 10, 64)
		case "perf-spring-logs":
			indexMetric.PERFSpringSize, _ = strconv.ParseInt(indexSize, 10, 64)
		case "prod-spring-logs":
			indexMetric.PRODSpringSize, _ = strconv.ParseInt(indexSize, 10, 64)
		// gateway
		case "sit-gateway-logs":
			indexMetric.SITGatewaySize, _ = strconv.ParseInt(indexSize, 10, 64)
		case "uat-gateway-logs":
			indexMetric.UATGatewaySize, _ = strconv.ParseInt(indexSize, 10, 64)
		case "perf-gateway-logs":
			indexMetric.PERFGatewaySize, _ = strconv.ParseInt(indexSize, 10, 64)
		case "prod-gateway-logs":
			indexMetric.PRODGatewaySize, _ = strconv.ParseInt(indexSize, 10, 64)
		// spark
		case "sit-spark-logs":
			indexMetric.SITSparkSize, _ = strconv.ParseInt(indexSize, 10, 64)
		case "uat-spark-logs":
			indexMetric.UATSparkSize, _ = strconv.ParseInt(indexSize, 10, 64)
		case "perf-spark-logs":
			indexMetric.PERFSparkSize, _ = strconv.ParseInt(indexSize, 10, 64)
		case "prod-spark-logs":
			indexMetric.PRODSparkSize, _ = strconv.ParseInt(indexSize, 10, 64)
		// geely
		case "sit-geely-logs":
			indexMetric.SITGeelySize, _ = strconv.ParseInt(indexSize, 10, 64)
		case "uat-geely-logs":
			indexMetric.UATGeelySize, _ = strconv.ParseInt(indexSize, 10, 64)
		case "perf-geely-logs":
			indexMetric.PERFGeelySize, _ = strconv.ParseInt(indexSize, 10, 64)
		case "prod-geely-logs":
			indexMetric.PRODGeelySize, _ = strconv.ParseInt(indexSize, 10, 64)
		}
	}

	return indexMetric
}
