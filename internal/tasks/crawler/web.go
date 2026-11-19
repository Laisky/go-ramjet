package crawler

import (
	"net/http"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/go-ramjet/library/db/mongosearch"
	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/go-ramjet/library/web"
)

// maxSearchQueryBytes bounds the search query in bytes.
const maxSearchQueryBytes = 500

// registerWeb registers the crawler HTTP routes on the shared web server.
//
// Parameters:
//   - svc: crawler service that answers search requests.
func registerWeb(svc *Service) {
	web.Server.GET("/crawler/search", searchHandler(svc))
}

// searchHandler returns the handler of GET /crawler/search?q=<text>.
//
// Parameters:
//   - svc: crawler service that runs the search.
//
// Returns:
//   - gin.HandlerFunc: handler answering the matching documents as JSON.
func searchHandler(svc *Service) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		q := strings.TrimSpace(ctx.Query("q"))
		if q == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "empty query"})
			return
		}
		if len(q) > maxSearchQueryBytes {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "query too long"})
			return
		}
		// A query MongoDB refuses as a pattern (NUL, invalid UTF-8) is a
		// client error; reaching the database it would be logged at ERROR.
		if err := mongosearch.ValidateQuery(q, maxSearchQueryBytes); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		rets, err := svc.Search(gmw.Ctx(ctx), q)
		if err != nil {
			log.Logger.Error("crawler search failed", zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "search failed"})
			return
		}

		ctx.JSON(http.StatusOK, rets)
	}
}
