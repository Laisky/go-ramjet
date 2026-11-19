// Package http is a http handler package for jav tasks
package http

import (
	"context"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	gset "github.com/deckarep/golang-set/v3"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-ramjet/internal/tasks/jav/dto"
	"github.com/Laisky/go-ramjet/internal/tasks/jav/model"
	"github.com/Laisky/go-ramjet/internal/tasks/jav/service"
	"github.com/Laisky/go-ramjet/library/web"
)

var searchCache = gutils.NewExpCache[[]*dto.MovieResponse](context.Background(), time.Hour)

// wordFilter builds the fulltext filter for the user-supplied search query.
//
// The query is a literal, case-insensitive substring: regexp.QuoteMeta escapes
// every regular-expression metacharacter so a caller cannot inject a pattern
// (e.g. `.*` to dump the collection, or a catastrophic-backtracking pattern to
// stall the database). Go's escaping uses only `\` + punctuation, which
// MongoDB's PCRE engine also reads as literal characters.
//
// Parameters:
//   - query: raw `q` parameter from the request.
//
// Returns:
//   - bson.M: filter matching fulltext documents whose word contains query.
func wordFilter(query string) bson.M {
	return bson.M{"word": bson.M{"$regex": regexp.QuoteMeta(query), "$options": "i"}}
}

// Search is a http handler to search movies
func Search(ctx *gin.Context) {
	query := ctx.Query("q")

	// search cache
	if res, ok := searchCache.Load(query); ok {
		ctx.JSON(200, res)
		return
	}

	var docus []model.Fulltext
	cur, err := model.GetColFulltext().
		Find(gmw.Ctx(ctx),
			wordFilter(query),
			options.Find().SetLimit(30),
		)
	if web.AbortErr(ctx, errors.Wrap(err, "search fulltext")) {
		return
	}
	err = cur.All(gmw.Ctx(ctx), &docus)
	if web.AbortErr(ctx, errors.Wrap(err, "search fulltext")) {
		return
	}

	moviesSet := gset.NewSet[string]()
	var movies []*dto.MovieResponse
	var mutex sync.Mutex
	var pool errgroup.Group
	pool.SetLimit(10)
	for _, docu := range docus {
		pool.Go(func() (err error) {
			for i := range docu.Movies {
				if moviesSet.Contains(docu.Movies[i].Hex()) {
					continue
				}

				movie, err := service.GetMovieInfo(gmw.Ctx(ctx), docu.Movies[i])
				if err != nil {
					return errors.Wrap(err, "get movie info")
				}

				mutex.Lock()
				moviesSet.Add(docu.Movies[i].Hex())
				movies = append(movies, movie)
				if len(movies) > 100 {
					mutex.Unlock()
					return nil
				}
				mutex.Unlock()
			}

			return nil
		})
	}

	err = pool.Wait()
	if web.AbortErr(ctx, errors.Wrap(err, "get movie info")) {
		return
	}

	// update cache
	searchCache.Store(query, movies)

	ctx.JSON(http.StatusOK, movies)
}
