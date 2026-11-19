package config

import (
	"testing"

	gconfig "github.com/Laisky/go-config/v2"
)

// setConfig sets key in the shared configuration for the duration of t and
// restores the previous value when t ends, so no test leaks settings into the
// tests that run after it (order dependence surfaced by -shuffle=on).
func setConfig(t *testing.T, key string, value any) {
	t.Helper()
	previous := gconfig.Shared.Get(key)
	gconfig.Shared.Set(key, value)
	t.Cleanup(func() { gconfig.Shared.Set(key, previous) })
}
