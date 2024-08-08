package arweave

import (
	"github.com/Laisky/laisky-blog-graphql/library/auth"

	"github.com/Laisky/go-ramjet/internal/tasks/arweave/ario"
	"github.com/Laisky/go-ramjet/internal/tasks/arweave/ario/dns"
	"github.com/Laisky/go-ramjet/library/web"
)

func bindHTTP() {
	grp := web.Server.Group("/arweave")
	grp.Any("/gateway/*fileKey", ario.GatewayHandler)
	grp.POST("/dns", auth.AuthMw, dns.CreateRecord)
	grp.PUT("/dns", auth.AuthMw, dns.CreateRecord)
	grp.GET("/dns", dns.ListReocrds)
	grp.GET("/dns/:name", dns.GetRecord)
	grp.GET("/alias/:name", dns.Query)
}
