package http

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/db"
)

const maxModelCatalogBytes = 8 << 20

// oneAPIHandlesImageBilling reports whether the OpenAI-compatible image
// request will go to the same HTTPS One API origin as the billing service.
func oneAPIHandlesImageBilling(apiBase, billingBase string) bool {
	a, err := url.Parse(apiBase)
	if err != nil {
		return false
	}
	b, err := url.Parse(billingBase)
	if err != nil {
		return false
	}
	return a.Scheme == "https" && b.Scheme == "https" && a.Host != "" &&
		strings.EqualFold(a.Host, b.Host) && a.User == nil && b.User == nil &&
		a.RawQuery == "" && b.RawQuery == "" && a.Fragment == "" && b.Fragment == "" &&
		strings.TrimRight(a.Path, "/") == strings.TrimRight(b.Path, "/")
}

// quoteGenerationImage resolves the effective model for the legacy Azure
// generation route, which selects the deployment from its configured URL and
// does not send a model from the request body.
func quoteGenerationImage(ctx context.Context, user *config.UserConfig, model string) (db.Price, error) {
	directAzure := strings.Contains(user.ImageUrl, "openai.azure.com")
	if directAzure && user.EnableExternalImageBilling {
		var err error
		model, err = azureImageCatalogID(user.ImageUrl)
		if err != nil {
			return 0, err
		}
	}
	return quoteExternalImage(ctx, user, model, directAzure)
}

// azureImageCatalogID matches the existing Azure deployment URL format. An
// opaque Azure deployment alias is not a One API catalog model ID.
func azureImageCatalogID(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", errors.New("invalid Azure image deployment URL")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 5 || parts[0] != "openai" || parts[1] != "deployments" ||
		parts[2] == "" || parts[3] != "images" || parts[4] != "generations" {
		return "", errors.New("Azure image deployment has no One API catalog model ID")
	}
	return parts[2], nil
}

// quoteExternalImage obtains a single server-side quote for legacy direct
// providers. Native One API image calls are billed by One API itself.
func quoteExternalImage(ctx context.Context, user *config.UserConfig, model string, directProvider bool) (db.Price, error) {
	if !user.EnableExternalImageBilling ||
		(!directProvider && oneAPIHandlesImageBilling(user.APIBase, config.Config.ExternalBillingAPI)) {
		return 0, nil
	}
	return oneAPIImagePrice(ctx, model)
}

// oneAPIImagePrice reads One API's published per-image component. Do not
// substitute a default, use token rates as a fixed price, or guess which
// channel applies when the same model has conflicting prices.
func oneAPIImagePrice(ctx context.Context, model string) (db.Price, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(config.Config.ExternalBillingAPI, "/")+"/api/models/display", nil)
	if err != nil {
		return 0, errors.Wrap(err, "build One API model catalog request")
	}
	// This trusted server-side endpoint is configured locally. Its quote
	// must come from that endpoint, not an intermediate redirect.
	client := *httpcli
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, errors.Wrap(err, "read One API model catalog")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0, errors.Errorf("One API model catalog returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxModelCatalogBytes+1))
	if err != nil {
		return 0, errors.Wrap(err, "read One API model catalog body")
	}
	if len(body) > maxModelCatalogBytes {
		return 0, errors.New("One API model catalog exceeds size limit")
	}
	var catalog struct {
		Success bool
		Data    map[string]struct {
			Models map[string]struct {
				ImagePrice *float64 `json:"image_price"`
			} `json:"models"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &catalog); err != nil {
		return 0, errors.Wrap(err, "decode One API model catalog")
	}
	if !catalog.Success {
		return 0, errors.New("One API model catalog lookup was unsuccessful")
	}
	// Short Flux aliases are persisted by older clients, while the catalog
	// publishes the publisher-qualified ID. No other ID is inferred.
	if strings.HasPrefix(model, "flux-") {
		model = "black-forest-labs/" + model
	}
	var price *float64
	for _, channel := range catalog.Data {
		entry, ok := channel.Models[model]
		if !ok {
			continue
		}
		if entry.ImagePrice == nil || *entry.ImagePrice <= 0 {
			return 0, errors.Errorf("model %q has no fixed One API image price; use One API for native billing", model)
		}
		if price != nil && *price != *entry.ImagePrice {
			return 0, errors.Errorf("model %q has conflicting One API image prices", model)
		}
		price = entry.ImagePrice
	}
	if price == nil {
		return 0, errors.Errorf("model %q has no fixed One API image price; use One API for native billing", model)
	}
	units := math.Round(*price * float64(db.PriceUSD))
	// The batch settlement multiplies this amount by at most the image limit.
	if units < 1 || units >= float64(math.MaxInt/maxImagesPerRequest) {
		return 0, errors.Errorf("model %q has an out-of-range One API image price", model)
	}
	return db.Price(units), nil
}
