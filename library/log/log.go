// Package log implements log.
package log

import (
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
)

var Logger glog.Logger

func init() {
	var err error
	Logger, err = glog.NewConsoleWithName("go-ramjet", glog.LevelInfo)
	if err != nil {
		panic(err)
	}

	Logger.WithOptions(zap.HooksWithFields())
}
