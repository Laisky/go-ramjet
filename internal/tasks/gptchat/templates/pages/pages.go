// Package pages implements web pages.
package pages

import _ "embed"

//go:embed chat.html
var Chat string

//go:embed payment.html
var Payment string
