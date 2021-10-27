package log

import gutils "github.com/Laisky/go-utils"

var Logger *gutils.LoggerType

func init() {
	var err error
	Logger, err = gutils.NewConsoleLoggerWithName("go-ramjet",
		gutils.LoggerLevelInfo)
	if err != nil {
		panic(err)
	}
}
