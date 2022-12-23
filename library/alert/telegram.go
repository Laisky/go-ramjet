package alert

import (
	"context"

	"github.com/Laisky/go-ramjet/library/log"

	"github.com/Laisky/errors"
	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/graphql"
	"github.com/Laisky/zap"
)

var (
	Telegram = new(TelegramCli)
)

type TelegramCli struct {
	url string
	cli *graphql.Client
}

func (t *TelegramCli) Setup() {
	t.url = gconfig.Shared.GetString("telegram.api")
	t.cli = graphql.NewClient(t.url, nil)
	log.Logger.Info("setup url", zap.String("url", t.url))
}

type alertMutation struct {
	TelegramMonitorAlert struct {
		Name graphql.String
	} `graphql:"TelegramMonitorAlert(type: $type, token: $token, msg: $msg)"`
}

func (t *TelegramCli) Send(ctx context.Context, alertType, token, msg string) (err error) {
	query := new(alertMutation)
	vars := map[string]interface{}{
		"type":  graphql.String(alertType),
		"token": graphql.String(token),
		"msg":   graphql.String(msg),
	}
	if err = t.cli.Mutate(ctx, query, vars); err != nil {
		return errors.Wrap(err, "send mutation")
	}

	log.Logger.Info("send telegram msg", zap.String("alert", alertType), zap.String("msg", msg))
	return nil
}

func (t *TelegramCli) SendAlert(msg string) (err error) {
	alertType := gconfig.Shared.GetString("telegram.alert")
	pushToken := gconfig.Shared.GetString("telegram.push_token")
	return t.Send(context.Background(), alertType, pushToken, msg)
}
