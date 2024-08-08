// Package http is a http handler package for jav tasks
package http

import (
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/Laisky/go-ramjet/internal/tasks/jav/dto"
	"github.com/Laisky/go-ramjet/internal/tasks/jav/model"
	"github.com/Laisky/go-ramjet/internal/tasks/jav/service"
	"github.com/Laisky/go-ramjet/library/web"
)

// Search is a http handler to search movies
func Search(ctx *gin.Context) {
	query := ctx.Query("q")
	query = strings.TrimSpace(strings.ReplaceAll(query, "-", ""))

	var docus []model.Fulltext
	cur, err := model.GetDB().
		GetCol("fulltext").
		Find(gmw.Ctx(ctx),
			bson.M{"word": bson.M{"$regex": query, "$options": "i"}},
			options.Find().SetLimit(5),
		)
	if web.AbortErr(ctx, errors.Wrap(err, "search fulltext")) {
		return
	}
	err = cur.All(gmw.Ctx(ctx), &docus)
	if web.AbortErr(ctx, errors.Wrap(err, "search fulltext")) {
		return
	}

	var movies []*dto.MovieResponse
	for _, docu := range docus {
		for i := range docu.Movies {
			if i > 20 {
				break
			}

			movie, err := service.GetMovieInfo(gmw.Ctx(ctx), docu.Movies[i])
			if web.AbortErr(ctx, errors.Wrapf(err, "get movie info `%s`", docu.Movies[i].Hex())) {
				return
			}

			movies = append(movies, movie)
		}
	}

	ctx.JSON(200, movies)
}
