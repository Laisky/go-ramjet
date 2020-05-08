package alert

import (
	"context"

	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"

	"github.com/pkg/errors"
	"github.com/shurcooL/graphql"
)

var (
	Telegram = new(TelegramCli)
)

type TelegramCli struct {
	url string
	cli *graphql.Client
}

func (t *TelegramCli) Setup() {
	t.url = utils.Settings.GetString("logger.push_api")
	t.cli = graphql.NewClient(t.url, nil)
	utils.Logger.Info("setup url", zap.String("url", t.url))
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
		return errors.Wrapf(err, "send mutation with query `%+v`, vars `%+v`", query, vars)
	}

	utils.Logger.Info("send telegram msg", zap.String("alert", alertType), zap.String("msg", msg))
	return nil
}

func (t *TelegramCli) SendAlert(msg string) (err error) {
	alertType := utils.Settings.GetString("logger.alert_type")
	pushToken := utils.Settings.GetString("logger.push_token")
	return t.Send(context.Background(), alertType, pushToken, msg)
}
