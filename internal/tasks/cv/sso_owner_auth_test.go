package cv

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/laisky-blog-graphql/library/auth"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const (
	testOwnerUID    = "0199b2a4-0000-7000-8000-00000000c0de"
	testVisitorUID  = "0199b2a4-0000-7000-8000-0000000f0e1d"
	testInactiveUID = "0199b2a4-0000-7000-8000-0000000dead0"
)

// fakeSSO is a stand-in for the SSO GraphQL endpoint. It verifies EdDSA tokens
// exactly as the manual specifies (signature, issuer, expiry, sub == uid) and
// answers WhoAmI, so the CV middleware is exercised against the real contract.
type fakeSSO struct {
	server   *httptest.Server
	public   ed25519.PublicKey
	private  ed25519.PrivateKey
	calls    atomic.Int32
	mu       sync.Mutex
	mode     string
	inactive map[string]bool
	seenAuth []string
}

// newFakeSSO starts the fake SSO endpoint.
func newFakeSSO(t *testing.T) *fakeSSO {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	sso := &fakeSSO{public: public, private: private, inactive: map[string]bool{testInactiveUID: true}}
	sso.server = httptest.NewServer(http.HandlerFunc(sso.serve))
	t.Cleanup(sso.server.Close)
	return sso
}

// setMode switches the fake's failure behavior.
func (s *fakeSSO) setMode(mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = mode
}

// endpoint returns the GraphQL URL.
func (s *fakeSSO) endpoint() string { return s.server.URL + "/query" }

// serve implements the subset of the SSO GraphQL API that CV relies on.
func (s *fakeSSO) serve(w http.ResponseWriter, r *http.Request) {
	s.calls.Add(1)
	s.mu.Lock()
	mode := s.mode
	s.seenAuth = append(s.seenAuth, r.Header.Get("Authorization"))
	s.mu.Unlock()

	switch mode {
	case "status500":
		http.Error(w, "boom", http.StatusInternalServerError)
		return
	case "malformed":
		_, _ = w.Write([]byte("{not json"))
		return
	case "slow":
		// Answer correctly, but only after the configured 1s verify timeout.
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
			return
		}
	case "redirect":
		http.Redirect(w, r, s.server.URL+"/elsewhere", http.StatusTemporaryRedirect)
		return
	}

	if mode == "huge" {
		// A well-formed owner answer padded past the response bound: only the
		// bound itself can reject it.
		_, _ = w.Write([]byte(`{"data":{"WhoAmI":{"id":"` + testOwnerUID + `","username":"` + strings.Repeat("a", 2<<20) + `"}}}`))
		return
	}

	if r.Method != http.MethodPost || r.URL.Path != "/query" {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !strings.Contains(body.Query, "WhoAmI") {
		writeGraphQLError(w, "unexpected query")
		return
	}
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		writeGraphQLError(w, "token subject is empty")
		return
	}
	claims, err := s.verify(token)
	if err != nil {
		writeGraphQLError(w, "parse asymmetric sso token: "+err.Error())
		return
	}
	if s.inactive[claims.UID] {
		writeGraphQLError(w, "load user by uid: invalid credentials")
		return
	}
	id := claims.UID
	switch mode {
	case "wrong_identity":
		id = testVisitorUID
	case "bad_shape":
		id = "not-a-uuid"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"WhoAmI": map[string]any{"id": id, "username": claims.Username}}})
}

// writeGraphQLError writes a gqlgen-shaped error response.
func writeGraphQLError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"errors": []map[string]any{{"message": message, "path": []string{"WhoAmI"}}},
		"data":   nil,
	})
}

// ssoTestClaims is the SSO JWT payload.
type ssoTestClaims struct {
	Issuer   string `json:"iss"`
	Subject  string `json:"sub"`
	UID      string `json:"uid"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp"`
	ID       string `json:"jti"`
	Username string `json:"username"`
}

// sign issues an EdDSA SSO token like laisky-blog-graphql does.
func (s *fakeSSO) sign(t *testing.T, claims ssoTestClaims) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"EdDSA","typ":"JWT"}`))
	payloadJSON, err := json.Marshal(claims)
	require.NoError(t, err)
	signingInput := header + "." + base64.RawURLEncoding.EncodeToString(payloadJSON)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(s.private, []byte(signingInput)))
}

// token issues a currently valid token for uid.
func (s *fakeSSO) token(t *testing.T, uid string) string {
	t.Helper()
	now := time.Now().UTC()
	return s.sign(t, ssoTestClaims{
		Issuer: "laisky-sso", Subject: uid, UID: uid, IssuedAt: now.Unix(),
		Expires: now.Add(time.Hour).Unix(), ID: uid, Username: uid + "@example.test",
	})
}

// verify checks a token the way the SSO server does.
func (s *fakeSSO) verify(token string) (*ssoTestClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errString("token is malformed")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !ed25519.Verify(s.public, []byte(parts[0]+"."+parts[1]), signature) {
		return nil, errString("token signature is invalid")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errString("token payload is malformed")
	}
	var claims ssoTestClaims
	if err = json.Unmarshal(payload, &claims); err != nil {
		return nil, errString("token payload is malformed")
	}
	if claims.Issuer != "laisky-sso" || claims.Expires < time.Now().Unix() {
		return nil, errString("token is expired or has a foreign issuer")
	}
	if claims.Subject == "" || claims.Subject != claims.UID {
		return nil, errString("sso token subject and uid must match")
	}
	return &claims, nil
}

// errString is a minimal error type for the fake.
type errString string

// Error implements error.
func (e errString) Error() string { return string(e) }

// recordingContentStore counts every repository call, so tests can prove a
// rejected request never reached a handler.
type recordingContentStore struct {
	fakeContentStore
	saves    atomic.Int32
	history  atomic.Int32
	versions atomic.Int32
}

// Save records the write and returns the configured payload.
func (r *recordingContentStore) Save(ctx context.Context, content string) (ContentPayload, error) {
	r.saves.Add(1)
	return r.fakeContentStore.Save(ctx, content)
}

// ListHistory records the read and returns the configured history.
func (r *recordingContentStore) ListHistory(ctx context.Context, limit int) ([]ContentHistoryEntry, error) {
	r.history.Add(1)
	return r.fakeContentStore.ListHistory(ctx, limit)
}

// LoadVersion records the read and returns the configured version.
func (r *recordingContentStore) LoadVersion(ctx context.Context, versionID string) (ContentPayload, error) {
	r.versions.Add(1)
	return r.fakeContentStore.LoadVersion(ctx, versionID)
}

// cvAuthHarness mounts the production CV routes and protection on a fresh engine.
type cvAuthHarness struct {
	sso      *fakeSSO
	store    *recordingContentStore
	renderer *trackingPDFRenderer
	engine   *gin.Engine
}

// newCVAuthHarness configures CV for the fake SSO and the given owner UIDs.
func newCVAuthHarness(t *testing.T, owners []string) *cvAuthHarness {
	t.Helper()
	sso := newFakeSSO(t)
	for key, value := range map[string]any{
		"tasks.cv.sso.graphql_endpoint": sso.endpoint(),
		"tasks.cv.sso.owner_uids":       owners,
		"tasks.cv.sso.timeout":          "1s",
		"server.jwt_secret":             "legacy-hs256-secret-for-tests-only-0123456789",
	} {
		previous := gconfig.Shared.Get(key)
		gconfig.Shared.Set(key, value)
		t.Cleanup(func() { gconfig.Shared.Set(key, previous) })
	}
	previousAuth := auth.Instance
	require.NoError(t, auth.Initialize([]byte(gconfig.Shared.GetString("server.jwt_secret"))))
	t.Cleanup(func() { auth.Instance = previousAuth })

	store := &recordingContentStore{fakeContentStore: fakeContentStore{
		payload: ContentPayload{Content: "# CV"},
		history: []ContentHistoryEntry{{VersionID: "v1"}},
		versions: map[string]ContentPayload{
			"v1": {Content: "# old"},
		},
	}}
	renderer := &trackingPDFRenderer{}
	pdfStore, err := NewS3PDFStore(newFakeS3Client(), "bucket", "cv.pdf")
	require.NoError(t, err)
	pdfService, err := NewPDFService(renderer, pdfStore)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerCVRoutes(engine, &handler{store: store, pdfService: pdfService}, cvRouteProtection())
	return &cvAuthHarness{sso: sso, store: store, renderer: renderer, engine: engine}
}

// do performs one request against the CV routes.
func (h *cvAuthHarness) do(t *testing.T, method string, path string, token string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	h.engine.ServeHTTP(recorder, req)
	return recorder
}

// protectedCalls lists every protected CV route with a valid request body.
var protectedCalls = []struct {
	method string
	path   string
	body   string
}{
	{http.MethodGet, "/cv/content/history", ""},
	{http.MethodGet, "/cv/content/version?version_id=v1", ""},
	{http.MethodPut, "/cv/content", `{"content":"# new"}`},
	{http.MethodPost, "/cv/pdf/preview", `{"content":"# tailored"}`},
}

// requireNoSideEffects asserts no protected handler ran.
func (h *cvAuthHarness) requireNoSideEffects(t *testing.T) {
	t.Helper()
	require.Zero(t, h.store.saves.Load(), "a rejected request must not write")
	require.Zero(t, h.store.history.Load(), "a rejected request must not read history")
	require.Zero(t, h.store.versions.Load(), "a rejected request must not read versions")
	require.False(t, h.renderer.called, "a rejected request must not render")
}

// errorCode decodes the machine-readable error code of a rejection.
func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	return payload.Code
}

// TestCVOwnerSessionReachesProtectedRoutes proves a valid EdDSA owner session
// issued by SSO can use every protected CV route.
func TestCVOwnerSessionReachesProtectedRoutes(t *testing.T) {
	h := newCVAuthHarness(t, []string{testOwnerUID})
	token := h.sso.token(t, testOwnerUID)

	for _, call := range protectedCalls {
		recorder := h.do(t, call.method, call.path, token, call.body)
		require.Equal(t, http.StatusOK, recorder.Code, "%s %s: %s", call.method, call.path, recorder.Body.String())
	}
	require.EqualValues(t, 1, h.store.saves.Load())
	require.EqualValues(t, 1, h.store.history.Load())
	require.EqualValues(t, 1, h.store.versions.Load())
	require.True(t, h.renderer.called)

	recorder := h.do(t, http.MethodGet, "/cv/auth/session", token, "")
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var session struct {
		UID   string `json:"uid"`
		Owner bool   `json:"owner"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &session))
	require.Equal(t, testOwnerUID, session.UID)
	require.True(t, session.Owner)

	for _, header := range h.sso.seenAuth {
		require.Equal(t, "Bearer "+token, header, "the bearer token must reach SSO unchanged")
	}
}

// TestCVNonOwnerSessionIsForbidden proves a different valid SSO account can
// neither read protected history nor mutate the CV.
func TestCVNonOwnerSessionIsForbidden(t *testing.T) {
	h := newCVAuthHarness(t, []string{testOwnerUID})
	token := h.sso.token(t, testVisitorUID)

	for _, call := range protectedCalls {
		recorder := h.do(t, call.method, call.path, token, call.body)
		require.Equal(t, http.StatusForbidden, recorder.Code, "%s %s", call.method, call.path)
		require.Equal(t, "cv_owner_required", errorCode(t, recorder))
	}
	recorder := h.do(t, http.MethodGet, "/cv/auth/session", token, "")
	require.Equal(t, http.StatusForbidden, recorder.Code)
	h.requireNoSideEffects(t)
}

// TestCVRejectsInvalidSessionsWithoutSideEffects proves missing, malformed,
// expired, tampered, foreign-issuer and inactive-account tokens are rejected
// before any handler runs, and malformed tokens never reach SSO at all.
func TestCVRejectsInvalidSessionsWithoutSideEffects(t *testing.T) {
	h := newCVAuthHarness(t, []string{testOwnerUID, testInactiveUID})
	now := time.Now().UTC()
	valid := h.sso.token(t, testOwnerUID)
	parts := strings.Split(valid, ".")
	other := newFakeSSO(t)

	cases := []struct {
		name         string
		token        string
		code         string
		reachesSSO   bool
		expectStatus int
	}{
		{name: "missing", token: "", code: "sso_session_missing", expectStatus: http.StatusUnauthorized},
		{name: "malformed", token: "not-a-jwt", code: "sso_session_invalid", expectStatus: http.StatusUnauthorized},
		{name: "oversized", token: strings.Repeat("a", 9000) + ".b.c", code: "sso_session_invalid", expectStatus: http.StatusUnauthorized},
		{name: "expired", token: h.sso.sign(t, ssoTestClaims{Issuer: "laisky-sso", Subject: testOwnerUID, UID: testOwnerUID,
			IssuedAt: now.Add(-2 * time.Hour).Unix(), Expires: now.Add(-time.Hour).Unix()}),
			code: "sso_session_invalid", reachesSSO: true, expectStatus: http.StatusUnauthorized},
		{name: "tampered", token: parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"laisky-sso","sub":"`+testOwnerUID+`","uid":"`+testOwnerUID+`","exp":9999999999}`)) + "." + parts[2],
			code: "sso_session_invalid", reachesSSO: true, expectStatus: http.StatusUnauthorized},
		{name: "foreign signer", token: other.token(t, testOwnerUID), code: "sso_session_invalid", reachesSSO: true, expectStatus: http.StatusUnauthorized},
		{name: "subject uid mismatch", token: h.sso.sign(t, ssoTestClaims{Issuer: "laisky-sso", Subject: testVisitorUID, UID: testOwnerUID,
			IssuedAt: now.Unix(), Expires: now.Add(time.Hour).Unix()}),
			code: "sso_session_invalid", reachesSSO: true, expectStatus: http.StatusUnauthorized},
		{name: "inactive account", token: h.sso.token(t, testInactiveUID), code: "sso_session_invalid", reachesSSO: true, expectStatus: http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := h.sso.calls.Load()
			for _, call := range protectedCalls {
				recorder := h.do(t, call.method, call.path, tc.token, call.body)
				require.Equal(t, tc.expectStatus, recorder.Code, "%s %s: %s", call.method, call.path, recorder.Body.String())
				require.Equal(t, tc.code, errorCode(t, recorder))
				require.NotContains(t, recorder.Body.String(), valid, "errors must never echo bearer tokens")
			}
			if !tc.reachesSSO {
				require.Equal(t, before, h.sso.calls.Load(), "a malformed token must be rejected locally")
			}
			h.requireNoSideEffects(t)
		})
	}
}

// TestCVFailsClosedWhenSSOMisbehaves proves upstream failures never create an
// authenticated session or a write, report availability distinctly from an
// invalid session, and never forward the bearer token through a redirect.
func TestCVFailsClosedWhenSSOMisbehaves(t *testing.T) {
	h := newCVAuthHarness(t, []string{testOwnerUID})
	token := h.sso.token(t, testOwnerUID)

	cases := []struct {
		mode   string
		status int
		code   string
	}{
		{mode: "status500", status: http.StatusServiceUnavailable, code: "sso_unavailable"},
		{mode: "malformed", status: http.StatusServiceUnavailable, code: "sso_unavailable"},
		{mode: "slow", status: http.StatusServiceUnavailable, code: "sso_unavailable"},
		{mode: "huge", status: http.StatusServiceUnavailable, code: "sso_unavailable"},
		{mode: "bad_shape", status: http.StatusServiceUnavailable, code: "sso_unavailable"},
		{mode: "redirect", status: http.StatusServiceUnavailable, code: "sso_unavailable"},
		{mode: "wrong_identity", status: http.StatusUnauthorized, code: "sso_session_invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			h.sso.setMode(tc.mode)
			h.sso.mu.Lock()
			h.sso.seenAuth = nil
			h.sso.mu.Unlock()

			recorder := h.do(t, http.MethodPut, "/cv/content", token, `{"content":"# new"}`)
			require.Equal(t, tc.status, recorder.Code, recorder.Body.String())
			require.Equal(t, tc.code, errorCode(t, recorder))
			require.NotContains(t, recorder.Body.String(), token)
			h.requireNoSideEffects(t)

			if tc.mode == "redirect" {
				h.sso.mu.Lock()
				seen := append([]string(nil), h.sso.seenAuth...)
				h.sso.mu.Unlock()
				require.Len(t, seen, 1, "the redirect must not be followed with the bearer token")
			}
		})
	}
}

// TestCVOwnerNotConfiguredFailsClosed proves a deployment without a configured
// owner refuses every protected request instead of trusting any SSO user.
func TestCVOwnerNotConfiguredFailsClosed(t *testing.T) {
	h := newCVAuthHarness(t, nil)
	token := h.sso.token(t, testOwnerUID)

	for _, call := range protectedCalls {
		recorder := h.do(t, call.method, call.path, token, call.body)
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		require.Equal(t, "cv_owner_not_configured", errorCode(t, recorder))
	}
	require.Zero(t, h.sso.calls.Load())
	h.requireNoSideEffects(t)
}

// TestCVPublicRoutesStayOpen proves public reads need no session.
func TestCVPublicRoutesStayOpen(t *testing.T) {
	h := newCVAuthHarness(t, []string{testOwnerUID})
	recorder := h.do(t, http.MethodGet, "/cv/content", "", "")
	require.Equal(t, http.StatusOK, recorder.Code)
	recorder = h.do(t, http.MethodGet, "/cv/content", "garbage", "")
	require.Equal(t, http.StatusOK, recorder.Code, "a stale token must not break public reads")
	require.Zero(t, h.sso.calls.Load())
}
