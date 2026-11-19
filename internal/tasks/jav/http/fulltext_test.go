package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
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
// MongoDB deployment and returns the `word.$regex` pattern and `$options`
// actually sent to the server.
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

	var cmd struct {
		Filter struct {
			Word struct {
				Regex   string `bson:"$regex"`
				Options string `bson:"$options"`
			} `bson:"word"`
		} `bson:"filter"`
	}
	require.NoError(mt, bson.Unmarshal(started.Command, &cmd))

	return cmd.Filter.Word.Regex, cmd.Filter.Word.Options
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

// TestSearchQueryCannotInjectOperators documents why CodeQL go/sql-injection
// #119 (the `wordFilter(query)` argument, i.e. alert #51 relocated by the
// regexp.QuoteMeta fix) is a false positive. Whatever the caller sends, the
// filter keeps the fixed shape {word: {$regex: <BSON string>, $options: "i"}}:
// Gin's Query("q") only ever yields a Go string (so `q[$ne]=` style keys are
// ignored and JSON-looking values stay text), the field name and options are
// constants, and the pattern is QuoteMeta(q), a literal. (Bytes MongoDB
// refuses in a regex never get this far; see
