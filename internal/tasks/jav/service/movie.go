// Package service is a service package for jav tasks
package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v5"
	gutils "github.com/Laisky/go-utils/v4"
	"github.com/Laisky/zap"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-ramjet/internal/tasks/jav/dto"
	"github.com/Laisky/go-ramjet/internal/tasks/jav/model"
)

var getMovieInfoCache = gutils.NewExpCache[(*dto.MovieResponse)](context.Background(), time.Hour)

// GetMovieInfo is a service to get movie info
func GetMovieInfo(ctx context.Context, movieID primitive.ObjectID) (*dto.MovieResponse, error) {
	logger := gmw.GetLogger(ctx)

	// search cache
	if res, ok := getMovieInfoCache.Load(movieID.Hex()); ok {
		return res, nil
	}

	// get movie from db
	movie := new(model.Movie)
	err := model.GetColMovie().
		FindOne(ctx, bson.M{"_id": movieID}).
		Decode(movie)
	if err != nil {
		return nil, errors.Wrapf(err, "get movie info by id %s", movieID.Hex())
	}

	resp := &dto.MovieResponse{
		Code:         movie.Name,
		ImageURLs:    movie.ImgUrls,
		Tags:         movie.Tags,
		Descriptions: movie.Descriptions,
	}

	// get actress from db
	var pool errgroup.Group
	var mu sync.Mutex
	pool.SetLimit(10)
	for _, actressID := range movie.Actresses {
		pool.Go(func() (err error) {
			actress := new(model.Actress)
			err = model.GetColActress().
				FindOne(ctx, bson.M{"_id": actressID}).
				Decode(actress)
			if err != nil {
				return errors.Wrapf(err, "get actress info by id %s", actressID.Hex())
			}

			name := actress.Name
			var uniqueOtherNames []string
			for _, n := range actress.OtherNames {
				if n != name {
					uniqueOtherNames = append(uniqueOtherNames, n)
				}
			}
			if len(uniqueOtherNames) > 0 {
				name = fmt.Sprintf("%s(%s)", name, strings.Join(uniqueOtherNames, ","))
			}

			mu.Lock()
			resp.Actresses = append(resp.Actresses, name)
			mu.Unlock()

			return nil
		})
	}

	err = pool.Wait()
	if err != nil {
		logger.Warn("get actress info", zap.Error(err))
	}

	// generate downloads
	for _, format := range []string{
		"https://16mag.net/search?q=%s",
		"https://www.torrentkitty.red/search/%s/",
		"https://www4.javhdporn.net/video/%s/",
	} {
		resp.Downloads = append(resp.Downloads, fmt.Sprintf(format, movie.Name))
	}

	// update cache
	getMovieInfoCache.Store(movieID.Hex(), resp)

	return resp, nil
}
