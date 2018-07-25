package ramjet

import (
	utils "github.com/Laisky/go-utils"
	"github.com/pkg/errors"
)

var (
	Email = &EmailType{}
)

type EmailType struct {
	sender *utils.Mail
}

func (e *EmailType) Setup() {
	e.sender = utils.NewMail(utils.Settings.GetString("email.host"), utils.Settings.GetInt("email.port"))
	e.sender.Login(utils.Settings.GetString("email.username"), utils.Settings.GetString("email.password"))
}

func (e *EmailType) Send(to, toName, subject, content string) (err error) {
	utils.Logger.Infof("send email `%v` to `%v`", subject, to)
	err = e.sender.Send(
		utils.Settings.GetString("email.sender"),
		to,
		utils.Settings.GetString("email.senderName"),
		toName,
		subject,
		content,
	)
	if err != nil {
		return errors.Wrap(err, "go-ramjet try to send email got error")
	}

	return nil
}
