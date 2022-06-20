package log

import gutils "github.com/Laisky/go-utils/v2"

var Logger gutils.LoggerItf

func init() {
	var err error
	Logger, err = gutils.NewConsoleLoggerWithName("go-ramjet",
		gutils.LoggerLevelInfo)
	if err != nil {
		panic(err)
	}
}
