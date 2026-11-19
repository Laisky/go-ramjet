package crawler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	gutils "github.com/Laisky/go-utils/v6"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"

	"github.com/Laisky/go-ramjet/library/db/mongotest"
)

// newMockService returns a crawler service whose documents collection
// resolves from the mock deployment bound to mt.
//
// Parameters:
//   - mt: mtest sub-test bound to a mock deployment.
//
// Returns:
//   - *Service: service under test, with a cache scoped to mt.
func newMockService(mt *mtest.T) *Service {
	mt.Helper()

	return &Service{
		dao: &Dao{DB: &BBT{
			db:           mongotest.New(mt.Client, "bbt"),
			dbName:       "bbt",
			docusColName: "docus",
		}},
		searchCache: gutils.NewExpCache[[]SearchResult](mt.Context(), time.Minute),
	}
}

// TestExtractSearchContextTreatsQueryAsLiteral verifies the highlighted
// context matches the query literally, as the MongoDB filter does
// (regexp.QuoteMeta in Dao.Search). The query used to be compiled as a regular
// expression here: "C++" or "(a" failed to compile, so every result came back
// with an empty context, and "a.c" or "x|" highlighted text the query does not
// contain.
func TestExtractSearchContextTreatsQueryAsLiteral(t *testing.T) {
	t.Parallel()

	dao := &Dao{} // DB not needed for extractSearchContext
	for _, tc := range []struct {
		name, query, text, wantMark string
	}{
		{name: "repetition operators", query: "C++", text: "learn c++ today", wantMark: "<mark>c++</mark>"},
		{name: "unbalanced group", query: "(a", text: "call f(a) now", wantMark: "<mark>(a</mark>"},
		{name: "wildcard", query: "a.c", text: "abc, then a.c", wantMark: "<mark>a.c</mark>"},
		{name: "alternation", query: "x|", text: "say x| here", wantMark: "<mark>x|</mark>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			results := dao.extractSearchContext(tc.query, []SearchResult{{Text: tc.text}})
			require.Len(t, results, 1)
			require.Contains(t, results[0].Context, tc.wantMark)
		})
	}
}

// TestSearchHandlerRejectsQueriesMongoDBRefuses verifies a query MongoDB
// refuses as a regular expression (a NUL byte, which the driver already
// refuses to encode, or invalid UTF-8, which MongoDB 8.0 answers with
// Location51091) is answered with 400 before any database round trip. Before,
// the failure surfaced from the database call and was logged at ERROR, the
// level the alert pusher forwards, so any anonymous caller could raise alerts.
func TestSearchHandlerRejectsQueriesMongoDBRefuses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))

	for _, tc := range []struct{ name, q string }{
		{name: "nul byte", q: "a\x00b"},
		{name: "invalid utf-8", q: "\xff"},
	} {
		mt.Run(tc.name, func(mt *mtest.T) {
			// The mock would answer (unexpected) finds with empty results.
			mt.AddMockResponses(
				mtest.CreateCursorResponse(0, "bbt.docus", mtest.FirstBatch),
				mtest.CreateCursorResponse(0, "bbt.docus", mtest.FirstBatch),
			)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/crawler/search?q="+url.QueryEscape(tc.q), nil)
			searchHandler(newMockService(mt))(c)

			require.Equal(mt, http.StatusBadRequest, w.Code, w.Body.String())
			require.Nil(mt, mt.GetStartedEvent(), "a refused query must not reach MongoDB")
		})
	}

	mt.Run("plain query still searches as a literal", func(mt *mtest.T) {
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, "bbt.docus", mtest.FirstBatch),
			mtest.CreateCursorResponse(0, "bbt.docus", mtest.FirstBatch),
		)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/crawler/search?q="+url.QueryEscape("c++"), nil)
		searchHandler(newMockService(mt))(c)
		require.Equal(mt, http.StatusOK, w.Code, w.Body.String())

		for _, field := range []string{"title", "text"} {
			started := mt.GetStartedEvent()
			require.NotNil(mt, started)
			pattern, opts, ok := started.Command.Lookup("filter", field).RegexOK()
			require.True(mt, ok, "%s must be a typed BSON regular expression", field)
			require.Equal(mt, `c\+\+`, pattern)
			require.Equal(mt, "i", opts)
		}
	})
}
