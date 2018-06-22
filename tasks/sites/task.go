package sites

import (
	"crypto/tls"
	"fmt"
	"time"

	ramjet "github.com/Laisky/go-ramjet"
	"github.com/Laisky/go-ramjet/tasks/store"
	utils "github.com/Laisky/go-utils"
	"github.com/pkg/errors"
)

func LoadCertExpiresAt(addr string) (t *time.Time, err error) {
	utils.Logger.Debugf("LoadCertExpiresAt for addr %v", addr)
	conn, err := tls.Dial("tcp", addr, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "request addr %v got error", addr)
	}
	defer conn.Close()

	return &conn.ConnectionState().VerifiedChains[0][0].NotAfter, nil
}

func checkIsTimeTooCloseToAlert(now, expiresAt *time.Time, d time.Duration) (isAlert bool) {
	utils.Logger.Debugf("checkIsTimeTooCloseToAlert for now %v, t %v, d %v", now, expiresAt, d)
	return expiresAt.Sub(*now) < d
}

func sendAlertEmail(addr, receiver string, expiresAt *time.Time) (err error) {
	utils.Logger.Infof("sendAlertEmail for addr %v, receiver %v", addr, receiver)
	err = ramjet.Email.Send(
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
	utils.Logger.Infof("run ssl-monitor...")
	var err error

	addr := utils.Settings.GetString("tasks.sites.addr")
	expiresAt, err := LoadCertExpiresAt(addr)
	if err != nil {
		utils.Logger.Errorf("LoadCertExpiresAt for addr %v got error: %+v", addr, err)
		return
	}

	now := time.Now()
	if checkIsTimeTooCloseToAlert(&now, expiresAt, utils.Settings.GetDuration("tasks.sites.sslMonitor.duration")*time.Second) {
		err = sendAlertEmail(addr, utils.Settings.GetString("tasks.sites.receiver"), expiresAt)
		if err != nil {
			utils.Logger.Errorf("sendAlertEmail for addr %v got error: %+v", addr, err)
		}
	}
}

// bindTask bind ssl-monitor task
func bindTask() {
	utils.Logger.Info("bind ssl-monitor task...")

	go store.TickerAfterRun(utils.Settings.GetDuration("tasks.sites.sslMonitor.interval")*time.Second, runTask)
}

func init() {
	store.Store("ssl-monitor", bindTask)
}
