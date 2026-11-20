package memoryx

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/go-utils/v6/agents/memory"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/internal/tasks/gptchat/config"
	"github.com/Laisky/go-ramjet/library/egress"
)

// memoryLLMService starts a loopback HTTPS model endpoint that counts requests.
// It stands in for an internal service that presents a certificate this server
// trusts, which is the case the memory library's https requirement cannot stop.
func memoryLLMService(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp","output":[]}`)
	}))
	t.Cleanup(server.Close)
	return server
}

// trustServerByDefault routes http.DefaultTransport, which the memory
// library's model client uses, to server only and trusts its certificate.
// Dials to example.com:443, a name the test certificate covers, reach server too.
func trustServerByDefault(t *testing.T, server *httptest.Server) {
	t.Helper()
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	base, ok := server.Client().Transport.(*http.Transport)
	require.True(t, ok)
	http.DefaultTransport = &http.Transport{
		Proxy:           nil,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: base.TLSClientConfig.RootCAs},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if address == "example.com:443" {
				address = server.Listener.Addr().String()
			}
			if address != server.Listener.Addr().String() {
				return nil, errors.Errorf("unexpected dial to %s", address)
			}
			var dialer net.Dialer
			conn, err := dialer.DialContext(ctx, network, address)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return conn, nil
		},
	}
}

// runMemoryTurn builds the engine for user and runs one after-turn, which asks
// the memory model to extract facts. It returns the first error.
func runMemoryTurn(t *testing.T, user *config.UserConfig) error {
	t.Helper()
	engineCacheMu.Lock()
	engineCache = map[string]memory.Engine{}
	engineCacheMu.Unlock()
	t.Cleanup(func() {
		engineCacheMu.Lock()
		engineCache = map[string]memory.Engine{}
		engineCacheMu.Unlock()
	})
	conf := &config.OpenAI{EnableMemory: true, MemoryProject: "gptchat", MemoryStorageMCPURL: "https://mcp.example.com",
		MemoryModel: "synthetic-model", MemoryLLMTimeoutSeconds: 5, MemoryLLMMaxOutputTokens: 64}
	engine, err := getEngine(t.Context(), conf, user, &mockToolCaller{})
	if err != nil {
		return err
	}
	return engine.AfterTurn(t.Context(), memory.AfterTurnInput{
		Project: "gptchat", SessionID: "session", UserID: "user", TurnID: "turn",
		InputItems: []memory.ResponseItem{{Type: "message", Role: "user",
			Content: []memory.ResponseContentPart{{Type: "input_text", Text: "my name is alice and I live in Paris"}}}},
		OutputItems: BuildAssistantOutputItems("Nice to meet you, Alice."),
	})
}

// TestMemoryEngineRefusesTenantSelectedPrivateLLMBase proves the memory model
// call, which carries the caller's key plus stored memory, is not sent to an
// internal host chosen through X-Laisky-Api-Base.
func TestMemoryEngineRefusesTenantSelectedPrivateLLMBase(t *testing.T) {
	var hits atomic.Int32
	internal := memoryLLMService(t, &hits)
	trustServerByDefault(t, internal)
	user := &config.UserConfig{UserName: "byok", OpenaiToken: "sk-SYNTHETIC-ONLY-MEMORY-0123", BYOK: true,
		APIBase: internal.URL, APIBaseUserSelected: true}

	err := runMemoryTurn(t, user)
	require.Zero(t, hits.Load(), "tenant-selected memory model base must not reach an internal address")
	require.Error(t, err)
	require.True(t, egress.IsExpectedRefusal(err), err.Error())
}

// TestMemoryEngineKeepsOperatorPrivateLLMBase leaves a server-configured
// private model base untouched.
func TestMemoryEngineKeepsOperatorPrivateLLMBase(t *testing.T) {
	var hits atomic.Int32
	operator := memoryLLMService(t, &hits)
	trustServerByDefault(t, operator)
	user := &config.UserConfig{UserName: "operator", OpenaiToken: "SYNTHETIC", APIBase: operator.URL}

	require.NoError(t, runMemoryTurn(t, user))
	require.Positive(t, hits.Load(), "the operator model base stays reachable")
}

// TestMemoryEngineAdmitsTenantSelectedPublicLLMBase keeps memory extraction
// working for a caller-selected public model base.
func TestMemoryEngineAdmitsTenantSelectedPublicLLMBase(t *testing.T) {
	var hits atomic.Int32
	provider := memoryLLMService(t, &hits)
	trustServerByDefault(t, provider)
	original := memoryLLMResolver
	t.Cleanup(func() { memoryLLMResolver = original })
	memoryLLMResolver = func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host != "example.com" {
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		}
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	user := &config.UserConfig{UserName: "byok", OpenaiToken: "sk-SYNTHETIC-ONLY-MEMORY-0123", BYOK: true,
		APIBase: "https://example.com", APIBaseUserSelected: true}

	require.NoError(t, runMemoryTurn(t, user))
	require.Positive(t, hits.Load(), "a public caller-selected model base stays reachable")
}

// TestMemoryEngineRebindingStopsBeforeTheRequest pins the assumption that lets
// the memory model base be admitted without pinning: when a later DNS answer
// steers the library's own client to an internal service, the https-only rule
// and certificate verification stop the exchange before any request is sent.
func TestMemoryEngineRebindingStopsBeforeTheRequest(t *testing.T) {
	var tlsHits, plainHits, tlsDials atomic.Int32
	internalTLS := memoryLLMService(t, &tlsHits)
	internalPlain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		plainHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp","output":[]}`)
	}))
	t.Cleanup(internalPlain.Close)

	// Admission sees a public answer for rebind.test ...
	original := memoryLLMResolver
	t.Cleanup(func() { memoryLLMResolver = original })
	memoryLLMResolver = func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host != "rebind.test" {
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		}
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	// ... while the library's client connects to internal services, which trust
	// the same roots but hold no certificate for rebind.test.
	trustServerByDefault(t, internalTLS)
	tlsTransport, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok)
	rebound := tlsTransport.Clone()
	rebound.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		routes := map[string]string{
			"rebind.test:443": internalTLS.Listener.Addr().String(),
			"rebind.test:80":  internalPlain.Listener.Addr().String(),
		}
		target, ok := routes[address]
		if !ok {
			return nil, errors.Errorf("unexpected dial to %s", address)
		}
		if address == "rebind.test:443" {
			tlsDials.Add(1)
		}
		var dialer net.Dialer
		conn, err := dialer.DialContext(ctx, network, target)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return conn, nil
	}
	http.DefaultTransport = rebound

	// A failed extraction over https degrades the turn without an error, while
	// a cleartext base is refused when the engine is built.
	for base, refused := range map[string]bool{"https://rebind.test": false, "http://rebind.test": true} {
		user := &config.UserConfig{UserName: "byok", OpenaiToken: "sk-SYNTHETIC-ONLY-MEMORY-0123", BYOK: true,
			APIBase: base, APIBaseUserSelected: true}
		err := runMemoryTurn(t, user)
		if refused {
			require.Error(t, err, base)
		} else {
			require.NoError(t, err, base)
		}
		require.Zero(t, tlsHits.Load(), "%s: certificate verification must stop a rebound TLS exchange", base)
		require.Zero(t, plainHits.Load(), "%s: the library must refuse a cleartext model base", base)
	}
	require.Positive(t, tlsDials.Load(), "the https extraction must have reached the rebound address")
}
