// Package alert implements alert.
package alert

var Manager = new(ManagerType)

type ManagerType struct {
	emailCli    *EmailType
	telegramCli *TelegramCli
}

func (m *ManagerType) Setup() {
	m.emailCli = Email
	m.emailCli.Setup()

	m.telegramCli = Telegram
	m.telegramCli.Setup()
}

func (m *ManagerType) Send(to, toName, subject, content string) (err error) {
	// return m.emailCli.Send(to, toName, subject, content)
	return m.telegramCli.SendAlert(subject + "\n" + content)
}
