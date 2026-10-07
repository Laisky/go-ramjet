package http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v87"

	"github.com/Laisky/go-ramjet/library/log"
)

// paymentTraceKey identifies a request-scoped value in the local payment fixture.
type paymentTraceKey struct{}

// paymentRoundTripper serves Stripe responses without accessing a payment network.
type paymentRoundTripper func(*http.Request) (*http.Response, error)

// RoundTrip executes the test transport for request and returns a local fixture response.
func (transport paymentRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

// TestStableStripePaymentContract verifies Stripe v87 preserves payment parameters, response decoding, and request context without contacting Stripe.
func TestStableStripePaymentContract(t *testing.T) {
	// Stripe's legacy entry point uses shared package state; this test must not run in parallel.
	originalBackend := stripe.GetBackend(stripe.APIBackend)
	originalKey := stripe.Key
	t.Cleanup(func() {
		stripe.SetBackend(stripe.APIBackend, originalBackend)
		stripe.Key = originalKey
	})
	stripe.Key = "sk_test_local_fixture"

	for _, status := range []int{http.StatusOK, http.StatusBadRequest} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var received *http.Request
			backend := stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
				URL:               stripe.String("https://stripe.invalid"),
				MaxNetworkRetries: stripe.Int64(0),
				HTTPClient: &http.Client{Transport: paymentRoundTripper(func(request *http.Request) (*http.Response, error) {
					received = request
					if err := request.ParseForm(); err != nil {
						return nil, errors.Wrap(err, "parse fixture payment form")
					}
					body := `{"id":"pi_fixture","object":"payment_intent","client_secret":"local_client_secret","amount":2000,"currency":"cny"}`
					if status != http.StatusOK {
						body = `{"error":{"type":"invalid_request_error","message":"fixture declined"}}`
					}
					return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
				})},
			})
			stripe.SetBackend(stripe.APIBackend, backend)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			ctx := context.WithValue(t.Context(), paymentTraceKey{}, "request-scope")
			c.Request = httptest.NewRequestWithContext(ctx, http.MethodPost, "/payment", strings.NewReader(`{"items":[{},{}]}`))
			c.Request.Header.Set("Content-Type", "application/json")
			gmw.SetLogger(c, log.Logger)
			PaymentHandler(c)
			require.NotNil(t, received)
			require.Equal(t, "request-scope", received.Context().Value(paymentTraceKey{}))
			require.Equal(t, http.MethodPost, received.Method)
			require.Equal(t, "/v1/payment_intents", received.URL.Path)
			require.Equal(t, "2000", received.PostForm.Get("amount"))
			require.Equal(t, "cny", received.PostForm.Get("currency"))
			require.Equal(t, "true", received.PostForm.Get("automatic_payment_methods[enabled]"))
			require.Equal(t, stripe.APIVersion, received.Header.Get("Stripe-Version"))
			if status != http.StatusOK {
				require.GreaterOrEqual(t, recorder.Code, http.StatusBadRequest)
				require.NotContains(t, recorder.Body.String(), "clientSecret")
				return
			}
			require.Equal(t, http.StatusOK, recorder.Code)
			require.JSONEq(t, `{"clientSecret":"local_client_secret"}`, recorder.Body.String())
		})
	}
}
