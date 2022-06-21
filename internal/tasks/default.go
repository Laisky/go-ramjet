package tasks

// import all tasks
import (
	// My blog's tasks
	_ "github.com/Laisky/go-ramjet/internal/tasks/blog"
	// Elasticsearch rollover & monitor
	_ "github.com/Laisky/go-ramjet/internal/tasks/elasticsearch"
	// monitor fluentd servers
	_ "github.com/Laisky/go-ramjet/internal/tasks/fluentd"
	// self heartbeat
	_ "github.com/Laisky/go-ramjet/internal/tasks/heartbeat"
	// auto compress & upload logs
	_ "github.com/Laisky/go-ramjet/internal/tasks/logrotate/backup"
	// general monitor
	_ "github.com/Laisky/go-ramjet/internal/tasks/monitor"
	// sites ssl monitor
	_ "github.com/Laisky/go-ramjet/internal/tasks/sites"
	// zipkin routine works
	_ "github.com/Laisky/go-ramjet/internal/tasks/zipkin"
	// twitter sync task
	_ "github.com/Laisky/go-ramjet/internal/tasks/twitter"
	// crawler task
	_ "github.com/Laisky/go-ramjet/internal/tasks/crawler"
)
