package http

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAzureExternalQuoteUsesConfiguredDeployment verifies that the Azure
// generation route quotes the deployment it actually calls, not the UI model.
func TestAzureExternalQuoteUsesConfiguredDeployment(t *testing.T) {
	f := &imageBillingFixture{}
	gctx := f.install(t)
	user := billedImageUser(true)
	user.ImageUrl = "https://fixture.openai.azure.com/openai/deployments/dall-e-3/images/generations?api-version=2023-12-01-preview"
	price, err := quoteGenerationImage(gctx.Request.Context(), user, "black-forest-labs/flux-dev")
	require.NoError(t, err)
	require.Equal(t, 15500, price.Int())
}

// TestOneAPIImageQuoteDoesNotFollowRedirect ensures only the configured
// price catalog supplies legacy billing quotes.
func TestOneAPIImageQuoteDoesNotFollowRedirect(t *testing.T) {
	f := &imageBillingFixture{}
	gctx := f.install(t)
	called := make(chan struct{}, 1)
	f.override = func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case "/api/models/display":
			http.Redirect(w, r, "/alternate-catalog", http.StatusFound)
		case "/alternate-catalog":
			called <- struct{}{}
			f.writeBody(w, []byte(`{"success":true,"data":{}}`))
		default:
			return false
		}
		return true
	}
	_, err := oneAPIImagePrice(gctx.Request.Context(), "dall-e-3")
	require.ErrorContains(t, err, "HTTP 302")
	require.Empty(t, called)
}
