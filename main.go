package main

import (
	"fmt"

	"github.com/go-ramjet/tasks/store"

	log "github.com/cihub/seelog"
	_ "github.com/go-ramjet/tasks/elasticsearch"
	_ "github.com/go-ramjet/tasks/fluentd"
	_ "github.com/go-ramjet/tasks/heartbeat"
	"github.com/go-ramjet/utils"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// setupSettings setup arguments restored in viper
func setupSettings() {
	defer log.Flush()
	utils.SetupSettings()

	if viper.GetBool("debug") { // debug mode
		fmt.Println("run in debug mode")
		utils.SetupLogger("debug")
	} else { // prod mode
		fmt.Println("run in prod mode")
		utils.SetupLogger("info")
	}
}

func main() {
	defer fmt.Println("All done")
	fmt.Println("start main...")
	pflag.Bool("debug", false, "run in debug mode")
	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)

	setupSettings()

	// Bind each task here
	store.Start()
	store.Run()
}
