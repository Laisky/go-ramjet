package monitor

import (
	"regexp"
	"strconv"
	"time"

	"github.com/Laisky/go-ramjet/library/log"

	chaining "github.com/Laisky/go-chaining"
	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
)

var (
	jsonPrefix    = "es.index."
	jsonSuffix    = ".size.mb"
	idxNameLayout = `^(\w+-\w+-\w+)(-.*)?$|` + // `perf-connector-logs-2018.10.21-000143`
		`^(zipkin_\w+)(:span-.*)?$` // `zipkin_perf:span-2018-08-24`
	idxNameReg *regexp.Regexp
)

type IndexStor struct {
	IndexName string `json:"index"`
	Size      string `json:"store.size"`
}

func GetESMetricName(name string) string {
	return jsonPrefix + name + jsonSuffix
}

func extractStatsToMetricForEachIndex(indexsStat []map[string]string) (metric map[string]interface{}) {
	var (
		indexName string
		indexSize int64
		err       error
	)
	metric = map[string]interface{}{
		"monitor_type": "index",
		"@timestamp":   utils.UTCNow().Format(time.RFC3339),
	}
	for _, stat := range indexsStat {
		indexName = stat["index"]
		indexSize, err = strconv.ParseInt(stat["store.size"], 10, 64)
		if err != nil {
			log.Logger.Error("parse es storage int got error",
				zap.String("index", indexName),
				zap.String("indexSize", stat["store.size"]))
			indexSize = 0
		}

		metric[indexName] = indexSize
	}

	return chaining.New(metric, nil).
		Next(ignoreInvalidIdxNames).
		Next(combineIdxNames).
		Next(normalizeIdxNames).
		GetVal().(map[string]interface{})
}

func ignoreInvalidIdxNames(c *chaining.Chain) (interface{}, error) {
	newMap := map[string]interface{}{}
	for k, v := range c.GetMapStringInterface() {
		if string(k[0]) == "." { // ignore
			continue
		}
		newMap[k] = v
	}

	return newMap, nil
}

func normalizeIdxNames(c *chaining.Chain) (interface{}, error) {
	newMap := map[string]interface{}{}
	for key, v := range c.GetMapStringInterface() {
		if idxNameReg.MatchString(key) {
			key = GetESMetricName(key)
		}

		newMap[key] = v
	}

	return newMap, nil
}

func combineIdxNames(c *chaining.Chain) (interface{}, error) {
	var (
		newMap  = map[string]interface{}{}
		matched []string
		idxName string
		sv      interface{}
		ok      bool
	)
	for key, v := range c.GetMapStringInterface() {
		if matched = idxNameReg.FindStringSubmatch(key); len(matched) > 4 {
			if matched[1] != "" {
				idxName = matched[1]
			} else {
				idxName = matched[3]
			}

			if sv, ok = newMap[idxName]; !ok { // combine
				newMap[idxName] = v
			} else {
				newMap[idxName] = sv.(int64) + v.(int64)
			}
			continue
		}

		newMap[key] = v
	}

	return newMap, nil
}

func init() {
	idxNameReg = regexp.MustCompile(idxNameLayout)
}
