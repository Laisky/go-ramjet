package log

import (
	"regexp"
	"strings"
)

var otlpKeyVersion = regexp.MustCompile(`v[0-9]+$`)
var otlpKeySeparators = strings.NewReplacer("_", "", "-", "", ".", "", " ", "")

// sensitiveOTLPKey follows go-utils' credential/raw-payload deny contract rather than a diagnostic allowlist.
// Actual ramjet MCP request/response and search result labels are private payloads as well.
func sensitiveOTLPKey(key string) bool {
	compact := otlpKeyVersion.ReplaceAllString(otlpKeySeparators.Replace(strings.ToLower(key)), "")
	switch compact {
	case "query", "querytext", "querystring", "searchquery", "prompt", "content", "body", "payload", "headers", "url", "uri",
		"password", "passwd", "pwd", "secret", "token", "credential", "credentials", "cookie", "setcookie", "html", "markdown", "rawresponse", "receipt",
		"request", "response", "argstruncated", "responsetruncated", "summary", "result":
		return true
	}
	for _, suffix := range []string{"apikey", "apitoken", "accesskey", "accesstoken", "refreshtoken", "idtoken", "authtoken", "pushtoken", "clientsecret", "authorization", "authheader", "credential", "credentials", "cookieheader", "bearertoken", "authorizationtoken", "sessiontoken", "password", "passwd", "passwordhash", "privatekey", "secretkey", "secret", "tokenprefix", "apikeyprefix", "requestheaders", "responseheaders", "requestbody", "responsebody", "rawbody", "requestpayload", "responsepayload", "rawpayload", "targeturl", "requesturl", "responseurl", "rawhtml", "resulthtml", "rawmarkdown", "resultmarkdown", "egressreceipt"} {
		if strings.HasSuffix(compact, suffix) {
			return true
		}
	}
	return false
}
