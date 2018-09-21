package elasticsearch

import (
	"github.com/Laisky/go-ramjet/tasks/elasticsearch/alias"
	"github.com/Laisky/go-ramjet/tasks/elasticsearch/monitor"
	"github.com/Laisky/go-ramjet/tasks/elasticsearch/password"
	"github.com/Laisky/go-ramjet/tasks/elasticsearch/remove"
	"github.com/Laisky/go-ramjet/tasks/elasticsearch/rollover"
	"github.com/Laisky/go-ramjet/tasks/store"
)

// bindTask Bind tasks for Elasticsearch
func init() {
	store.Store("es-monitor", monitor.BindMonitorTask)
	store.Store("es-remove", remove.BindRemoveCPLogs)
	store.Store("es-rollover", rollover.BindRolloverIndices)
	store.Store("es-password", password.BindPasswordTask)
	store.Store("es-aliases", alias.BindAliasesTask)
}
