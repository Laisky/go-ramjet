package http

import (
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	itemplates "github.com/Laisky/go-ramjet/internal/tasks/gptchat/templates"
	ijs "github.com/Laisky/go-ramjet/internal/tasks/gptchat/templates/js"
	ipages "github.com/Laisky/go-ramjet/internal/tasks/gptchat/templates/pages"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/gin-gonic/gin"
)

var (
	httpcli *http.Client
)

func SetupHTTPCli() (err error) {
	httpcli, err = gutils.NewHTTPClient(
		gutils.WithHTTPClientTimeout(60*time.Second),
		gutils.WithHTTPClientProxy(gconfig.Shared.GetString("openai.proxy")),
	)
	if err != nil {
		return errors.Wrap(err, "new http client")
	}

	return nil
}

func RegisterStatic(g gin.IRouter) {
	g.GET("/*any", func(ctx *gin.Context) {
		switch ctx.Param("any") {
		case "/chat.js":
			ctx.Data(http.StatusOK, "application/javascript", ijs.Chat)
		case "/common.js":
			ctx.Data(http.StatusOK, "application/javascript", ijs.Common)
		case "/libs.js":
			ctx.Data(http.StatusOK, "application/javascript", ijs.Libs)
		}
	})
}

func Chat(ctx *gin.Context) {
	tpl := template.New("mytemplate")
	for name, cnt := range map[string]string{
		"base": itemplates.Base,
		"chat": ipages.Chat,
	} {
		_, err := tpl.New(name).Parse(cnt)
		if AbortErr(ctx, err) {
			return
		}
	}

	ctx.Status(http.StatusOK)
	ctx.Header(gutils.HTTPHeaderContentType, "text/html; charset=utf-8")

	injectData := map[string]any{
		"openai": map[string]any{
			"direct": gconfig.Shared.GetString("openai.api"),
			"proxy":  "/api/",
		},
	}
	injectDataPayload, err := gutils.JSON.MarshalToString(injectData)
	if AbortErr(ctx, err) {
		return
	}

	arg := struct {
		CurrentModel string
		DataJS       string
	}{
		CurrentModel: "chat",
		DataJS:       injectDataPayload,
	}
	err = tpl.ExecuteTemplate(ctx.Writer, "base", arg)
	if AbortErr(ctx, err) {
		return
	}
}

func CopyHeader(to, from http.Header) {
	for k, v := range from {
		if gutils.Contains([]string{
			"content-length",
		}, strings.ToLower(k)) {
			continue
		}

		to.Set(k, strings.Join(v, ";"))
	}
}
