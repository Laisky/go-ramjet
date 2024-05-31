package arweave

import (
	"github.com/Laisky/go-ramjet/internal/tasks/arweave/ario"
	"github.com/Laisky/go-ramjet/library/web"
)

func bindHTTP() {
	grp := web.Server.Group("/arweave")
	grp.Any("/gateway/*fileKey", ario.GatewayHandler)
}
