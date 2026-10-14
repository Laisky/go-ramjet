package tasks

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/url"
	"os"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/chromedp/chromedp"
)

const crawlerDiagnosticTextLimit = 2048

// crawlerCauseText returns useful sanitized text without rendering known page payloads or transport targets.
// A cause formatter panic produces a fixed message and never exposes its panic value.
func crawlerCauseText(cause error) (text string) {
	defer func() {
		if recover() != nil {
			text = "diagnostic cause unavailable"
		}
	}()
	if cause == nil {
		return ""
	}
	var exception *chromedp.ExceptionError
	if errors.As(cause, &exception) {
		return "browser evaluation failed"
	}
	var request *url.Error
	if errors.As(cause, &request) {
		return crawlerCauseText(request.Err)
	}
	var dns *net.DNSError
	if errors.As(cause, &dns) {
		if dns.IsTimeout {
			return "DNS lookup failed: timeout"
		}
		if dns.IsNotFound {
			return "DNS lookup failed: host not found"
		}
		return "DNS lookup failed"
	}
	var verification *tls.CertificateVerificationError
	var hostname x509.HostnameError
	var authority x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	if errors.As(cause, &verification) || errors.As(cause, &hostname) || errors.As(cause, &authority) || errors.As(cause, &invalid) {
		return "TLS certificate verification failed"
	}
	var record tls.RecordHeaderError
	if errors.As(cause, &record) {
		return "TLS record header invalid"
	}
	var connection *net.OpError
	if errors.As(cause, &connection) {
		if detail := crawlerCauseText(connection.Err); detail != "" {
			return "connection failed: " + detail
		}
		return "connection failed"
	}
	var system *os.SyscallError
	if errors.As(cause, &system) {
		if detail := crawlerCauseText(system.Err); detail != "" {
			return "system call failed: " + detail
		}
		return "system call failed"
	}
	var errno syscall.Errno
	if errors.As(cause, &errno) {
		return errno.Error()
	}
	return glog.SanitizeDiagnosticText(cause.Error())
}

// boundCrawlerDiagnosticText caps the complete emitted text, including its marker, on a valid UTF-8 boundary.
func boundCrawlerDiagnosticText(text string) string {
	text = strings.ToValidUTF8(text, "�")
	if len(text) <= crawlerDiagnosticTextLimit {
		return text
	}
	const marker = " [truncated]"
	end := crawlerDiagnosticTextLimit - len(marker)
	for !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + marker
}
