package elasticsearch

import (
	"github.com/go-ramjet/tasks/elasticsearch/monitor"
	"github.com/go-ramjet/tasks/elasticsearch/remove"
	"github.com/go-ramjet/tasks/store"
	"github.com/spf13/viper"
)

func setupTaskSettings() {
	if viper.GetBool("debug") { // set for debug
		viper.Set("tasks.elasticsearch.interval", 1)
		viper.Set("tasks.elasticsearch.batch", 1)
	}

}

// bindTask Bind tasks for Elasticsearch
func bindTask() {
	setupTaskSettings()

	// remove ES documents
	remove.BindRemoveCPLogs()

	// ES monitor
	monitor.BindMonitorTask()
}

func init() {
	store.Store(bindTask)
}
