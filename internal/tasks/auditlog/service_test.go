package auditlog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"

	"github.com/Laisky/go-ramjet/library/db/mongotest"
	"github.com/Laisky/go-ramjet/library/log"
)

// injectionPayloads are classic MongoDB operator-injection strings. Each one
// would be dangerous only if it reached the server as an operator document or
// inside `$where`; the tests prove it is sent as a plain string instead.
var injectionPayloads = []string{
	`{"$ne": null}`,
	`{"$gt": ""}`,
	`$where`,
	`$ne`,
	`'; return true; var x='`,
	`prod" || "1"=="1`,
}

// newMockService returns a service whose collections resolve from the mock
// deployment bound to mt.
//
// Parameters:
//   - mt: mtest sub-test bound to a mock deployment.
//
// Returns:
//   - *service: service under test.
func newMockService(mt *mtest.T) *service {
	mt.Helper()

	return &service{
		logger: log.Logger,
		db: &AuditDB{
			db:               mongotest.New(mt.Client, "auditlog"),
			dbName:           "auditlog",
			logColName:       "logs",
			normalLogColName: "normal-logs",
			taskColName:      "tasks",
		},
	}
}

// capturedFilter returns the `filter` document of the next find command the
// mock deployment received.
//
// Parameters:
//   - mt: mtest sub-test bound to a mock deployment.
//
// Returns:
//   - bson.Raw: the filter exactly as it went over the wire.
func capturedFilter(mt *mtest.T) bson.Raw {
	mt.Helper()

	started := mt.GetStartedEvent()
	require.NotNil(mt, started)
	require.Equal(mt, "find", started.CommandName)

	filter, ok := started.Command.Lookup("filter").DocumentOK()
	require.True(mt, ok, "find must carry a filter document")
	return filter
}

// requireDeployEnvStringEquality asserts filter is exactly {deploy_env: want}
// with want encoded as a BSON string, i.e. a plain equality match that cannot
// carry a query operator.
//
// Parameters:
//   - mt: mtest sub-test.
//   - filter: wire filter document.
//   - want: expected deploy_env value.
func requireDeployEnvStringEquality(mt *mtest.T, filter bson.Raw, want string) {
	mt.Helper()

	elems, err := filter.Elements()
	require.NoError(mt, err)
	require.Len(mt, elems, 1, "filter must contain only deploy_env: %s", filter)
	require.Equal(mt, "deploy_env", elems[0].Key())

	got, ok := elems[0].Value().StringValueOK()
	require.True(mt, ok, "deploy_env must be a BSON string, got %s", elems[0].Value().Type)
	require.Equal(mt, want, got)
}

// TestListLogsDeployEnvIsLiteral documents why CodeQL go/sql-injection #32
// (ListLogs) and #33 (ListNormalLogs) are false positives: the user-provided
// `env` query parameter is a Go string, so the driver encodes it as a BSON
// string under a fixed key. MongoDB treats it as an equality match, never as
// an operator, and no `$where`/aggregation expression is involved.
func TestListLogsDeployEnvIsLiteral(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))

	for _, payload := range injectionPayloads {
		mt.Run("logs "+payload, func(mt *mtest.T) {
			mt.AddMockResponses(mtest.CreateCursorResponse(0, "auditlog.logs", mtest.FirstBatch))

			_, err := newMockService(mt).ListLogs(context.Background(), payload)
			require.NoError(mt, err)
			requireDeployEnvStringEquality(mt, capturedFilter(mt), payload)
		})

		mt.Run("normal logs "+payload, func(mt *mtest.T) {
			mt.AddMockResponses(mtest.CreateCursorResponse(0, "auditlog.normal-logs", mtest.FirstBatch))

			_, err := newMockService(mt).ListNormalLogs(context.Background(), payload)
			require.NoError(mt, err)
			requireDeployEnvStringEquality(mt, capturedFilter(mt), payload)
		})
	}
}

// TestListLogsHandlerIgnoresOperatorSyntax drives the real HTTP handlers with
// query strings that some frameworks parse into operator objects
// (`env[$ne]=x`). Gin's Query("env") only ever yields a string, so such keys
// are ignored and the filter is identical to an unfiltered request, while a
// JSON-looking value stays a literal string.
func TestListLogsHandlerIgnoresOperatorSyntax(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))

	for _, tc := range []struct {
		name     string
		rawQuery string
		wantEnv  string
	}{
		{name: "bracket operator", rawQuery: "env[$ne]=prod", wantEnv: ""},
		{name: "bracket operator on gt", rawQuery: "env[$gt]=", wantEnv: ""},
		{name: "json document value", rawQuery: "env=%7B%22%24ne%22%3Anull%7D", wantEnv: `{"$ne":null}`},
		{name: "plain env", rawQuery: "env=prod", wantEnv: "prod"},
	} {
		for _, endpoint := range []struct {
			name    string
			ns      string
			handler func(*router, *gin.Context)
		}{
			{name: "log", ns: "auditlog.logs", handler: (*router).listLogs},
			{name: "normal-log", ns: "auditlog.normal-logs", handler: (*router).listNormalLogs},
		} {
			mt.Run(endpoint.name+" "+tc.name, func(mt *mtest.T) {
				mt.AddMockResponses(mtest.CreateCursorResponse(0, endpoint.ns, mtest.FirstBatch))
				r := &router{logger: log.Logger, svc: newMockService(mt)}

				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodGet, "/auditlog/"+endpoint.name+"?"+tc.rawQuery, nil)
				endpoint.handler(r, c)
				require.Equal(mt, http.StatusOK, w.Code, w.Body.String())

				filter := capturedFilter(mt)
				if tc.wantEnv == "" {
					elems, err := filter.Elements()
					require.NoError(mt, err)
					require.Empty(mt, elems, "operator syntax must not reach the filter: %s", filter)
					return
				}

				requireDeployEnvStringEquality(mt, filter, tc.wantEnv)
			})
		}
	}
}
