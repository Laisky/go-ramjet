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
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/gin-gonic/gin"

	iconfig "github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	itemplates "github.com/Laisky/go-ramjet/internal/tasks/gptchat/templates"
	ijs "github.com/Laisky/go-ramjet/internal/tasks/gptchat/templates/js"
	ipages "github.com/Laisky/go-ramjet/internal/tasks/gptchat/templates/pages"
	ipartials "github.com/Laisky/go-ramjet/internal/tasks/gptchat/templates/partials"
)

var (
	httpcli     *http.Client
	staticFiles struct {
		LibJs, SiteJs *staticFile
	}
)

func init() {
	prepareStaticFiles()
}

func SetupHTTPCli() (err error) {
	httpargs := []gutils.HTTPClientOptFunc{
		gutils.WithHTTPClientTimeout(60 * time.Second),
	}

	if gconfig.Shared.GetString("openai.proxy") != "" {
		httpargs = append(httpargs, gutils.WithHTTPClientProxy(iconfig.Config.Proxy))
	}

	httpcli, err = gutils.NewHTTPClient(httpargs...)
	if err != nil {
		return errors.Wrap(err, "new http client")
	}

	return nil
}

type staticFile struct {
	Name        string
	Content     []byte
	Hash        string
	ContentType string
}

func prepareStaticFiles() {
	staticFiles.LibJs = &staticFile{
		Name:        "libs",
		ContentType: "application/javascript",
		Content:     ijs.Libs,
	}
	staticFiles.SiteJs = &staticFile{
		Name:        "sites",
		ContentType: "application/javascript",
		Content:     append(ijs.Common, ijs.Chat...),
	}

	hasher := sha1.New()
	for _, v := range []*staticFile{
		staticFiles.LibJs,
		staticFiles.SiteJs,
	} {
		hasher.Reset()
		hasher.Write(v.Content)
		v.Hash = fmt.Sprintf("%x", hasher.Sum(nil))[:7]
		v.Name = fmt.Sprintf("%s-%s.js", v.Name, v.Hash)
	}
}

func RegisterStatic(g gin.IRouter) {
	for _, sf := range []*staticFile{
		staticFiles.LibJs,
		staticFiles.SiteJs,
	} {
		sf := sf
		g.GET(fmt.Sprintf("/%s", sf.Name), func(ctx *gin.Context) {
			ctx.Header("Cache-Control", "max-age=86400")
			ctx.Data(http.StatusOK, sf.ContentType, sf.Content)
		})
	}
}

var ts = time.Now().Format(time.RFC3339Nano)

func Chat(ctx *gin.Context) {
	tpl := template.New("mytemplate")
	for name, cnt := range map[string]string{
		"base": itemplates.Base,
		"chat": ipages.Chat,
	} {
		if _, err := tpl.New(name).Parse(cnt); AbortErr(ctx, err) {
			return
		}
	}

	if iconfig.Config.GoogleAnalytics != "" {
		if _, err := tpl.Parse(ipartials.GoogleAnalytics); AbortErr(ctx, err) {
			return
		}
	}

	ctx.Status(http.StatusOK)
	ctx.Header(gutils.HTTPHeaderContentType, "text/html; charset=utf-8")

	injectData := map[string]any{
		"openai": map[string]any{
			"direct": iconfig.Config.API,
			"proxy":  "/api/",
		},
	}
	injectDataPayload, err := gutils.JSON.MarshalToString(injectData)
	if AbortErr(ctx, err) {
		return
	}

	tplArg := struct {
		DataJS string
		BootstrapJs, BootstrapCss,
		SeeJs, ShowdownJs string
		LibJs, SiteJs string
		Version       string
		GaCode        string
	}{
		DataJS:       injectDataPayload,
		BootstrapJs:  iconfig.Config.StaticLibs["bootstrap_js"],
		BootstrapCss: iconfig.Config.StaticLibs["bootstrap_css"],
		SeeJs:        iconfig.Config.StaticLibs["sse_js"],
		ShowdownJs:   iconfig.Config.StaticLibs["showdown_js"],
		LibJs:        staticFiles.LibJs.Name,
		SiteJs:       staticFiles.SiteJs.Name,
		Version:      ts,
		GaCode:       iconfig.Config.GoogleAnalytics,
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
