package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
)

// TestRamjetCapabilityGoldenScopes binds the versioned namespace to fixed UTF-8
// vectors independently of the production derivation.
func TestRamjetCapabilityGoldenScopes(t *testing.T) {
	for _, tc := range []struct{ key, scope string }{
		{"sk-SYNTHETIC-01-A", "ckv2-41f833095f224f66400b6a0aca6a1d2a7487d1cb35965dae10a7de5fa99c2923"},
		{"sk-SYNTHETIC-01-B", "ckv2-ded3fc65bb930e7449e56b55705827ef2a9edb3bdd5c87bfae625da72b2a192c"},
		{"sk-SYNTHETIC-ROTATED-KEY", "ckv2-29b4392cdac940f2830ca0ca5a103d6d80587d0aa31a2b922b20289befd3e433"},
		{"laisky-SYNTHETIC-UTF8-é", "ckv2-fdcf7c2cf55074a2613381bcfda26590616ce1cf144387fcd9380a19c00123e7"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			require.Equal(t, tc.scope, ramjetCredentialScope(tc.key))
			require.NotContains(t, tc.scope, tc.key[:15])
		})
	}
}

// TestRamjetCapabilityResolverDiscardsCachedNames requires complete-key scopes
// while preserving generic user state and provider credentials.
func TestRamjetCapabilityResolverDiscardsCachedNames(t *testing.T) {
	keys := []string{"sk-SYNTHETIC-01-A", "sk-SYNTHETIC-01-B", "sk-SYNTHETIC-ROTATED-KEY"}
	require.Equal(t, keys[0][:15], keys[1][:15])
	scopes := make([]string, len(keys))
	for index, key := range keys {
		t.Run(fmt.Sprintf("key%d", index), func(t *testing.T) {
			byokAuditSetup(t)
			ctx := newAuthContext(key)
			ctx.Request.Header.Set("X-Laisky-User-Id", "forged-victim")
			ctx.Request.Header.Set("X-Laisky-User-Is-Free", "false")
			original := &config.UserConfig{
				BYOK: true, Token: key, OpenaiToken: key, ImageToken: key,
				UserName: "forged-cached-victim", APIBase: "http://provider.fixture",
			}
			ctx.Set(ctxKeyUser, original)
			resolved, err := resolveRamjetUser(ctx)
			require.NoError(t, err)
			require.Equal(t, ramjetCredentialScope(key), resolved.UserName)
			require.Equal(t, "forged-cached-victim", original.UserName)
			require.NotSame(t, original, resolved)
			require.Equal(t, key, resolved.Token)
			require.Equal(t, key, resolved.OpenaiToken)
			require.Equal(t, key, resolved.ImageToken)
			require.Equal(t, original.APIBase, resolved.APIBase)
			require.False(t, resolved.IsFree, "generic BYOK state is not a paid Ramjet grant")
			scopes[index] = resolved.UserName
			again, err := resolveRamjetUser(ctx)
			require.NoError(t, err)
			require.Equal(t, scopes[index], again.UserName)
		})
	}
	require.NotEqual(t, scopes[0], scopes[1])
	require.NotEqual(t, scopes[0], scopes[2], "rotation does not claim the old key's data")
	require.NotEqual(t, scopes[1], scopes[2])
}

// TestRamjetCapabilityProxyHeaders verifies list/delete forwarding rejects header
// ownership claims and never mutates the incoming caller or generic identity.
func TestRamjetCapabilityProxyHeaders(t *testing.T) {
	keys := []string{"sk-SYNTHETIC-01-A", "sk-SYNTHETIC-01-B", "sk-SYNTHETIC-ROTATED-KEY"}
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		for index, key := range keys {
			for _, freeHeader := range []string{"", "true", "false"} {
				t.Run(fmt.Sprintf("%s/key%d/free=%s", method, index, freeHeader), func(t *testing.T) {
					byokAuditSetup(t)
					config.Config.RamjetURL = "http://ramjet.fixture"
					recorder := httptest.NewRecorder()
					ctx, _ := gin.CreateTestContext(recorder)
					body := ""
					if method == http.MethodDelete {
						body = `{"datasets":["synthetic-dataset"]}`
					}
					ctx.Request = httptest.NewRequest(method, "/gptchat/ramjet/gptchat/files", strings.NewReader(body))
					ctx.Request.Header.Set("Authorization", "Bearer "+key)
					ctx.Request.Header.Set("X-Laisky-User-Id", "forged-victim")
					ctx.Request.Header.Set("X-Laisky-Api-Base", "http://provider.fixture/v1")
					if freeHeader != "" {
						ctx.Request.Header.Set("X-Laisky-User-Is-Free", freeHeader)
					}
					incoming := ctx.Request.Header.Clone()
					general, err := getUserByAuthHeader(ctx)
					require.NoError(t, err)
					calls := 0
					httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
						calls++
						require.Equal(t, "ramjet.fixture", req.URL.Host)
						require.Equal(t, "/gptchat/files", req.URL.Path)
						require.Equal(t, method, req.Method)
						require.Equal(t, key, req.Header.Get("Authorization"))
						require.Equal(t, ramjetCredentialScope(key), req.Header.Get("X-Laisky-User-Id"))
						require.Equal(t, "true", req.Header.Get("X-Laisky-User-Is-Free"))
						require.Equal(t, "http://provider.fixture/v1", req.Header.Get("X-Laisky-Openai-Api-Base"))
						payload, err := io.ReadAll(req.Body)
						require.NoError(t, err)
						require.Equal(t, body, string(payload))
						return byokAuditResponse(req, "{}"), nil
					})}
					RamjetProxyHandler(ctx)
					require.Equal(t, http.StatusOK, recorder.Code)
					require.Equal(t, 1, calls)
					require.Equal(t, incoming, ctx.Request.Header)
					require.Equal(t, key[:15], general.UserName)
					require.False(t, general.IsFree)
					require.Equal(t, key, GetRawUserToken(ctx))
				})
			}
		}
	}
}

// TestRamjetCapabilityChunkHeaders applies the same capability contract to the
// URL-enrichment caller, preserving request headers and generic user state.
func TestRamjetCapabilityChunkHeaders(t *testing.T) {
	for index, key := range []string{"sk-SYNTHETIC-01-A", "sk-SYNTHETIC-01-B", "sk-SYNTHETIC-ROTATED-KEY"} {
		t.Run(fmt.Sprintf("key%d", index), func(t *testing.T) {
			byokAuditSetup(t)
			config.Config.RamjetURL = "http://ramjet.fixture"
			ctx := newAuthContext(key)
			ctx.Request.Header.Set("X-Laisky-User-Id", "forged-victim")
			ctx.Request.Header.Set("X-Laisky-User-Is-Free", "false")
			ctx.Request.Header.Set("X-Laisky-Api-Base", "http://provider.fixture")
			incoming := ctx.Request.Header.Clone()
			user, err := getUserByAuthHeader(ctx)
			require.NoError(t, err)
			calls := 0
			httpcli = &http.Client{Transport: byokAuditTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "ramjet.fixture", req.URL.Host)
				require.Equal(t, "/gptchat/query/chunks", req.URL.Path)
				require.Equal(t, key, req.Header.Get("Authorization"))
				require.Equal(t, ramjetCredentialScope(key), req.Header.Get("X-Laisky-User-Id"))
				require.Equal(t, "true", req.Header.Get("X-Laisky-User-Is-Free"))
				require.Equal(t, "http://provider.fixture", req.Header.Get("X-Laisky-Openai-Api-Base"))
				return byokAuditResponse(req, `{"results":"synthetic-only"}`), nil
			})}
			result, err := queryChunks(ctx, queryChunksArgs{
				user: user, query: "synthetic", ext: ".txt", model: "test-model", content: []byte("local"),
			})
			require.NoError(t, err)
			require.Equal(t, "synthetic-only", result)
			require.Equal(t, 1, calls)
			require.Equal(t, incoming, ctx.Request.Header)
			require.Equal(t, key[:15], user.UserName)
			require.False(t, user.IsFree)
		})
	}
}

// TestRamjetCapabilityPythonContract compares actual Go forwarding against the
// companion pure Python namespace resolver without contacting either service.
func TestRamjetCapabilityPythonContract(t *testing.T) {
	namespacePath := os.Getenv("RAMJET_NAMESPACE_RESOLVER_PATH")
	if namespacePath == "" {
		t.Skip("Set RAMJET_NAMESPACE_RESOLVER_PATH for local cross-repository qualification.")
	}
	require.FileExists(t, namespacePath)
	credentialsPath := filepath.Join(filepath.Dir(namespacePath), "credentials.py")
	require.FileExists(t, credentialsPath)
	keyCredentialsPath := filepath.Join(filepath.Dir(namespacePath), "..", "..", "credentials.py")
	require.FileExists(t, keyCredentialsPath)
	const script = `
import importlib.util, json, sys, types
for name in ("ramjet", "ramjet.tasks", "ramjet.tasks.gptchat"):
    package = types.ModuleType(name)
    package.__path__ = []
    sys.modules[name] = package
for name, path in (
    ("ramjet.credentials", sys.argv[1]),
    ("ramjet.tasks.gptchat.credentials", sys.argv[2]),
    ("ramjet.tasks.gptchat.namespace", sys.argv[3]),
):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
module = sys.modules["ramjet.tasks.gptchat.namespace"]
json.dump([module.credential_scope(key) for key in json.load(sys.stdin)], sys.stdout)
`
	keys := []string{"sk-SYNTHETIC-01-A", "sk-SYNTHETIC-01-B", "sk-SYNTHETIC-ROTATED-KEY", "laisky-SYNTHETIC-CUSTOM-KEY"}
	input, err := json.Marshal(keys)
	require.NoError(t, err)
	command := exec.CommandContext(t.Context(), "python3", "-c", script, keyCredentialsPath, credentialsPath, namespacePath)
	command.Stdin = strings.NewReader(string(input))
	output, err := command.Output()
	require.NoError(t, err)
	var scopes []string
	require.NoError(t, json.Unmarshal(output, &scopes))
	require.Len(t, scopes, len(keys))
	for index, key := range keys {
		t.Run(fmt.Sprintf("key%d", index), func(t *testing.T) {
			byokAuditSetup(t)
			ctx := newAuthContext(key)
			req := httptest.NewRequest(http.MethodGet, "http://ramjet.fixture/gptchat/files", nil)
			require.NoError(t, setUserAuth(ctx, req))
			require.Equal(t, scopes[index], req.Header.Get("X-Laisky-User-Id"))
			require.Equal(t, "true", req.Header.Get("X-Laisky-User-Is-Free"))
			require.Equal(t, key, req.Header.Get("Authorization"))
		})
	}
}
