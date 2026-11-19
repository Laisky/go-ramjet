package web

import "github.com/gin-gonic/gin"

// UntrustedContentPolicy is the Content-Security-Policy for third-party content
// that a handler relays on this service's origin. The sandbox directive without
// allow-same-origin gives the document an opaque origin, so it cannot read this
// origin's cookies or storage (where the GPTChat app keeps credentials), and
// without allow-scripts it cannot run scripts or submit forms. Links may still
// open in a new, unsandboxed tab. Framing stays limited to the same origin.
const UntrustedContentPolicy = "sandbox allow-popups allow-popups-to-escape-sandbox; frame-ancestors 'self'"

// MarkUntrustedContent sets the response headers that keep third-party content
// relayed by c's handler from acting on this service's origin. It replaces the
// default Content-Security-Policy and must be called before the body is written.
func MarkUntrustedContent(c *gin.Context) {
	c.Header("Content-Security-Policy", UntrustedContentPolicy)
	c.Header("X-Content-Type-Options", "nosniff")
}
