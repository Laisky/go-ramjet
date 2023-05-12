// Package elasticsearch implements elasticsearch tasks.
package elasticsearch

import (
	"github.com/Laisky/go-ramjet/internal/tasks/elasticsearch/alias"
	"github.com/Laisky/go-ramjet/internal/tasks/elasticsearch/monitor"
	"github.com/Laisky/go-ramjet/internal/tasks/elasticsearch/password"
	"github.com/Laisky/go-ramjet/internal/tasks/elasticsearch/remove"
	"github.com/Laisky/go-ramjet/internal/tasks/elasticsearch/rollover"
	"github.com/Laisky/go-ramjet/internal/tasks/store"
)

// bindTask Bind tasks for Elasticsearch
func init() {
	store.TaskStore.Store("es-monitor", monitor.BindMonitorTask)
	store.TaskStore.Store("es-remove", remove.BindRemoveCPLogs)
	store.TaskStore.Store("es-rollover", rollover.BindRolloverIndices)
	store.TaskStore.Store("es-password", password.BindPasswordTask)
	store.TaskStore.Store("es-aliases", alias.BindAliasesTask)
}
