package http

import (
	"crypto/sha1"
	"fmt"
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
	httpargs := []gutils.HTTPClientOptFunc{
		gutils.WithHTTPClientTimeout(60 * time.Second),
	}

	if gconfig.Shared.GetString("openai.proxy") != "" {
		httpargs = append(httpargs, gutils.WithHTTPClientProxy(gconfig.Shared.GetString("openai.proxy")))
	}

	httpcli, err = gutils.NewHTTPClient(httpargs...)
	if err != nil {
		return errors.Wrap(err, "new http client")
	}

	return nil
}

func ETAG(cnt []byte) string {
	hasher := sha1.New()
	hasher.Sum(cnt)
	return fmt.Sprintf("%x", hasher.Sum(nil))[:7]
}

func RegisterStatic(g gin.IRouter) {
	chatJsHash := ETAG(ijs.Chat)
	commonJsHash := ETAG(ijs.Common)
	libsJsHash := ETAG(ijs.Libs)

	g.GET("/*any", func(ctx *gin.Context) {
		ctx.Header("Cache-Control", "max-age=86400")

		switch ctx.Param("any") {
		case fmt.Sprintf("/chat-%s.js", chatJsHash):
			ctx.Data(http.StatusOK, "application/javascript", ijs.Chat)
		case fmt.Sprintf("/common-%s.js", commonJsHash):
			ctx.Data(http.StatusOK, "application/javascript", ijs.Common)
		case fmt.Sprintf("/libs-%s.js", libsJsHash):
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

	tplArg := struct {
		CurrentModel string
		DataJS       string
		BootstrapJs, BootstrapCss,
		SeeJs, ShowdownJs string
		LibJsSuffix, CommonJsSuffix, ChatJsSuffix string
	}{
		CurrentModel:   "chat",
		DataJS:         injectDataPayload,
		BootstrapJs:    gconfig.Shared.GetString("openai.static_libs.bootstrap_js"),
		BootstrapCss:   gconfig.Shared.GetString("openai.static_libs.bootstrap_css"),
		SeeJs:          gconfig.Shared.GetString("openai.static_libs.sse_js"),
		ShowdownJs:     gconfig.Shared.GetString("openai.static_libs.showdown_js"),
		LibJsSuffix:    "-" + ETAG(ijs.Libs),
		CommonJsSuffix: "-" + ETAG(ijs.Common),
		ChatJsSuffix:   "-" + ETAG(ijs.Chat),
	}

	tplArg.BootstrapJs = gutils.OptionalVal(&tplArg.BootstrapJs, "https://s3.laisky.com/static/twitter-bootstrap/5.2.3/js/bootstrap.bundle.min.js")
	tplArg.BootstrapCss = gutils.OptionalVal(&tplArg.BootstrapCss, "https://s3.laisky.com/static/twitter-bootstrap/5.2.3/css/bootstrap.min.css")
	tplArg.ShowdownJs = gutils.OptionalVal(&tplArg.ShowdownJs, "https://s3.laisky.com/static/showdown/2.1.0/showdown.min.js")
	tplArg.SeeJs = gutils.OptionalVal(&tplArg.SeeJs, "https://s3.laisky.com/static/sse/0.6.1/sse.js")

	err = tpl.ExecuteTemplate(ctx.Writer, "base", tplArg)
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
