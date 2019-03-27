package ramjet

import (
	"github.com/kataras/iris"
	"github.com/kataras/iris/middleware/pprof"
)

var (
	Server = iris.New()
)

func RunServer(addr string) {
	Server.Get("/health", func(ctx iris.Context) {
		ctx.Write([]byte("Hello, World"))
	})

	Server.Any("/admin/pprof/{action:path}", pprof.New())

	Server.Run(iris.Addr(addr))
}
