package monitor

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Laisky/errors"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v3"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	alertManager "github.com/Laisky/go-ramjet/library/alert"
	"github.com/Laisky/go-ramjet/library/log"
)

var (
	httpClient = &http.Client{
		Timeout: 5 * time.Second,
	}
)

func runTask() {
	log.Logger.Info("run monitor")
	defer log.Logger.Info("monitor done")

	result := &sync.Map{}
	wg := &sync.WaitGroup{}

	for name := range gconfig.Shared.Get("tasks.monitor.tenants").(map[string]interface{}) {
		wg.Add(1)
		switch gconfig.Shared.GetString("tasks.monitor.tenants." + name + ".type") {
		case "http":
			checkHealthByHTTP(wg, name, gconfig.Shared.GetString("tasks.monitor.tenants."+name+".url"), result)
		default:
			log.Logger.Error("unknown type",
				zap.String("type", gconfig.Shared.GetString("tasks.monitor.tenants."+name+".type")))
		}
	}

	wg.Wait()
	alertForReceivers := map[string]string{}
	for name := range gconfig.Shared.Get("tasks.monitor.tenants").(map[string]interface{}) {
		for _, receiver := range gconfig.Shared.GetStringSlice("tasks.monitor.tenants." + name + ".receivers") {
			var (
				alert string
				ok    bool
			)
			if alert, ok = alertForReceivers[receiver]; !ok {
				alert = ""
			}

			if err, ok := result.Load(name); !ok {
				log.Logger.Error("should contains monitor task", zap.String("name", name))
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

		alert = fmt.Sprintf("tested from: %v\n\n", gconfig.Shared.GetString("host")) + alert
		alert = time.Now().Format(time.RFC3339) + "\n" + alert
		if err := alertManager.Manager.Send(
			gconfig.Shared.GetString("tasks.monitor.receivers."+receiver),
			receiver,
			"[google]ramjet monitor report",
			alert,
		); err != nil {
			log.Logger.Error("try to send monitor alert email got error", zap.Error(err))
		}
	}

}

func BindTask() {
	log.Logger.Info("bind monitor")
	go store.TaskStore.TickerAfterRun(gconfig.Shared.GetDuration("tasks.monitor.interval")*time.Second, runTask)
}

func checkHealthByHTTP(wg *sync.WaitGroup, name, url string, result *sync.Map) {
	log.Logger.Debug("checkHealthByHTTP", zap.String("name", name), zap.String("url", url))
	defer wg.Done()
	resp, err := httpClient.Get(url)
	if err != nil {
		log.Logger.Warn("try to request url got error",
			zap.Error(err),
			zap.String("name", name),
			zap.String("url", url))
		result.Store(name, errors.Wrap(err, "try to request url got error"))
		return
	}
	defer gutils.SilentClose(resp.Body)

	if err = gutils.CheckResp(resp); err != nil {
		log.Logger.Warn("request url return error",
			zap.Error(err),
			zap.String("name", name),
			zap.String("url", url))
		result.Store(name, errors.Wrap(err, "request url return error"))
		return
	}

	log.Logger.Debug("monitor task is good",
		zap.String("name", name),
		zap.String("url", url))
	result.Store(name, nil)
}

func init() {
	store.TaskStore.Store("monitor", BindTask)
}
