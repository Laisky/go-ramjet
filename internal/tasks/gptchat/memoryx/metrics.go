package memoryx

import (
	"expvar"
)

var (
	memoryBeforeTurnTotal    = expvar.NewInt("memory_before_turn_total")
	memoryBeforeTurnFail     = expvar.NewInt("memory_before_turn_fail_total")
	memoryAfterTurnTotal     = expvar.NewInt("memory_after_turn_total")
	memoryAfterTurnFail      = expvar.NewInt("memory_after_turn_fail_total")
	memoryRecallFactCount    = expvar.NewInt("memory_recall_fact_count")
	memoryBeforeLatencyMs    = expvar.NewMap("memory_before_latency_ms")
	memoryAfterLatencyMs     = expvar.NewMap("memory_after_latency_ms")
	memoryBeforeLatencyCount = expvar.NewInt("memory_before_latency_count")
	memoryAfterLatencyCount  = expvar.NewInt("memory_after_latency_count")
)

func observeLatencyHistogram(hist *expvar.Map, latencyMs int64) {
	bucket := "1000+"
	switch {
	case latencyMs < 10:
		bucket = "0-9"
	case latencyMs < 50:
		bucket = "10-49"
	case latencyMs < 100:
		bucket = "50-99"
	case latencyMs < 300:
		bucket = "100-299"
	case latencyMs < 1000:
		bucket = "300-999"
	}

	hist.Add(bucket, 1)
	hist.Add("sum_ms", latencyMs)
	hist.Add("last_ms", latencyMs)
}
