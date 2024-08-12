// Package http is a http handler package for jav tasks
package http

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	gutils "github.com/Laisky/go-utils/v4"
	gset "github.com/deckarep/golang-set/v2"
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
			bson.M{"word": bson.M{"$regex": query, "$options": "i"}},
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
