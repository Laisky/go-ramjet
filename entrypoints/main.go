package main

import (
	"context"
	"fmt"

	"github.com/Laisky/go-ramjet/alert"

	"github.com/Laisky/go-ramjet"
	_ "github.com/Laisky/go-ramjet/tasks"
	"github.com/Laisky/go-ramjet/tasks/store"
	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/spf13/pflag"
)

// setupSettings setup arguments restored in viper
func setupSettings(flag *pflag.FlagSet) {
	var err error
	//参数加载
	if err = utils.Settings.BindPFlags(flag); err != nil {
		utils.Logger.Panic("BindPFlags", zap.Error(err))
	}
	//配置加载
	if err = utils.Settings.Setup(utils.Settings.GetString("config")); err != nil {
		utils.Logger.Panic("setup settings", zap.Error(err))
	}

	//根据入参来区分日志输出级别
	if utils.Settings.GetBool("debug") { // debug mode
		fmt.Println("run in debug mode")
		_ = utils.Logger.ChangeLevel("debug")
		utils.Settings.Set("log-level", "debug")
	} else { // prod mode
		fmt.Println("run in prod mode")
		_ = utils.Logger.ChangeLevel("info")
		utils.Settings.Set("log-level", "info")
	}

}

func setupLogger(ctx context.Context) {
	// log
	alertPusher, err := utils.NewAlertPusherWithAlertType(
		ctx,
		utils.Settings.GetString("logger.push_api"),
		utils.Settings.GetString("logger.alert_type"),
		utils.Settings.GetString("logger.push_token"),
	)
	if err != nil {
		utils.Logger.Panic("create AlertPusher", zap.Error(err))
	}

	hook := utils.NewAlertHook(alertPusher)
	if _, err := utils.SetDefaultLogger(
		"go-ramjet:"+utils.Settings.GetString("host"),
		utils.Settings.GetString("log-level"),
		zap.HooksWithFields(hook.GetZapHook())); err != nil {
		utils.Logger.Panic("setup logger", zap.Error(err))
	}
}

func setupCMDArgs() *pflag.FlagSet {
	pflag.Bool("debug", false, "run in debug mode")
	pflag.Bool("dry", false, "run in dry mode")
	pflag.Bool("pprof", false, "run with pprof")
	pflag.String("addr", "127.0.0.1:24087", "like `127.0.0.1:24087`")
	pflag.String("config", "/etc/go-ramjet/settings", "config file path")
	pflag.String("host", "", "hostname")
	pflag.String("log-level", "", "logger level")
	pflag.StringSliceP("task", "t", []string{}, "which tasks want to runnning, like\n ./main -t t1,t2,heartbeat")
	pflag.StringSliceP("exclude", "e", []string{}, "which tasks do not want to runnning, like\n ./main -e t1,t2,heartbeat")
	pflag.Parse()
	return pflag.CommandLine
}

//入口
func main() {
	defer fmt.Println("All done")
	ctx := context.Background()

	//加载参数并启动邮箱
	flags := setupCMDArgs()
	setupSettings(flags)
	setupLogger(ctx)
	alert.Manager.Setup()

	if err := alert.Telegram.SendAlert("start go-ramjet"); err != nil {
		utils.Logger.Error("send telegram msg", zap.Error(err))
	}

	//获取参数
	utils.Logger.Info("running...",
		zap.Bool("debug", utils.Settings.GetBool("debug")),
		zap.String("addr", utils.Settings.GetString("addr")),
		zap.String("config", utils.Settings.GetString("config")),
		zap.Strings("task", utils.Settings.GetStringSlice("task")),
		zap.Strings("exclude", utils.Settings.GetStringSlice("exclude")),
	)

	// Bind each task here
	store.TaskStore.Start(ctx)

	// Run HTTP Server
	ramjet.RunServer(utils.Settings.GetString("addr"))
}
