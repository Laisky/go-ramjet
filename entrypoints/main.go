package main

import (
	"fmt"

	"github.com/Laisky/go-ramjet"
	_ "github.com/Laisky/go-ramjet/tasks"
	"github.com/Laisky/go-ramjet/tasks/store"
	"github.com/Laisky/go-utils"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
)

// setupSettings setup arguments restored in viper
func setupSettings(flag *pflag.FlagSet) {
	//参数加载
	utils.Settings.BindPFlags(flag)
	//配置加载
	utils.Settings.Setup(utils.Settings.GetString("config"))

	//根据入参来区分日志输出级别
	if utils.Settings.GetBool("debug") { // debug mode
		fmt.Println("run in debug mode")
		utils.SetupLogger("debug")
	} else { // prod mode
		fmt.Println("run in prod mode")
		utils.SetupLogger("info")
	}

}

func setupCMDArgs() *pflag.FlagSet {
	pflag.Bool("debug", false, "run in debug mode")
	pflag.Bool("dry", false, "run in dry mode")
	pflag.Bool("pprof", false, "run with pprof")
	pflag.String("addr", "127.0.0.1:24087", "like `127.0.0.1:24087`")
	pflag.String("config", "/etc/go-ramjet/settings", "config file path")
	pflag.String("host", "", "hostname")
	pflag.StringSliceP("task", "t", []string{}, "which tasks want to runnning, like\n ./main -t t1,t2,heartbeat")
	pflag.StringSliceP("exclude", "e", []string{}, "which tasks do not want to runnning, like\n ./main -e t1,t2,heartbeat")
	pflag.Parse()
	return pflag.CommandLine
}

//入口
func main() {
	defer fmt.Println("All done")
	defer utils.Logger.Sync()

	//加载参数并启动邮箱
	flags := setupCMDArgs()
	setupSettings(flags)
	ramjet.Email.Setup()

	//获取参数
	utils.Logger.Info("running...",
		zap.Bool("debug", utils.Settings.GetBool("debug")),
		zap.String("addr", utils.Settings.GetString("addr")),
		zap.String("config", utils.Settings.GetString("config")),
		zap.Strings("task", utils.Settings.GetStringSlice("task")),
		zap.Strings("exclude", utils.Settings.GetStringSlice("exclude")),
	)

	// Bind each task here
	store.Start()
	go store.Run()

	// Run HTTP Server
	ramjet.RunServer(utils.Settings.GetString("addr"))
}
