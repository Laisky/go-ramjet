package main

import (
	"fmt"

	"github.com/Laisky/go-ramjet"
	_ "github.com/Laisky/go-ramjet/tasks"
	"github.com/Laisky/go-ramjet/tasks/store"
	"github.com/Laisky/go-utils"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// setupSettings setup arguments restored in viper
func setupSettings() {
	utils.Settings.Setup(utils.Settings.GetString("config"))

	if utils.Settings.GetBool("debug") { // debug mode
		fmt.Println("run in debug mode")
		utils.SetupLogger("debug")
	} else { // prod mode
		fmt.Println("run in prod mode")
		utils.SetupLogger("info")
	}
}

func setupCMDArgs() {
	pflag.Bool("debug", false, "run in debug mode")
	pflag.Bool("dry", false, "run in dry mode")
	pflag.Bool("pprof", false, "run with pprof")
	pflag.String("addr", "127.0.0.1:24087", "like `127.0.0.1:24087`")
	pflag.String("config", "/etc/go-ramjet/settings", "config file directory path")
	pflag.StringSliceP("task", "t", []string{}, "which tasks want to runnning, like\n ./main -t t1,t2,heartbeat")
	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)
}

func main() {
	defer fmt.Println("All done")
	defer utils.Logger.Flush()
	fmt.Println("start main...")

	setupCMDArgs()
	setupSettings()
	ramjet.Email.Setup()

	// Bind each task here
	store.Start()
	go store.Run()

	// Run HTTP Server
	ramjet.RunServer(utils.Settings.GetString("addr"))
}
