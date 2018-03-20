package main

import (
	"fmt"

	"pateo.com/go-ramjet/tasks/store"

	log "github.com/cihub/seelog"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	_ "pateo.com/go-ramjet/tasks/elasticsearch"
	_ "pateo.com/go-ramjet/tasks/fluentd"
	_ "pateo.com/go-ramjet/tasks/heartbeat"
	"pateo.com/go-ramjet/utils"
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
