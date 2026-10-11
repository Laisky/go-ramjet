//go:build !linux || !amd64

package main

import (
	"fmt"
	"os"
)

// main refuses unsupported platforms instead of starting an unconfined browser.
func main() {
	if _, err := fmt.Fprintln(os.Stderr, "crawler browser sandbox requires Linux amd64"); err != nil {
		os.Exit(2)
	}
	os.Exit(1)
}
