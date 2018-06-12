package ramjet

import (
	"github.com/kataras/iris"
)

var (
	Server = iris.New()
)

func RunServer(addr string) {
	Server.Get("/", func(ctx iris.Context) {
		ctx.Write([]byte("Hello, World"))
	})

	Server.Run(iris.Addr(addr))
}
