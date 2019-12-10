package tasks

import (
	_ "github.com/Laisky/go-ramjet/tasks/blog"
	_ "github.com/Laisky/go-ramjet/tasks/elasticsearch"
	_ "github.com/Laisky/go-ramjet/tasks/fluentd"
	_ "github.com/Laisky/go-ramjet/tasks/heartbeat"
	_ "github.com/Laisky/go-ramjet/tasks/logrotate/backup"
	_ "github.com/Laisky/go-ramjet/tasks/monitor"
	_ "github.com/Laisky/go-ramjet/tasks/sites"
	_ "github.com/Laisky/go-ramjet/tasks/zipkin"
)
