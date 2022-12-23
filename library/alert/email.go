package alert

import (
	"github.com/Laisky/go-ramjet/library/log"

	"github.com/Laisky/errors"
	gconfig "github.com/Laisky/go-config/v2"
	emailSDK "github.com/Laisky/go-utils/v3/email"
	"github.com/Laisky/zap"
)

var (
	Email = &EmailType{}
)

type EmailType struct {
	sender emailSDK.Mail
}

func (e *EmailType) Setup() {
	e.sender = emailSDK.NewMail(gconfig.Shared.GetString("email.host"), gconfig.Shared.GetInt("email.port"))
	e.sender.Login(gconfig.Shared.GetString("email.username"), gconfig.Shared.GetString("email.password"))
}

func (e *EmailType) Send(to, toName, subject, content string) (err error) {
	if gconfig.Shared.GetBool("dry") {
		log.Logger.Info("send email",
			zap.String("cnt", content),
			zap.String("subject", subject),
			zap.String("to", to))
		return nil
	}

	err = e.sender.Send(
		gconfig.Shared.GetString("email.sender"),
		to,
		gconfig.Shared.GetString("email.senderName"),
		toName,
		subject,
		content,
	)
	if err != nil {
		return errors.Wrap(err, "go-ramjet try to send email got error")
	}

	log.Logger.Info("successed send email",
		zap.String("subject", subject),
		zap.String("to", to))
	return nil
}
