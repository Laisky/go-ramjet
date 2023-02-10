package sites

import (
	"crypto/tls"
	"fmt"
	"time"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	gutils "github.com/Laisky/go-utils/v3"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/internal/tasks/store"
	"github.com/Laisky/go-ramjet/library/alert"
	"github.com/Laisky/go-ramjet/library/log"
)

func LoadCertExpiresAt(addr string) (t time.Time, err error) {
	log.Logger.Debug("LoadCertExpiresAt", zap.String("addr", addr))
	conn, err := tls.Dial("tcp", addr, nil)
	if err != nil {
		return time.Time{}, errors.Wrapf(err, "request addr %v got error", addr)
	}
	gutils.SilentClose(conn)

	return conn.ConnectionState().VerifiedChains[0][0].NotAfter, nil
}

func checkIsTimeTooCloseToAlert(now, expiresAt time.Time, d time.Duration) (isAlert bool) {
	log.Logger.Debug("checkIsTimeTooCloseToAlert", zap.Time("now", now), zap.Time("expiresAt", expiresAt), zap.Duration("duration", d))
	return expiresAt.Sub(now) < d
}

func sendAlertEmail(addr, receiver string, expiresAt time.Time) (err error) {
	log.Logger.Info("sendAlertEmail", zap.String("addr", addr), zap.String("receiver", receiver))
	err = alert.Manager.Send(
		receiver,
		"Laisky Cai",
		"SSL Cert Nearly expires",
		fmt.Sprintf("SSL Cert [%v] Nearly expires [%v]", addr, expiresAt),
	)
	if err != nil {
		return errors.Wrapf(err, "try to send email to [%v] got error", receiver)
	}

	return nil
}

func runTask() {
	log.Logger.Info("run ssl-monitor...")
	var err error

	addr := gconfig.Shared.GetString("tasks.sites.addr")
	expiresAt, err := LoadCertExpiresAt(addr)
	if err != nil {
		log.Logger.Error("LoadCertExpiresAt got error", zap.String("addr", addr), zap.Error(err))
		return
	}

	now := time.Now()
	if checkIsTimeTooCloseToAlert(now, expiresAt, gconfig.Shared.GetDuration("tasks.sites.sslMonitor.duration")*time.Second) {
		err = sendAlertEmail(addr, gconfig.Shared.GetString("tasks.sites.receiver"), expiresAt)
		if err != nil {
			log.Logger.Error("sendAlertEmail got error", zap.String("addr", addr), zap.Error(err))
		}
	}
}

// bindTask bind ssl-monitor task
func bindTask() {
	log.Logger.Info("bind ssl-monitor task...")

	go store.TaskStore.TickerAfterRun(gconfig.Shared.GetDuration("tasks.sites.sslMonitor.interval")*time.Second, runTask)
}

func init() {
	store.TaskStore.Store("ssl-monitor", bindTask)
}
