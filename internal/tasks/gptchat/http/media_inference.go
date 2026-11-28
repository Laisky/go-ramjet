package http

import (
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// mediaInferenceBase returns user's selected upstream for BYOK or the explicitly
// configured One API endpoint for managed inference. It rejects other managed
// bases rather than sending that base's credential to a different origin.
func mediaInferenceBase(user *config.UserConfig) (string, error) {
	base := strings.TrimRight(user.APIBase, "/")
	if user.BYOK {
		return base, nil
	}
	oneAPI := strings.TrimRight(config.Config.OneAPIBase, "/")
	if oneAPI == "" {
		oneAPI = config.DefaultOneAPIBase
	}
	if base != oneAPI {
		return "", errors.New("managed media inference requires the configured One API endpoint")
	}
	return base, nil
}

// mediaInferenceClient returns the user's egress client for the inference POST.
// Managed calls may not redirect out of their configured One API endpoint.
func mediaInferenceClient(user *config.UserConfig) *http.Client {
	client := upstreamHTTPClient(user)
	if user.BYOK {
		return client
	}
	noRedirect := *client
	noRedirect.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &noRedirect
}
