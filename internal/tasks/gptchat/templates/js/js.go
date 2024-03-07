// Package templates implements templates.
package templates

import _ "embed"

//go:embed chat.js
var Chat []byte

//go:embed libs.js
var Libs []byte

//go:embed chat-prompts.js
var ChatPrompts []byte

//go:embed payment.js
var Payment []byte
