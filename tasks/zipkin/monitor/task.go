package monitor

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	ramjet "github.com/Laisky/go-ramjet"
	"github.com/Laisky/go-ramjet/tasks/store"
	"github.com/Laisky/go-utils"
	"go.uber.org/zap"
)

var (
	httpClient = &http.Client{
		Timeout: 5 * time.Second,
	}
)

func runTask() {
	utils.Logger.Info("run zipkin-monitor")
	defer utils.Logger.Info("zipkin-monitor done")

	alert := ""
	result := &sync.Map{}
	wg := &sync.WaitGroup{}

	for name, urli := range utils.Settings.Get("tasks.zipkin.monitor.configs").(map[string]interface{}) {
		url := urli.(string)
		wg.Add(1)
		checkEndpointHealth(wg, name, url, result)
	}

	wg.Wait()
	for name := range utils.Settings.Get("tasks.zipkin.monitor.configs").(map[string]interface{}) {
		if health, ok := result.Load(name); !ok {
			alert += fmt.Sprintf("cannot check zipkin-server `%v`\n", name)
		} else if !health.(bool) {
			alert += fmt.Sprintf("zipkin-server `%v` is not health\n", name)
		}
	}

	if alert != "" {
		alert = time.Now().Format(time.RFC3339) + "\n" + alert
		if err := ramjet.Email.Send(
			"ppcelery@gmail.com",
			"Laisky Cai",
			"[pateo]zipkin-server got problem",
			alert,
		); err != nil {
			utils.Logger.Error("try to send zipkin-monitor alert email got error", zap.Error(err))
		}
	}
}

func BindTask() {
	utils.Logger.Info("bind zipkin-monitor")
	go store.TickerAfterRun(utils.Settings.GetDuration("tasks.zipkin.monitor.interval")*time.Second, runTask)
}

func checkEndpointHealth(wg *sync.WaitGroup, name, url string, result *sync.Map) {
	defer wg.Done()
	resp, err := httpClient.Get(url)
	if err != nil {
		utils.Logger.Error("try to request url got error",
			zap.Error(err),
			zap.String("name", name),
			zap.String("url", url))
		return
	}
	defer resp.Body.Close()

	if err = utils.CheckResp(resp); err != nil {
		utils.Logger.Error("request url return error",
			zap.Error(err),
			zap.String("name", name),
			zap.String("url", url))
		result.Store(name, false)
		return
	}

	utils.Logger.Debug("zipkin-server health",
		zap.String("name", name),
		zap.String("url", url))
	result.Store(name, true)
}
