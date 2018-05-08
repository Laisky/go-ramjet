// Package utils 一些常用工具
package utils

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/astaxie/beego"
	log "github.com/cihub/seelog"
	viper "github.com/spf13/viper"
)

// UTCNow 获取当前 UTC 时间
func UTCNow() time.Time {
	return time.Now().UTC()
}

// GetFunctionName get func name by func
func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

// SetupLogger 初始化日志
func SetupLogger(logLevel string) {
	args := struct {
		LogLevel string
	}{
		LogLevel: logLevel,
	}
	logConfig := `
		<seelog type="asynctimer" asyncinterval="1000000" minlevel="{{.LogLevel}}" maxlevel="error">
			<exceptions>
				<exception funcpattern="*main.test*Something*" minlevel="{{.LogLevel}}"/>
				<exception filepattern="*main.go" minlevel="{{.LogLevel}}"/>
			</exceptions>
			<outputs formatid="main">
				<console/>  <!-- 输出到控制台 -->
			</outputs>
			<formats>
				<format id="main" format="[%UTCDate(2006-01-02T15:04:05.000000Z) - %LEVEL - %RelFile:%Line] %Msg%n"/>
			</formats>
		</seelog>
	`
	tmpl, err := template.New("seelogConfig").Parse(logConfig)
	if err != nil {
		panic(err.Error())
	}
	var configBytes bytes.Buffer
	if err := tmpl.Execute(&configBytes, args); err != nil {
		panic(err.Error())
	}
	logger, err := log.LoggerFromConfigAsBytes(configBytes.Bytes())
	if err != nil {
		panic(err.Error())
	}
	logger.Info("SetupLogger ok")
	log.ReplaceLogger(logger)
}

// GetRunmode 获取运行模式
func GetRunmode() string {
	Runmode := os.Getenv("DOCKERKIT_RUNMODE")
	if Runmode == "" {
		Runmode = beego.AppConfig.String("runmode")
	}
	if Runmode == "" {
		Runmode = "dev"
	}

	Runmode = strings.ToLower(Runmode)
	return Runmode
}

// LoadSettings load settings file
func LoadSettings() {
	err := viper.ReadInConfig() // Find and read the config file
	// log.Info("load settings")
	if err != nil { // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s", err))
	}
}

// GetFuncName return the name of func
func GetFuncName(f interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

// FallBack return the fallback when orig got error
func FallBack(orig func() interface{}, fallback interface{}) (ret interface{}) {
	defer func() {
		if recover() != nil {
			ret = fallback
		}
	}()

	ret = orig()
	return ret
}

// SetupSettings load config file settings.yml
func SetupSettings() {
	viper.SetConfigType("yaml")
	viper.SetConfigName("settings") // name of config file (without extension)
	viper.AddConfigPath("/etc/go-ramjet/settings/")
	viper.AddConfigPath(os.Getenv("GOPATH") + "/src/github.com/Laisky/go-ramjet/settings/")
	viper.AddConfigPath(".")

	LoadSettings()
	// WatchSettingsFileChange()
}
