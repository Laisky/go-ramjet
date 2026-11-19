// Package egress applies the shared public-destination policy to outbound
// requests whose target a user chooses. It attributes every refusal to a
// finite Reason so callers can decide severity and alert content without
// parsing dependency error text.
package egress

import (
	"context"
	"net"
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"

	"github.com/Laisky/go-ramjet/library/log"
)

// Resolver resolves host to IP addresses with the semantics of net.Resolver.LookupIPAddr.
type Resolver func(ctx context.Context, host string) ([]net.IPAddr, error)

// Reason is a finite, alert-safe attribution of an admission refusal.
type Reason string

const (
	// ReasonDestinationDenied means the destination itself is not public:
	// a non-public literal, a local hostname, or a non-public DNS answer.
	ReasonDestinationDenied Reason = "destination_denied"
	// ReasonInvalidTarget means the URL shape (scheme, credentials, port, encoding)
	// was refused independently of its destination.
	ReasonInvalidTarget Reason = "invalid_target"
	// ReasonDNSNotFound means the hostname does not exist or has no addresses.
	ReasonDNSNotFound Reason = "dns_not_found"
	// ReasonDNSTimeout means hostname resolution timed out before the caller's context ended.
	ReasonDNSTimeout Reason = "dns_timeout"
	// ReasonDNSFailed means the resolver failed for any other reason.
	ReasonDNSFailed Reason = "dns_failed"
)

// Expected reports whether r is caused by the requested target rather than by
// this service's infrastructure. Expected refusals are input errors, not incidents.
func (r Reason) Expected() bool {
	switch r {
	case ReasonDestinationDenied, ReasonInvalidTarget, ReasonDNSNotFound:
		return true
	default:
		return false
	}
}

// AdmissionError is a refused admission carrying its typed reason and preserved cause.
// For DNS reasons the cause is the resolver's own error, so timeout and DNS
// identities stay available to errors.Is and errors.As.
type AdmissionError struct {
	reason Reason
	cause  error
}

// AdmissionError implements the logging package's explicit admission contract.
var _ log.AdmissionReasonError = (*AdmissionError)(nil)

// Error returns the reason and the cause; neither contains credentials beyond the caller's own target.
func (e *AdmissionError) Error() string {
	return "target admission refused (" + string(e.reason) + "): " + e.cause.Error()
}

// Unwrap returns the preserved cause.
func (e *AdmissionError) Unwrap() error { return e.cause }

// Reason returns the typed refusal reason.
func (e *AdmissionError) Reason() Reason { return e.reason }

// AdmissionReason returns the reason for packages that must not import egress.
func (e *AdmissionError) AdmissionReason() string { return string(e.reason) }

// ReasonOf returns the admission refusal reason carried anywhere in err's chain.
// The boolean is false when err is not an admission refusal.
func ReasonOf(err error) (Reason, bool) {
	var admission *AdmissionError
	if !errors.As(err, &admission) {
		return "", false
	}
	return admission.reason, true
}

// IsExpectedRefusal reports whether err is an admission refusal caused by the
// requested target itself, which callers should answer as invalid input
// rather than raise as an operational failure.
func IsExpectedRefusal(err error) bool {
	reason, ok := ReasonOf(err)
	return ok && reason.Expected()
}

// lookupObservation records the last resolver outcome seen during one admission.
type lookupObservation struct {
	called    bool
	addresses []net.IPAddr
	err       error
}

// Admit applies the shared crawler egress policy to raw and returns the pinned
// admission. resolve may be nil to use the system resolver.
//
// Refusals are returned as *AdmissionError. The shared policy sanitizes resolver
// failures into the same input error it uses for private destinations, so the
// resolver outcome is observed at the lookup boundary instead of parsed from
// text. When ctx has ended, its error identity is returned unchanged.
func Admit(ctx context.Context, raw string, resolve Resolver) (crawleregress.Admission, error) {
	if resolve == nil {
		resolve = net.DefaultResolver.LookupIPAddr
	}
	var observed lookupObservation
	admission, err := crawleregress.AdmitURLWithResolver(ctx, raw, func(lookupCtx context.Context, host string) ([]net.IPAddr, error) {
		addresses, lookupErr := resolve(lookupCtx, host)
		observed = lookupObservation{called: true, addresses: addresses, err: lookupErr}
		return addresses, lookupErr
	})
	if err != nil {
		if ctx != nil && ctx.Err() != nil {
			return crawleregress.Admission{}, errors.WithStack(err)
		}
		reason, cause := classifyRefusal(ctx, raw, observed, err)
		return crawleregress.Admission{}, errors.WithStack(&AdmissionError{reason: reason, cause: cause})
	}
	return admission, nil
}

// classifyRefusal maps one refused admission to its reason and the cause to preserve.
func classifyRefusal(ctx context.Context, raw string, observed lookupObservation, policyErr error) (Reason, error) {
	switch {
	case observed.called && observed.err != nil:
		return resolverReason(observed.err), observed.err
	case observed.called && len(observed.addresses) == 0:
		return ReasonDNSNotFound, policyErr
	case observed.called:
		// Resolution succeeded, so the only remaining refusal is a non-public answer.
		return ReasonDestinationDenied, policyErr
	default:
		return preResolutionReason(ctx, raw), policyErr
	}
}

// resolverReason classifies a resolver failure without reading its message.
func resolverReason(err error) Reason {
	var dnsErr *net.DNSError
	var netErr net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return ReasonDNSTimeout
	case errors.As(err, &netErr) && netErr.Timeout():
		return ReasonDNSTimeout
	case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
		return ReasonDNSNotFound
	default:
		return ReasonDNSFailed
	}
}

// errProbeResolution stops a classification probe at the lookup boundary.
var errProbeResolution = errors.New("classification probe does not resolve")

// preResolutionReason distinguishes a refused destination from a refused URL
// shape when admission failed before any lookup. It re-admits only raw's host
// through the same shared policy with a resolver that never touches the
// network, so destination rules stay defined in exactly one place.
func preResolutionReason(ctx context.Context, raw string) Reason {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Hostname() == "" {
		return ReasonInvalidTarget
	}
	host := parsed.Hostname()
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	probe := (&url.URL{Scheme: "https", Host: host, Path: "/"}).String()
	probeCtx := context.Background()
	if ctx != nil {
		probeCtx = context.WithoutCancel(ctx)
	}
	resolved := false
	_, probeErr := crawleregress.AdmitURLWithResolver(probeCtx, probe,
		func(context.Context, string) ([]net.IPAddr, error) {
			resolved = true
			return nil, errProbeResolution
		})
	if probeErr != nil && !resolved {
		return ReasonDestinationDenied
	}
	return ReasonInvalidTarget
}
