package http

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/db"
	"github.com/Laisky/go-ramjet/library/log"
)

// externalBillingTransport serves only explicitly configured in-process responses.
type externalBillingTransport func(*http.Request) (*http.Response, error)

// RoundTrip executes the fixture without contacting any external service.
func (f externalBillingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// externalBillingBody tracks whether the billing response body is closed.
type externalBillingBody struct {
	io.Reader
	closed bool
}

// Close records response cleanup and returns no error.
func (b *externalBillingBody) Close() error {
	b.closed = true
	return nil
}

// setupExternalBilling replaces shared network configuration and restores it at cleanup.
// Tests using it must not run in parallel because the application client is shared.
func setupExternalBilling(t *testing.T, transport externalBillingTransport) context.Context {
	t.Helper()
	oldClient, oldConfig := httpcli, config.Config
	httpcli = &http.Client{Transport: transport}
	config.Config = &config.OpenAI{ExternalBillingAPI: "https://billing.invalid"}
	t.Cleanup(func() {
		httpcli, config.Config = oldClient, oldConfig
	})
	return gmw.SetLogger(t.Context(), log.Logger)
}

// TestExternalBillingUsage verifies optional latency and unchanged cost/authentication.
func TestExternalBillingUsage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		elapsed time.Duration
		legacy  bool
		millis  int64
	}{
		{name: "upfront charge omits unknown latency", legacy: true},
		{name: "positive", elapsed: 1234 * time.Millisecond, millis: 1234},
		{name: "zero is a known duration"},
		{name: "negative is clamped", elapsed: -time.Second},
		{name: "fractional milliseconds", elapsed: 1900 * time.Microsecond, millis: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			body := &externalBillingBody{Reader: strings.NewReader(`{"success":true}`)}
			ctx := setupExternalBilling(t, func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, http.MethodPost, req.Method)
				require.Equal(t, "https://billing.invalid/api/token/consume", req.URL.String())
				require.Equal(t, "fixture-token", req.Header.Get("Authorization"))
				require.Equal(t, "application/json", req.Header.Get("Content-Type"))
				deadline, ok := req.Context().Deadline()
				require.True(t, ok)
				require.LessOrEqual(t, time.Until(deadline), 15*time.Second)
				var payload map[string]json.RawMessage
				require.NoError(t, json.NewDecoder(req.Body).Decode(&payload))
				require.Equal(t, "12500", string(payload["add_used_quota"]))
				require.Equal(t, `"txt2image:flux-dev"`, string(payload["add_reason"]))
				if tc.legacy {
					require.Len(t, payload, 2)
					require.NotContains(t, payload, "elapsed_time_ms")
				} else {
					require.Len(t, payload, 3)
					var millis int64
					require.NoError(t, json.Unmarshal(payload["elapsed_time_ms"], &millis))
					require.Equal(t, tc.millis, millis)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: body, Header: make(http.Header)}, nil
			})
			user := &config.UserConfig{OpenaiToken: "fixture-token"}
			if tc.legacy {
				require.NoError(t, checkUserExternalBilling(ctx, user, db.PriceTxt2ImageFluxDev, "txt2image:flux-dev"))
			} else {
				require.NoError(t, checkUserExternalBillingWithElapsed(ctx, user, db.PriceTxt2ImageFluxDev, "txt2image:flux-dev", tc.elapsed))
			}
			require.Equal(t, 1, calls)
			require.True(t, body.closed)
		})
	}
}

// TestExternalBillingFailure verifies errors propagate without duplicate charges.
func TestExternalBillingFailure(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			body := &externalBillingBody{Reader: strings.NewReader("billing rejected")}
			ctx := setupExternalBilling(t, func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: status, Body: body, Header: make(http.Header)}, nil
			})
			err := checkUserExternalBillingWithElapsed(ctx, &config.UserConfig{}, 12500, "txt2image:flux-dev", time.Second)
			require.ErrorContains(t, err, "billing rejected")
			require.Equal(t, 1, calls)
			require.True(t, body.closed)
		})
	}
}

// TestExternalBillingContext verifies request values and cancellation survive.
func TestExternalBillingContext(t *testing.T) {
	type traceKey struct{}
	calls := 0
	ctx := setupExternalBilling(t, func(req *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "trace-fixture", req.Context().Value(traceKey{}))
		require.ErrorIs(t, req.Context().Err(), context.Canceled)
		return nil, req.Context().Err()
	})
	ctx = context.WithValue(ctx, traceKey{}, "trace-fixture")
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	err := checkUserExternalBilling(ctx, &config.UserConfig{}, 12500, "tts")
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, calls)
}

// TestExternalBillingDeadline verifies billing cannot extend a caller's shorter deadline.
func TestExternalBillingDeadline(t *testing.T) {
	deadline := time.Now().Add(time.Second)
	ctx := setupExternalBilling(t, func(req *http.Request) (*http.Response, error) {
		actual, ok := req.Context().Deadline()
		require.True(t, ok)
		require.Equal(t, deadline, actual)
		return nil, context.DeadlineExceeded
	})
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	err := checkUserExternalBillingWithElapsed(ctx, &config.UserConfig{}, 12500, "tts", time.Second)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestFluxGenerationBilling verifies the real production helper charges only after
// successful generation, includes its duration, and does not expose unbilled images.
func TestFluxGenerationBilling(t *testing.T) {
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	for _, tc := range []struct {
		name            string
		inpaint         bool
		generationFails bool
		billingFails    bool
	}{
		{name: "text to image"},
		{name: "inpainting", inpaint: true},
		{name: "generation fails", generationFails: true},
		{name: "billing fails", billingFails: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests []string
			ctx := setupExternalBilling(t, func(req *http.Request) (*http.Response, error) {
				requests = append(requests, req.URL.Host)
				status, body := http.StatusOK, ""
				switch req.URL.Host {
				case "api.replicate.com":
					time.Sleep(5 * time.Millisecond)
					status = http.StatusCreated
					body = `{"urls":{"get":"https://replicate.invalid/predictions/test"}}`
					if tc.generationFails {
						status, body = http.StatusBadRequest, "generation rejected"
					}
				case "replicate.invalid":
					body = `{"status":"succeeded","output":["https://image.invalid/result.png"]}`
				case "image.invalid":
					body = encoded.String()
				case "billing.invalid":
					require.Equal(t, []string{"api.replicate.com", "replicate.invalid", "image.invalid", "billing.invalid"}, requests)
					var usage externalBillingUsage
					require.NoError(t, json.NewDecoder(req.Body).Decode(&usage))
					require.Equal(t, db.PriceTxt2ImageFluxDev, usage.Cost)
					require.Equal(t, "txt2image:flux-dev", usage.Reason)
					require.NotNil(t, usage.ElapsedTimeMS)
					require.GreaterOrEqual(t, *usage.ElapsedTimeMS, int64(5))
					body = `{"success":true}`
					if tc.billingFails {
						status, body = http.StatusForbidden, "billing rejected"
					}
				default:
					return nil, errors.Errorf("unexpected fixture host %s", req.URL.Host)
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})
			var request any = &DrawImageByFluxReplicateRequest{}
			if tc.inpaint {
				request = &InpaintingImageByFlusReplicateRequest{}
			}
			img, err := generateAndBillFluxImage(ctx, &config.UserConfig{}, "flux-dev", db.PriceTxt2ImageFluxDev, request)
			switch {
			case tc.generationFails:
				require.ErrorContains(t, err, "generation rejected")
				require.Nil(t, img)
				require.Equal(t, []string{"api.replicate.com"}, requests)
			case tc.billingFails:
				require.ErrorContains(t, err, "billing rejected")
				require.Nil(t, img)
				require.Len(t, requests, 4)
			default:
				require.NoError(t, err)
				_, err = png.Decode(bytes.NewReader(img))
				require.NoError(t, err)
				require.Len(t, requests, 4)
			}
		})
	}
}

// TestFluxUnsupportedRequest verifies malformed internal requests are never billed.
func TestFluxUnsupportedRequest(t *testing.T) {
	ctx := setupExternalBilling(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("unsupported requests must not call upstream or billing")
		return nil, errors.New("unexpected request")
	})
	img, err := generateAndBillFluxImage(ctx, &config.UserConfig{}, "flux-dev", 12500, struct{}{})
	require.ErrorContains(t, err, "unknown request type")
	require.Nil(t, img)
}
