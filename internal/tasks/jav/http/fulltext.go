// Package http is a http handler package for jav tasks
package http

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

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

// maxSearchQueryBytes bounds the `q` parameter. Search is a substring match,
// so a query longer than every stored word can never match anything; the bound
// sits far above any plausible word yet far below MongoDB's ~32 KiB regex
// limit, and it also caps the size of each searchCache key.
const maxSearchQueryBytes = 512

// wordFilter builds the fulltext filter for the user-supplied search query.
//
// The query is a literal, case-insensitive substring: regexp.QuoteMeta escapes
// every regular-expression metacharacter so a caller cannot inject pattern
// syntax, such as a catastrophic-backtracking pattern that stalls the
// database or anchors/classes/alternation that change what matches. (An empty
// query still matches every word, limited to 30 results, as before.) Go's
// escaping uses only `\` + punctuation, which MongoDB's PCRE engine also reads
// as literal characters.
//
// Parameters:
//   - query: raw `q` parameter from the request.
//
// Returns:
//   - bson.M: filter matching fulltext documents whose word contains query.
func wordFilter(query string) bson.M {
	return bson.M{"word": bson.M{"$regex": regexp.QuoteMeta(query), "$options": "i"}}
}

// validateSearchQuery checks query before it reaches MongoDB.
//
// It rejects only queries MongoDB's regex engine refuses anyway (a NUL byte,
// invalid UTF-8) or that are longer than maxSearchQueryBytes, so they are
// answered as a client error instead of a database failure, which
// web.AbortErr would log at the alerting ERROR level.
//
// Parameters:
//   - query: raw `q` parameter from the request.
//
// Returns:
//   - error: why query cannot be searched, or nil when it is acceptable.
func validateSearchQuery(query string) error {
	switch {
	case len(query) > maxSearchQueryBytes:
		return errors.Errorf("query must be at most %d bytes", maxSearchQueryBytes)
	case !utf8.ValidString(query):
		return errors.New("query must be valid UTF-8")
	case strings.ContainsRune(query, 0):
		return errors.New("query must not contain NUL")
	default:
		return nil
	}
}

// Search is a http handler to search movies
func Search(ctx *gin.Context) {
	query := ctx.Query("q")
	if err := validateSearchQuery(query); err != nil {
		ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"err": err.Error()})
		return
	}

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
