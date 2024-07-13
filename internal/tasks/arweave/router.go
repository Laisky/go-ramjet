package arweave

import (
	"github.com/Laisky/go-ramjet/internal/tasks/arweave/ario"
	"github.com/Laisky/go-ramjet/internal/tasks/arweave/ario/dns"
	"github.com/Laisky/go-ramjet/library/web"
)

func bindHTTP() {
	grp := web.Server.Group("/arweave")
	grp.Any("/gateway/*fileKey", ario.GatewayHandler)
	grp.POST("/dns", dns.CreateRecord)
	grp.PUT("/dns", dns.CreateRecord)
	grp.GET("/dns/:name", dns.GetRecord)
}
