// Package log implements log.
package log

import (
	glog "github.com/Laisky/go-utils/v6/log"
)

// rootLoggerName is the application root logger name; reviewed alert scopes are named below it.
const rootLoggerName = "go-ramjet"

// Logger is the application root logger.
var Logger glog.Logger

func init() {
	var err error
	Logger, err = glog.NewConsoleWithName(rootLoggerName, glog.LevelInfo)
	if err != nil {
		panic(err)
	}

	Logger = Logger.WithOptions(privateOTLPOption())
}
