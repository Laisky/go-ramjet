package tasks

import (
	_ "github.com/Laisky/go-ramjet/tasks/elasticsearch"
	_ "github.com/Laisky/go-ramjet/tasks/fluentd"
	_ "github.com/Laisky/go-ramjet/tasks/heartbeat"
	_ "github.com/Laisky/go-ramjet/tasks/logrotate/backup"
	_ "github.com/Laisky/go-ramjet/tasks/sites"
)
