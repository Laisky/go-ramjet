// Package static implements static files.
package static

import _ "embed"

//go:embed favicon.ico
var Favicon []byte
