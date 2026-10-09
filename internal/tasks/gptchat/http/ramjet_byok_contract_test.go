package http

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// TestRamjetBYOKActualResolverContract checks real gateway headers against the companion Python resolver without network access.
func TestRamjetBYOKActualResolverContract(t *testing.T) {
	resolverPath := os.Getenv("RAMJET_CREDENTIAL_RESOLVER_PATH")
	if resolverPath == "" {
		t.Skip("Set RAMJET_CREDENTIAL_RESOLVER_PATH for local cross-repository qualification.")
	}
	source, err := os.ReadFile(resolverPath)
	require.NoError(t, err)
	t.Logf("Ramjet resolver SHA256: %x", sha256.Sum256(source))
	const key = "sk-SYNTHETIC-ONLY-CROSS-REPOSITORY-KEY"
	const script = `
import importlib.util, json, sys
spec = importlib.util.spec_from_file_location("ramjet_credential_contract", sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
headers = json.load(sys.stdin)
options = module.resolve_request_credentials(headers["authorization"], headers["api_base"])
json.dump(options, sys.stdout)
`
	for _, base := range []string{"http://100.64.0.10:3000/provider", "http://100.64.0.10:3000/provider/v1"} {
		for _, authorization := range []string{key, "Bearer " + key} {
			t.Run(fmt.Sprintf("%s/%s", base, strings.Split(authorization, " ")[0]), func(t *testing.T) {
				byokAuditSetup(t)
				ctx := newAuthContext(key)
				ctx.Request.Header.Set("Authorization", authorization)
				ctx.Request.Header.Set("X-Laisky-Api-Base", base)
				config.Config.RamjetURL = "http://ramjet.test"
				ctx.Request.Method = http.MethodPost
				ctx.Request.URL.Path = "/gptchat/ramjet/gptchat/query/chunks"
				ctx.Request.Body = io.NopCloser(strings.NewReader("{}"))
				recorder := httptest.NewRecorder()
				proxyContext, _ := gin.CreateTestContext(recorder)
				proxyContext.Request = ctx.Request
				calls := 0
				httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
					calls++
					require.Equal(t, "ramjet.test", req.URL.Host)
					captured := map[string]string{"authorization": req.Header.Get("Authorization"), "api_base": req.Header.Get("X-Laisky-Openai-Api-Base")}
					input, err := json.Marshal(captured)
					require.NoError(t, err)
					command := exec.CommandContext(t.Context(), "python3", "-c", script, resolverPath)
					command.Stdin = strings.NewReader(string(input))
					output, err := command.Output()
					require.NoError(t, err)
					var options map[string]string
					require.NoError(t, json.Unmarshal(output, &options))
					require.Equal(t, key, options["api_key"])
					require.Equal(t, "http://100.64.0.10:3000/provider/v1", options["base_url"])
					return byokAuditResponse(req, "{}"), nil
				})}
				RamjetProxyHandler(proxyContext)
				require.Equal(t, http.StatusOK, recorder.Code)
				require.Equal(t, 1, calls)
			})
		}
	}
}
