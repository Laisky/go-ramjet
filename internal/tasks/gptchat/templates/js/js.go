package templates

import _ "embed"

//go:embed chat.js
var Chat []byte

//go:embed common.js
var Common []byte

//go:embed libs.js
var Libs []byte

//go:embed chat-prompts.js
var ChatPrompts []byte
