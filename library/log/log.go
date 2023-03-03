package log

import logsdk "github.com/Laisky/go-utils/v4/log"

var Logger logsdk.Logger

func init() {
	var err error
	Logger, err = logsdk.NewConsoleWithName("go-ramjet", logsdk.LevelInfo)
	if err != nil {
		panic(err)
	}
}
