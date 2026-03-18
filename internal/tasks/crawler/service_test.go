package crawler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHttpGet_BodySizeLimit(t *testing.T) {
	t.Parallel()

	// Create a server that returns a response larger than 10MB
	largeBody := strings.Repeat("A", 11*1024*1024) // 11MB
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, largeBody)
	}))
	defer server.Close()

	result, err := httpGet(t.Context(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Result should be truncated to 10MB
	if len(result) > 10*1024*1024 {
		t.Errorf("response body should be limited to 10MB, got %d bytes", len(result))
	}
}

func TestHttpGet_NormalResponse(t *testing.T) {
	t.Parallel()

	expectedBody := "<html><title>Test</title><body>Hello</body></html>"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, expectedBody)
	}))
	defer server.Close()

	result, err := httpGet(t.Context(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != expectedBody {
		t.Errorf("expected %q, got %q", expectedBody, result)
	}
}
