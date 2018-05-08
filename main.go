package main

import (
	"fmt"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	_ "github.com/Laisky/go-ramjet/tasks/elasticsearch"
	_ "github.com/Laisky/go-ramjet/tasks/fluentd"
	_ "github.com/Laisky/go-ramjet/tasks/heartbeat"
	_ "github.com/Laisky/go-ramjet/tasks/logrotate/backup"
	"github.com/Laisky/go-ramjet/tasks/store"
	"github.com/Laisky/go-ramjet/utils"
)

// setupSettings setup arguments restored in viper
func setupSettings() {
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
	pflag.Bool("dry", false, "run in dry mode")
	pflag.StringSliceP("task", "t", []string{}, "which tasks want to runnning")
	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)

	setupSettings()

	// Bind each task here
	store.Start()
	store.Run()
}
