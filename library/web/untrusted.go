package web

import (
	"mime"

	"github.com/gin-gonic/gin"
)

// UntrustedContentPolicy is the Content-Security-Policy for third-party content
// that a handler relays on this service's origin. The sandbox directive without
// allow-same-origin gives the document an opaque origin, so it cannot read this
// origin's cookies or storage (where the GPTChat app keeps credentials), and
// without allow-scripts it cannot run scripts or submit forms. Links may still
// open in a new, unsandboxed tab. Framing stays limited to the same origin.
const UntrustedContentPolicy = "sandbox allow-popups allow-popups-to-escape-sandbox; " + sameOriginFramingPolicy

// PassiveContentPolicy is the Content-Security-Policy for relayed media types
// that browsers display with built-in viewers isolated from this origin. They
// cannot act on the origin, and a sandbox would stop the viewer from loading.
const PassiveContentPolicy = sameOriginFramingPolicy

// MarkUntrustedContent sets the response headers that keep third-party content
// relayed by c's handler from acting on this service's origin. contentType is
// the Content-Type the response will carry. Every type is sandboxed except the
// passive documents listed in isPassiveDocument; sniffing is always disabled.
// It replaces the default Content-Security-Policy and must be called before the
// body is written.
func MarkUntrustedContent(c *gin.Context, contentType string) {
	c.Header("X-Content-Type-Options", "nosniff")
	if isPassiveDocument(contentType) {
		c.Header("Content-Security-Policy", PassiveContentPolicy)
		return
	}
	c.Header("Content-Security-Policy", UntrustedContentPolicy)
}

// isPassiveDocument reports whether contentType names a document that browsers
// render in a built-in viewer outside the page's origin (currently PDF). An
// unparseable or empty type is treated as active and therefore sandboxed.
func isPassiveDocument(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "application/pdf"
}
