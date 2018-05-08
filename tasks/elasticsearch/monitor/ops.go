package monitor

import "sync"

// OperatorsMetric is the operations metric that happened on each node
type OperatorsMetric struct {
	*IndexingMetric
	*GetMetric
	*SearchMetric
}

// IndexingMetric indexing metric
type IndexingMetric struct {
	Index  int `json:"es.operator.index.1m"`
	Delete int `json:"es.operator.delete.1m"`
}

// GetMetric get metric
type GetMetric struct {
	Get int `json:"es.operator.get.1m"`
}

// SearchMetric search metric
type SearchMetric struct {
	Query  int `json:"es.operator.query.1m"`
	Fetch  int `json:"es.operator.fetch.1m"`
	Scroll int `json:"es.operator.scroll.1m"`
}

// nodeOperatorStats: map[nodeName]lastOperatorsMetric
// save `lastOperatorsMetric` for each node
var nodeOperatorStats = &sync.Map{}

func getNodeOpsStatChange(nodeName string, newMetric *OperatorsMetric) *OperatorsMetric {
	var currentMetric *OperatorsMetric
	if n, ok := nodeOperatorStats.Load(nodeName); ok {
		lastOperatorsMetrics := n.(*OperatorsMetric)
		currentMetric = lastOperatorsMetrics
		currentMetric.IndexingMetric.Index = newMetric.IndexingMetric.Index - lastOperatorsMetrics.IndexingMetric.Index
		currentMetric.IndexingMetric.Delete = newMetric.IndexingMetric.Delete - lastOperatorsMetrics.IndexingMetric.Delete
		currentMetric.GetMetric.Get = newMetric.GetMetric.Get - lastOperatorsMetrics.GetMetric.Get
		currentMetric.SearchMetric.Query = newMetric.SearchMetric.Query - lastOperatorsMetrics.SearchMetric.Query
		currentMetric.SearchMetric.Fetch = newMetric.SearchMetric.Fetch - lastOperatorsMetrics.SearchMetric.Fetch
		currentMetric.SearchMetric.Scroll = newMetric.SearchMetric.Scroll - lastOperatorsMetrics.SearchMetric.Scroll
	} else {
		currentMetric = &OperatorsMetric{
			IndexingMetric: &IndexingMetric{Index: 0, Delete: 0},
			GetMetric:      &GetMetric{Get: 0},
			SearchMetric:   &SearchMetric{Query: 0, Fetch: 0, Scroll: 0},
		}
	}
	nodeOperatorStats.Store(nodeName, newMetric)
	return currentMetric
}

func getOperatorsMetric(nodeData map[string]interface{}) *OperatorsMetric {
	nodeName := nodeData["name"].(string)
	indices := nodeData["indices"].(map[string]interface{})
	newMetric := &OperatorsMetric{
		IndexingMetric: getIndexingMetric(indices),
		GetMetric:      getGetMetric(indices),
		SearchMetric:   getSearchMetric(indices),
	}
	return getNodeOpsStatChange(nodeName, newMetric)
}

func getIndexingMetric(indices map[string]interface{}) *IndexingMetric {
	indexing := indices["indexing"].(map[string]interface{})
	return &IndexingMetric{
		Index:  int(indexing["index_total"].(float64)),
		Delete: int(indexing["delete_total"].(float64)),
	}
}

func getGetMetric(indices map[string]interface{}) *GetMetric {
	get := indices["get"].(map[string]interface{})
	return &GetMetric{
		Get: int(get["total"].(float64)),
	}
}

func getSearchMetric(indices map[string]interface{}) *SearchMetric {
	indexing := indices["search"].(map[string]interface{})
	return &SearchMetric{
		Query:  int(indexing["query_total"].(float64)),
		Fetch:  int(indexing["fetch_total"].(float64)),
		Scroll: int(indexing["scroll_total"].(float64)),
	}
}
