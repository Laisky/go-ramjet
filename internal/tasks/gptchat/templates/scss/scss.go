// Package static implements static files.
package static

import _ "embed"

//go:embed base.css
var CSSBase []byte

//go:embed chat.css
var ChatCss []byte

var SitesCSS []byte

func init() {
	SitesCSS = append(SitesCSS, CSSBase...)
	SitesCSS = append(SitesCSS, ChatCss...)
}
