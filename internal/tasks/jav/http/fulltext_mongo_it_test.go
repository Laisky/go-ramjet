package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/Laisky/go-ramjet/internal/tasks/jav/dto"
	"github.com/Laisky/go-ramjet/internal/tasks/jav/model"
	"github.com/Laisky/go-ramjet/library/db/mongotest"
)

// itSeedWords are the fulltext words seeded for the real-MongoDB test. Each
// one is also the name (response `code`) of the single movie it points to, so
// the handler's response reveals exactly which words MongoDB matched.
var itSeedWords = []string{
	"tokyo", "TOKYO hot", "kyoto", "a.*b", "x(a+)+$", "^X|Y", "x", "y", "xy",
	"[A-Z]", "q", `a\db`, "7", `{"$gt":""}`, "$where", "a #b", "a b",
	strings.Repeat("a", 32) + "!",
}

// seedFulltext inserts one movie per word and one fulltext entry pointing to
// it into a fresh database, and installs that database as the jav DB.
//
// Parameters:
//   - t: test handle; the database is dropped on cleanup.
//   - client: connected driver client.
func seedFulltext(t *testing.T, client *mongo.Client) {
	t.Helper()

	ctx := t.Context()
	dbName := fmt.Sprintf("jav_it_%d", time.Now().UTC().UnixNano())
	db := mongotest.New(client, dbName)
	t.Cleanup(func() {
		require.NoError(t, client.Database(dbName).Drop(context.Background()))
	})

	var movies, words []any
	for _, word := range itSeedWords {
		id := primitive.NewObjectID()
		movies = append(movies, model.Movie{ID: id, Name: word})
		words = append(words, model.Fulltext{Word: word, Movies: []primitive.ObjectID{id}})
	}
	_, err := db.GetCol("movies").InsertMany(ctx, movies)
	require.NoError(t, err)
	_, err = db.GetCol("fulltext").InsertMany(ctx, words)
	require.NoError(t, err)

	model.SetDB(db)
}

// searchCodes calls the Search handler with q and returns the HTTP status and
// the sorted movie codes it answered with.
//
// Parameters:
//   - t: test handle.
//   - q: raw `q` value; it is URL-encoded here.
//
// Returns:
//   - int: response status code.
//   - []string: sorted codes of the returned movies (nil on errors).
func searchCodes(t *testing.T, q string) (int, []string) {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/jav/search?q="+url.QueryEscape(q), nil)
	Search(c)
	if w.Code != http.StatusOK {
		return w.Code, nil
	}

	var movies []*dto.MovieResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &movies))
	codes := make([]string, 0, len(movies))
	for _, m := range movies {
		codes = append(codes, m.Code)
	}
	slices.Sort(codes)
	return w.Code, codes
}

// literalMatches returns the seeded words that contain q case-insensitively,
// i.e. what a literal substring search must return. It uses no regex engine.
//
// Parameters:
//   - q: search query.
//
// Returns:
//   - []string: sorted matching seed words.
func literalMatches(q string) []string {
	matches := []string{}
	for _, word := range itSeedWords {
		if strings.Contains(strings.ToLower(word), strings.ToLower(q)) {
			matches = append(matches, word)
		}
	}
	slices.Sort(matches)
	return matches
}

// TestSearchIsLiteralOnRealMongoDB runs the Search handler against a real
// MongoDB, whose $regex engine is PCRE rather than Go's RE2, and proves the
// escaped query is a literal, case-insensitive substring there too: pattern,
// operator and inline-flag syntax only ever match itself, a catastrophic-
// backtracking pattern returns at once with no match, and bytes MongoDB
// refuses in a regex (NUL, invalid UTF-8; MongoDB 8.0 answers Location51091)
// fail the request instead of widening it.
//
// It is an integration test: start a disposable server, e.g.
// `docker run --rm -p 127.0.0.1:27017:27017 mongo:8.0`, and set
// JAV_MONGO_IT_URI=mongodb://127.0.0.1:27017 to run it.
func TestSearchIsLiteralOnRealMongoDB(t *testing.T) {
	uri := os.Getenv("JAV_MONGO_IT_URI")
	if uri == "" {
		t.Skip("integration test disabled: set JAV_MONGO_IT_URI to run")
	}

	gin.SetMode(gin.TestMode)
	useTestSearchCache(t)
	origDB := model.GetDB()
	t.Cleanup(func() { model.SetDB(origDB) })

	client, err := mongo.Connect(t.Context(), options.Client().ApplyURI(uri).SetTimeout(10*time.Second))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Disconnect(context.Background())) })
	seedFulltext(t, client)

	for _, q := range []string{
		"", "Tokyo", "A B", ".*", "^x|y", "[a-z]", `\d`, "(a+)+$", `{"$gt":""}`,
		"$where", `(?x)a #b`, "#", `\Q.*\E`, "(?i)TOKYO",
	} {
		t.Run("q="+q, func(t *testing.T) {
			status, codes := searchCodes(t, q)
			require.Equal(t, http.StatusOK, status)
			require.Equal(t, literalMatches(q), codes)
		})
	}

	for _, q := range []string{"a\x00b", "\xff"} {
		t.Run(fmt.Sprintf("rejected %q", q), func(t *testing.T) {
			status, _ := searchCodes(t, q)
			require.Equal(t, http.StatusBadRequest, status, "this pattern must be refused, never widened")
		})
	}
}
