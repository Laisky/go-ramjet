// Package cmd implements the root command of go-ramjet.
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v6"
	gcmd "github.com/Laisky/go-utils/v6/cmd"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/spf13/cobra"

	"github.com/Laisky/go-ramjet/internal/pkg/singleton"
	_ "github.com/Laisky/go-ramjet/internal/tasks"
	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/alert"
	"github.com/Laisky/go-ramjet/library/log"
	rlimiter "github.com/Laisky/go-ramjet/library/ratelimit"
	"github.com/Laisky/go-ramjet/library/web"
)

var rootCMD = &cobra.Command{
	Use:   "go-ramjet",
	Short: "go-ramjet",
	Long:  `go-ramjet`,
	Args:  gcmd.NoExtraArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if !initialize(ctx, cmd) {
			return nil
		}

		shutdown, err := log.ConfigureOTLP(ctx)
		if err != nil {
			return errors.Wrap(err, "configure OTLP logs")
		}
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := shutdown(shutdownCtx); err != nil {
				log.Logger.Error("flush OTLP logs", zap.Error(err))
			}
			stats := log.LogOTLPStats()
			log.Logger.Info("OTLP log shutdown", zap.Uint64("emitted", stats.Emitted), zap.Uint64("exported", stats.Exported), zap.Uint64("failed", stats.Failed), zap.Uint64("unresolved", stats.Unresolved))
		}()

		//加载参数并启动邮箱
		alert.Manager.Setup()

		// if err := alert.Telegram.SendAlert("start go-ramjet"); err != nil {
		// 	log.Logger.Error("send telegram msg", zap.Error(err))
		// }

		//获取参数
		log.Logger.Info("running...",
			zap.Bool("debug", gconfig.Shared.GetBool("debug")),
			zap.String("addr", gconfig.Shared.GetString("server.addr")),
			zap.String("config", gconfig.Shared.GetString("config")),
			zap.Strings("task", gconfig.Shared.GetStringSlice("task")),
			zap.Strings("exclude", gconfig.Shared.GetStringSlice("exclude")),
		)

		// Bind each task here
		store.TaskStore.Start(ctx)

		// Run HTTP Server
		return web.RunServerContext(ctx, gconfig.Shared.GetString("server.addr"))
	},
}

func initialize(ctx context.Context, cmd *cobra.Command) bool {
	if err := gconfig.Shared.BindPFlags(cmd.Flags()); err != nil {
		log.Logger.Panic("bind pflags", zap.Error(err))
	}

	if !setupSettings(ctx) {
		return false
	}

	setupLogger(ctx)
	return true
}

func setupSettings(_ context.Context) bool {
	var err error

	if gconfig.Shared.GetBool("version") {
		fmt.Println(gutils.PrettyBuildInfo())
		return false
	}

	//配置加载
	cfgFile := gconfig.Shared.GetString("config")
	log.Logger.Info("load config", zap.String("file", cfgFile))
	if err = gconfig.Shared.LoadFromFile(cfgFile); err != nil {
		log.Logger.Panic("setup settings", zap.Error(err))
	}

	if err = singleton.Setup(); err != nil {
		log.Logger.Panic("setup singleton", zap.Error(err))
	}

	return true
}

func setupLogger(ctx context.Context) {
	opts := []zap.Option{}

	if gconfig.Shared.GetString("logger.push_api") != "" {
		ratelimiter, err := rlimiter.New(ctx, "logger:alert", rlimiter.Args{
			Max:     1,
			NPerSec: 1,
		})
		if err != nil {
			log.Logger.Panic("create ratelimiter", zap.Error(err))
		}

		alertPusher, err := glog.NewAlert(
			ctx,
			gconfig.Shared.GetString("logger.push_api"),
			glog.WithAlertType(gconfig.Shared.GetString("logger.alert_type")),
			glog.WithAlertToken(gconfig.Shared.GetString("logger.push_token")),
			glog.WithAlertHookLevel(zap.ErrorLevel),
			glog.WithAlertMaxMessageBytes(log.HTTPAlertMaxMessageBytes),
			glog.WithRateLimiter(ratelimiter),
		)
		if err != nil {
			log.Logger.Panic("create AlertPusher", zap.Error(err))
		}

		alertCore := log.CrawlerDiagnosticAlertCore(alertPusher.GetZapCore())
		opts = append(opts, zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			return zapcore.NewTee(core, alertCore)
		}))
		log.Logger.Info("set alert",
			zap.String("alert_api", gconfig.Shared.GetString("logger.push_api")),
			zap.String("alert_type", gconfig.Shared.GetString("logger.alert_type")),
		)
	}

	hostname, err := os.Hostname()
	if err != nil {
		log.Logger.Panic("get hostname", zap.Error(err))
	}

	logger := log.Logger.WithOptions(opts...).With(
		zap.String("host", hostname),
	)
	log.Logger = logger

	//根据入参来区分日志输出级别
	if gconfig.Shared.GetBool("debug") { // debug mode
		fmt.Println("run in debug mode")
		_ = log.Logger.ChangeLevel("debug")
		gconfig.Shared.Set("log-level", "debug")
	} else { // prod mode
		fmt.Println("run in prod mode")
		_ = log.Logger.ChangeLevel("info")
		gconfig.Shared.Set("log-level", "info")
	}
}

func init() {
	rootCMD.PersistentFlags().Bool("debug", false, "run in debug mode")
	rootCMD.PersistentFlags().Bool("dry", false, "run in dry mode")
	rootCMD.PersistentFlags().Bool("pprof", false, "run with pprof")
	// rootCMD.PersistentFlags().String("addr", "127.0.0.1:24087", "like `127.0.0.1:24087`")
	rootCMD.PersistentFlags().StringP("config", "c", "/etc/go-ramjet/settings.yml", "config file path")
	// rootCMD.PersistentFlags().String("host", "127.0.0.1", "hostname")
	rootCMD.PersistentFlags().BoolP("version", "v", false, "show version")
	rootCMD.PersistentFlags().String("log-level", "info", "logger level")
	rootCMD.PersistentFlags().StringSliceP("task", "t", []string{},
		"which tasks want to runnning, like\n ./main -t t1,t2,heartbeat")
	rootCMD.PersistentFlags().StringSliceP("exclude", "e", []string{},
		"which tasks do not want to runnning, like\n ./main -e t1,t2,heartbeat")
}

// Execute run root command
func Execute() {
	if err := rootCMD.Execute(); err != nil {
		glog.Shared.Panic("start", zap.Error(err))
	}
}
