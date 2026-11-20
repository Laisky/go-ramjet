package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	gutils "github.com/Laisky/go-utils/v6"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"

	"github.com/Laisky/go-ramjet/internal/tasks/jav/dto"
	"github.com/Laisky/go-ramjet/internal/tasks/jav/model"
	"github.com/Laisky/go-ramjet/library/db/mongotest"
)

// useTestSearchCache swaps the package search cache for one whose cleanup
// goroutine stops when t ends, and restores the original afterwards.
//
// Parameters:
//   - t: test handle that owns the cache lifetime.
func useTestSearchCache(t *testing.T) {
	t.Helper()

	orig := searchCache
	searchCache = gutils.NewExpCache[[]*dto.MovieResponse](t.Context(), time.Hour)
	t.Cleanup(func() { searchCache = orig })
}

// capturedWordRegex runs the Search handler for query q against a mock
// MongoDB deployment and returns the pattern and options of the `word`
// regular expression actually sent to the server.
//
// Parameters:
//   - mt: mtest sub-test bound to a mock deployment.
//   - q: raw `q` query parameter.
//
// Returns:
//   - string: the regex pattern sent to MongoDB.
//   - string: the regex options sent to MongoDB.
func capturedWordRegex(mt *mtest.T, q string) (string, string) {
	mt.Helper()

	// Search caches results per query; drop q so repeated runs (-count>1)
	// still reach the database.
	searchCache.Delete(q)
	mt.AddMockResponses(mtest.CreateCursorResponse(0, "jav.fulltext", mtest.FirstBatch))
	model.SetDB(mongotest.New(mt.Client, "jav"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/jav/search?q="+url.QueryEscape(q), nil)
	Search(c)
	require.Equal(mt, http.StatusOK, w.Code, w.Body.String())

	started := mt.GetStartedEvent()
	require.NotNil(mt, started)
	require.Equal(mt, "find", started.CommandName)

	word := started.Command.Lookup("filter", "word")
	pattern, opts, ok := word.RegexOK()
	require.True(mt, ok, "word must be a BSON regular expression, got %s", word.Type)

	return pattern, opts
}

// TestSearchTreatsQueryAsLiteral proves the `q` parameter can no longer inject
// regular-expression syntax into the MongoDB `$regex` filter (CodeQL
// go/sql-injection #51): metacharacters must match only themselves, so a
// caller cannot stall the server with a catastrophic-backtracking pattern or
// change what matches with wildcards, anchors, classes or alternation.
func TestSearchTreatsQueryAsLiteral(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useTestSearchCache(t)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))

	for _, tc := range []struct {
		name     string
		query    string
		mustHit  []string
		mustMiss []string
	}{
		{
			name:     "match-all wildcard",
			query:    ".*",
			mustHit:  []string{"a.*b"},
			mustMiss: []string{"tokyo", ""},
		},
		{
			name:     "catastrophic backtracking",
			query:    "(a+)+$",
			mustHit:  []string{"x(a+)+$"},
			mustMiss: []string{"aaaaaaaaaaaaaaaaaaaa"},
		},
		{
			name:     "anchors and alternation",
			query:    "^x|y",
			mustHit:  []string{"^X|Y"},
			mustMiss: []string{"x", "y", "xy"},
		},
		{
			name:     "character class",
			query:    "[a-z]",
			mustHit:  []string{"[A-Z]"},
			mustMiss: []string{"q"},
		},
		{
			name:     "escape sequence",
			query:    `\d`,
			mustHit:  []string{`a\db`},
			mustMiss: []string{"7"},
		},
		{
			name:     "plain word keeps case-insensitive substring search",
			query:    "Tokyo",
			mustHit:  []string{"tokyo", "TOKYO hot", "in-tokyo"},
			mustMiss: []string{"kyoto"},
		},
	} {
		mt.Run(tc.name, func(mt *mtest.T) {
			pattern, opts := capturedWordRegex(mt, tc.query)
			require.Equal(mt, "i", opts, "search must stay case-insensitive")

			// MongoDB evaluates $regex with PCRE; for these ASCII patterns
			// Go's RE2 is an equivalent oracle of what the server matches.
			re, err := regexp.Compile("(?i)" + pattern)
			require.NoError(mt, err)
			for _, word := range tc.mustHit {
				require.Truef(mt, re.MatchString(word), "pattern %q must match %q", pattern, word)
			}
			for _, word := range tc.mustMiss {
				require.Falsef(mt, re.MatchString(word), "pattern %q must not match %q", pattern, word)
			}
		})
	}
}

// capturedWordFilter runs the Search handler for the raw URL query rawQuery
// against a mock MongoDB deployment and returns the `filter` document exactly
// as it went over the wire.
//
// Parameters:
//   - mt: mtest sub-test bound to a mock deployment.
//   - rawQuery: raw, already URL-encoded query string (without "?").
//
// Returns:
//   - bson.Raw: the find command's filter document.
func capturedWordFilter(mt *mtest.T, rawQuery string) bson.Raw {
	mt.Helper()

	useTestSearchCache(mt.T)
	mt.AddMockResponses(mtest.CreateCursorResponse(0, "jav.fulltext", mtest.FirstBatch))
	model.SetDB(mongotest.New(mt.Client, "jav"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/jav/search?"+rawQuery, nil)
	Search(c)
	require.Equal(mt, http.StatusOK, w.Code, w.Body.String())

	started := mt.GetStartedEvent()
	require.NotNil(mt, started)
	require.Equal(mt, "find", started.CommandName)
	filter, ok := started.Command.Lookup("filter").DocumentOK()
	require.True(mt, ok, "find must carry a filter document")
	return filter
}

// TestSearchQueryCannotInjectOperators proves `q` can neither inject a query
// operator nor regular-expression syntax (CodeQL go/sql-injection #51, and
// #119 where the first fix moved it). Whatever the caller sends, the filter
// keeps the fixed shape {word: <BSON regex QuoteMeta(q), options "i">}: Gin's
// Query("q") only ever yields a Go string (so `q[$ne]=` style keys are ignored
// and JSON-looking values stay text), the field name and options are
// constants, and the value is a typed BSON regular expression, which MongoDB
// matches as a pattern and can never read as an operator document. (Bytes
// MongoDB refuses in a regex never get this far; see
// TestSearchRejectsQueriesMongoDBRefuses.)
func TestSearchQueryCannotInjectOperators(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))

	for _, tc := range []struct {
		name     string
		rawQuery string
		wantQ    string
	}{
		{name: "bracket operator", rawQuery: "q[$ne]=x", wantQ: ""},
		{name: "bracket where", rawQuery: "q[$where]=sleep(5000)", wantQ: ""},
		{name: "json operator document", rawQuery: "q=" + url.QueryEscape(`{"$gt":""}`), wantQ: `{"$gt":""}`},
		{name: "json ne null", rawQuery: "q=" + url.QueryEscape(`{"$ne":null}`), wantQ: `{"$ne":null}`},
		{name: "where literal", rawQuery: "q=" + url.QueryEscape(`$where`), wantQ: `$where`},
		{name: "repeated parameter keeps the first", rawQuery: "q=tokyo&q=" + url.QueryEscape(`$expr`), wantQ: "tokyo"},
		{name: "inline regex flags", rawQuery: "q=" + url.QueryEscape(`(?x)a #b`), wantQ: `(?x)a #b`},
		{name: "pcre quoting", rawQuery: "q=" + url.QueryEscape(`\Q.*\E`), wantQ: `\Q.*\E`},
	} {
		mt.Run(tc.name, func(mt *mtest.T) {
			filter := capturedWordFilter(mt, tc.rawQuery)

			elems, err := filter.Elements()
			require.NoError(mt, err)
			require.Len(mt, elems, 1, "filter must only constrain word: %s", filter)
			require.Equal(mt, "word", elems[0].Key())

			pattern, opts, ok := elems[0].Value().RegexOK()
			require.True(mt, ok, "word must be a typed BSON regular expression, got %s: %s",
				elems[0].Value().Type, filter)
			require.Equal(mt, regexp.QuoteMeta(tc.wantQ), pattern)
			require.Equal(mt, "i", opts)
		})
	}
}

// TestSearchRejectsQueriesMongoDBRefuses verifies queries that MongoDB's regex
// engine refuses (a NUL byte, invalid UTF-8, or a pattern over its ~32 KiB
// limit, all observed on MongoDB 8.0 by TestSearchIsLiteralOnRealMongoDB and
// manual probes) are answered with 400 before any database round trip.
// Before, each one reached MongoDB, failed there, and went through
// web.AbortErr, which logs at ERROR: the level the alert pusher forwards. Any
// anonymous caller could therefore raise alerts at will and use up the shared
// alert rate limit, and the response echoed the server's error text. Queries
// up to the bound, which already exceeds any plausible word, still search.
func TestSearchRejectsQueriesMongoDBRefuses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))

	for _, tc := range []struct {
		name string
		q    string
	}{
		{name: "nul byte", q: "a\x00b"},
		{name: "invalid utf-8", q: "\xff"},
		{name: "over the length bound", q: strings.Repeat("a", maxSearchQueryBytes+1)},
		{name: "multi-byte runes over the length bound", q: strings.Repeat("東", maxSearchQueryBytes/3+1)},
	} {
		mt.Run(tc.name, func(mt *mtest.T) {
			useTestSearchCache(mt.T)
			// The mock would answer an (unexpected) find with an empty result.
			mt.AddMockResponses(mtest.CreateCursorResponse(0, "jav.fulltext", mtest.FirstBatch))
			model.SetDB(mongotest.New(mt.Client, "jav"))

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/jav/search?q="+url.QueryEscape(tc.q), nil)
			Search(c)

			require.Equal(mt, http.StatusBadRequest, w.Code, w.Body.String())
			require.True(mt, mt.GetStartedEvent() == nil, "a refused query must not reach MongoDB")
		})
	}

	mt.Run("query at the length bound still searches", func(mt *mtest.T) {
		q := strings.Repeat("a", maxSearchQueryBytes)
		filter := capturedWordFilter(mt, "q="+url.QueryEscape(q))
		pattern, _, ok := filter.Lookup("word").RegexOK()
		require.True(mt, ok)
		require.Equal(mt, q, pattern)
	})
}
