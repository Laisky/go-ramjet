package egress

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/log"
)

// staticResolver returns a resolver that answers every lookup with addresses and err
// and counts its calls.
func staticResolver(calls *int, err error, addresses ...string) Resolver {
	return func(context.Context, string) ([]net.IPAddr, error) {
		*calls++
		if err != nil {
			return nil, err
		}
		answer := make([]net.IPAddr, 0, len(addresses))
		for _, address := range addresses {
			answer = append(answer, net.IPAddr{IP: net.ParseIP(address)})
		}
		return answer, nil
	}
}

// TestAdmitClassifiesRefusals pins every refusal path to one finite reason and
// verifies resolver failures are never reported as destination denials.
func TestAdmitClassifiesRefusals(t *testing.T) {
	timeout := &net.DNSError{Err: "i/o timeout", Name: "example.com", IsTimeout: true}
	for _, tc := range []struct {
		name, target string
		err          error
		addresses    []string
		reason       Reason
		lookups      bool
	}{
		{name: "loopback_literal", target: "http://127.0.0.1:8080/private?token=q", reason: ReasonDestinationDenied},
		{name: "metadata_literal", target: "http://169.254.169.254/latest/meta-data/", reason: ReasonDestinationDenied},
		{name: "ipv6_loopback", target: "http://[::1]/", reason: ReasonDestinationDenied},
		{name: "local_name", target: "http://localhost:1/", reason: ReasonDestinationDenied},
		{name: "internal_suffix", target: "https://metadata.internal/", reason: ReasonDestinationDenied},
		{name: "numeric_host", target: "http://2130706433/", reason: ReasonDestinationDenied},
		{name: "scheme", target: "ftp://example.com/resource", reason: ReasonInvalidTarget},
		{name: "credentials", target: "https://user:secret@example.com/", reason: ReasonInvalidTarget},
		{name: "port", target: "https://example.com:99999/", reason: ReasonInvalidTarget},
		{name: "malformed", target: "http://%zz/", reason: ReasonInvalidTarget},
		{name: "private_answer", target: "https://example.com/", addresses: []string{"10.0.0.1"}, reason: ReasonDestinationDenied, lookups: true},
		{name: "mixed_answer", target: "https://example.com/", addresses: []string{"93.184.216.34", "127.0.0.1"}, reason: ReasonDestinationDenied, lookups: true},
		{name: "empty_answer", target: "https://example.com/", reason: ReasonDNSNotFound, lookups: true},
		{name: "nxdomain", target: "https://example.com/", err: &net.DNSError{Err: "no such host", Name: "example.com", IsNotFound: true}, reason: ReasonDNSNotFound, lookups: true},
		{name: "dns_timeout", target: "https://example.com/", err: timeout, reason: ReasonDNSTimeout, lookups: true},
		{name: "lookup_deadline", target: "https://example.com/", err: errors.Wrap(context.DeadlineExceeded, "lookup"), reason: ReasonDNSTimeout, lookups: true},
		{name: "servfail", target: "https://example.com/", err: &net.DNSError{Err: "server misbehaving", Name: "example.com", IsTemporary: true}, reason: ReasonDNSFailed, lookups: true},
		{name: "resolver_error", target: "https://example.com/", err: errors.New("resolver unavailable"), reason: ReasonDNSFailed, lookups: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			_, err := Admit(t.Context(), tc.target, staticResolver(&calls, tc.err, tc.addresses...))
			require.Error(t, err)
			reason, ok := ReasonOf(errors.Wrap(err, "caller context"))
			require.True(t, ok)
			require.Equal(t, tc.reason, reason)
			require.Equal(t, tc.reason.Expected(), IsExpectedRefusal(err))
			require.Equal(t, tc.lookups, calls > 0, "only hostname targets may reach the resolver")
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err, "resolver identity must be preserved")
			}
		})
	}
}

// TestReasonExpected separates caller-fixable refusals from operational failures.
func TestReasonExpected(t *testing.T) {
	for _, reason := range []Reason{ReasonDestinationDenied, ReasonInvalidTarget, ReasonDNSNotFound} {
		require.True(t, reason.Expected(), reason)
	}
	for _, reason := range []Reason{ReasonDNSTimeout, ReasonDNSFailed, Reason("unknown")} {
		require.False(t, reason.Expected(), reason)
	}
	require.False(t, IsExpectedRefusal(errors.New("storage failed")))
	require.False(t, IsExpectedRefusal(nil))
}

// TestAdmitAcceptsPublicTargets returns the pinned public answer unchanged.
func TestAdmitAcceptsPublicTargets(t *testing.T) {
	calls := 0
	admission, err := Admit(t.Context(), "https://Example.COM./path?q=1", staticResolver(&calls, nil, "93.184.216.34"))
	require.NoError(t, err)
	require.Equal(t, "example.com", admission.Host)
	require.Equal(t, 443, admission.Port)
	require.Equal(t, []string{"93.184.216.34"}, admission.Addresses)

	admission, err = Admit(t.Context(), "http://93.184.216.34:8080/", nil)
	require.NoError(t, err, "a public literal must not need the resolver")
	require.Equal(t, []string{"93.184.216.34"}, admission.Addresses)
}

// TestAdmitPreservesCallerContext reports an ended caller context as itself,
// never as a DNS or destination refusal.
func TestAdmitPreservesCallerContext(t *testing.T) {
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	calls := 0
	_, err := Admit(canceled, "https://example.com/", staticResolver(&calls, nil, "93.184.216.34"))
	require.ErrorIs(t, err, context.Canceled)
	_, refused := ReasonOf(err)
	require.False(t, refused)

	deadline, stop := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer stop()
	_, err = Admit(deadline, "https://example.com/", func(ctx context.Context, _ string) ([]net.IPAddr, error) {
		<-ctx.Done()
		return nil, errors.WithStack(ctx.Err())
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	_, refused = ReasonOf(err)
	require.False(t, refused, "the caller's own budget is not a resolver timeout")
}

// TestDependencyCollapsesResolverFailures characterizes why Admit observes the
// resolver: the shared policy returns the same sanitized error type for a DNS
// timeout and a private destination. If this starts failing, the dependency
// preserves resolver identity and Admit's observation layer can be simplified.
func TestDependencyCollapsesResolverFailures(t *testing.T) {
	calls := 0
	_, timeoutErr := crawleregress.AdmitURLWithResolver(t.Context(), "https://example.com/",
		staticResolver(&calls, &net.DNSError{Err: "i/o timeout", IsTimeout: true}))
	_, privateErr := crawleregress.AdmitURLWithResolver(t.Context(), "https://example.com/",
		staticResolver(&calls, nil, "10.0.0.1"))
	require.Error(t, timeoutErr)
	require.Error(t, privateErr)
	var dnsErr *net.DNSError
	require.False(t, errors.As(timeoutErr, &dnsErr))
	require.NotErrorIs(t, timeoutErr, context.DeadlineExceeded)
	require.IsType(t, errors.Cause(privateErr), errors.Cause(timeoutErr))
}

// TestAdmissionReasonsSurviveArchiveProjection proves the cross-package contract:
// every reason defined here is exported verbatim by the archive alert projection.
func TestAdmissionReasonsSurviveArchiveProjection(t *testing.T) {
	for _, reason := range []Reason{ReasonDestinationDenied, ReasonInvalidTarget, ReasonDNSNotFound, ReasonDNSTimeout, ReasonDNSFailed} {
		var captured []zapcore.Field
		hook := log.CrawlerDiagnosticAlertHook(func(_ zapcore.Entry, fields []zapcore.Field) error {
			captured = fields
			return nil
		})
		cause := errors.Wrap(&AdmissionError{reason: reason, cause: errors.New("private cause")}, "fetch url")
		require.NoError(t, hook(zapcore.Entry{LoggerName: log.ArchiveLoggerName},
			[]zapcore.Field{zap.String("stage", "fetch"), zap.Error(cause)}))
		require.Equal(t, []zapcore.Field{zap.String("stage", "admission"), zap.String("category", string(reason))}, captured)
	}
}
