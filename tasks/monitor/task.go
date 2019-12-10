package monitor

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	alertManager "github.com/Laisky/go-ramjet/alert"

	"github.com/Laisky/go-ramjet/tasks/store"
	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/pkg/errors"
)

var (
	httpClient = &http.Client{
		Timeout: 5 * time.Second,
	}
)

func runTask() {
	utils.Logger.Info("run monitor")
	defer utils.Logger.Info("monitor done")

	result := &sync.Map{}
	wg := &sync.WaitGroup{}

	for name := range utils.Settings.Get("tasks.monitor.tenants").(map[string]interface{}) {
		wg.Add(1)
		switch utils.Settings.GetString("tasks.monitor.tenants." + name + ".type") {
		case "http":
			checkHealthByHTTP(wg, name, utils.Settings.GetString("tasks.monitor.tenants."+name+".url"), result)
		default:
			utils.Logger.Error("unknown type",
				zap.String("type", utils.Settings.GetString("tasks.monitor.tenants."+name+".type")))
		}
	}

	wg.Wait()
	alertForReceivers := map[string]string{}
	for name := range utils.Settings.Get("tasks.monitor.tenants").(map[string]interface{}) {
		for _, receiver := range utils.Settings.GetStringSlice("tasks.monitor.tenants." + name + ".receivers") {
			var (
				alert string
				ok    bool
			)
			if alert, ok = alertForReceivers[receiver]; !ok {
				alert = ""
			}

			if err, ok := result.Load(name); !ok {
				utils.Logger.Error("should contains monitor task", zap.String("name", name))
				alert += fmt.Sprintf("should contains monitor task `%v`\n", name)
				alert += "\n   ------------------\n\n"
			} else if err != nil {
				alert += fmt.Sprintf("monitor task `%v` is not health: %+v\n", name, err)
				alert += "\n   ------------------\n\n"
			}

			alertForReceivers[receiver] = alert
		}
	}

	for receiver, alert := range alertForReceivers {
		if alert == "" {
			continue
		}

		alert = fmt.Sprintf("tested from: %v\n\n", utils.Settings.GetString("host")) + alert
		alert = time.Now().Format(time.RFC3339) + "\n" + alert
		if err := alertManager.Manager.Send(
			utils.Settings.GetString("tasks.monitor.receivers."+receiver),
			receiver,
			"[google]ramjet monitor report",
			alert,
		); err != nil {
			utils.Logger.Error("try to send monitor alert email got error", zap.Error(err))
		}
	}

}

func BindTask() {
	utils.Logger.Info("bind monitor")
	go store.TaskStore.TickerAfterRun(utils.Settings.GetDuration("tasks.monitor.interval")*time.Second, runTask)
}

func checkHealthByHTTP(wg *sync.WaitGroup, name, url string, result *sync.Map) {
	utils.Logger.Debug("checkHealthByHTTP", zap.String("name", name), zap.String("url", url))
	defer wg.Done()
	resp, err := httpClient.Get(url)
	if err != nil {
		utils.Logger.Warn("try to request url got error",
			zap.Error(err),
			zap.String("name", name),
			zap.String("url", url))
		result.Store(name, errors.Wrap(err, "try to request url got error"))
		return
	}
	defer resp.Body.Close()

	if err = utils.CheckResp(resp); err != nil {
		utils.Logger.Warn("request url return error",
			zap.Error(err),
			zap.String("name", name),
			zap.String("url", url))
		result.Store(name, errors.Wrap(err, "request url return error"))
		return
	}

	utils.Logger.Debug("monitor task is good",
		zap.String("name", name),
		zap.String("url", url))
	result.Store(name, nil)
}

func init() {
	store.TaskStore.Store("monitor", BindTask)
}
