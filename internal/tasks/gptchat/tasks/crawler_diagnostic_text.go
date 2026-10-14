package tasks

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
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

// crawlerCauseText retains sanitized operational context without rendering page payloads, certificates, URLs or addresses.
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
		return crawlerBrowserExceptionText(exception)
	}
	var request *url.Error
	if errors.As(cause, &request) {
		return glog.SanitizeDiagnosticText(request.Op) + ": " + crawlerCauseText(request.Err)
	}
	var dns *net.DNSError
	if errors.As(cause, &dns) {
		text := "DNS lookup failed"
		if name := glog.SanitizeDiagnosticText(dns.Name); name != "" {
			text += " for " + name
		}
		if detail := glog.SanitizeDiagnosticText(dns.Err); detail != "" {
			return text + ": " + detail
		}
		if dns.IsTimeout {
			return text + ": timeout"
		}
		if dns.IsNotFound {
			return text + ": host not found"
		}
		return text
	}
	var hostname x509.HostnameError
	if errors.As(cause, &hostname) {
		return "TLS certificate verification failed for " + glog.SanitizeDiagnosticText(hostname.Host)
	}
	var hostnamePointer *x509.HostnameError
	if errors.As(cause, &hostnamePointer) {
		return "TLS certificate verification failed for " + glog.SanitizeDiagnosticText(hostnamePointer.Host)
	}
	var verification *tls.CertificateVerificationError
	var authority x509.UnknownAuthorityError
	var authorityPointer *x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var invalidPointer *x509.CertificateInvalidError
	if errors.As(cause, &verification) || errors.As(cause, &authority) || errors.As(cause, &authorityPointer) || errors.As(cause, &invalid) || errors.As(cause, &invalidPointer) {
		return "TLS certificate verification failed"
	}
	var record tls.RecordHeaderError
	if errors.As(cause, &record) {
		return "TLS record header invalid"
	}
	var connection *net.OpError
	if errors.As(cause, &connection) {
		text := "connection failed: " + strings.TrimSpace(glog.SanitizeDiagnosticText(connection.Op)+" "+glog.SanitizeDiagnosticText(connection.Net))
		if detail := crawlerCauseText(connection.Err); detail != "" {
			return text + ": " + detail
		}
		return text
	}
	var system *os.SyscallError
	if errors.As(cause, &system) {
		text := "system call failed: " + glog.SanitizeDiagnosticText(system.Syscall)
		if detail := crawlerCauseText(system.Err); detail != "" {
			return text + ": " + detail
		}
		return text
	}
	var errno syscall.Errno
	if errors.As(cause, &errno) {
		return errno.Error()
	}
	return glog.SanitizeDiagnosticText(cause.Error())
}

// crawlerBrowserExceptionText retains standard error class and bounded source location without page text or descriptions.
func crawlerBrowserExceptionText(exception *chromedp.ExceptionError) string {
	text := "browser evaluation failed"
	if exception.ExceptionDetails == nil {
		return text
	}
	if object := exception.Exception; object != nil {
		// Custom class names are page-controlled content; only built-in JavaScript error identities are diagnostic types.
		switch object.ClassName {
		case "Error", "EvalError", "RangeError", "ReferenceError", "SyntaxError", "TypeError", "URIError", "AggregateError":
			text += ": " + object.ClassName
		}
	}
	if exception.LineNumber >= 0 && exception.LineNumber <= 1<<24 && exception.ColumnNumber >= 0 && exception.ColumnNumber <= 1<<24 {
		text += fmt.Sprintf(" (%d:%d)", exception.LineNumber, exception.ColumnNumber)
	}
	return text
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
