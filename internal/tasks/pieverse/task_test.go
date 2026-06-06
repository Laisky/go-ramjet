package pieverse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckHealth(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		status  int
		body    string
		wantErr bool
	}{
		{
			name:    "healthy",
			status:  http.StatusOK,
			body:    `{"state":"master","ok":true}`,
			wantErr: false,
		},
		{
			name:    "non-200 status",
			status:  http.StatusServiceUnavailable,
			body:    `{"state":"master"}`,
			wantErr: true,
		},
		{
			name:    "invalid json",
			status:  http.StatusOK,
			body:    `not-a-json`,
			wantErr: true,
		},
		{
			name:    "state not master",
			status:  http.StatusOK,
			body:    `{"state":"follower"}`,
			wantErr: true,
		},
		{
			name:    "state missing",
			status:  http.StatusOK,
			body:    `{"ok":true}`,
			wantErr: true,
		},
		{
			name:    "valid json but wrong shape (array)",
			status:  http.StatusOK,
			body:    `["master"]`,
			wantErr: true,
		},
		{
			name:    "json null",
			status:  http.StatusOK,
			body:    `null`,
			wantErr: true,
		},
		{
			name:    "empty body",
			status:  http.StatusOK,
			body:    ``,
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			err := checkHealth(context.Background(), srv.URL)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %+v", err)
			}
		})
	}
}

func TestExplicitlyEnabled_DefaultOff(t *testing.T) {
	// With no `-t` flag bound and no TASKS env, the task must stay disabled.
	t.Setenv("TASKS", "")
	if explicitlyEnabled() {
		t.Fatal("pieverse_alert must be disabled by default when not selected")
	}
}

func TestExplicitlyEnabled_ViaEnv(t *testing.T) {
	t.Setenv("TASKS", "heartbeat,pieverse_alert")
	if !explicitlyEnabled() {
		t.Fatal("pieverse_alert should be enabled when present in TASKS env")
	}
}
