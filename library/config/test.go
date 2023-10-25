// Package config provides config for go-ramjet
package config

import (
	"testing"

	gconfig "github.com/Laisky/go-config/v2"
	"github.com/Laisky/testify/require"
)

func LoadTest(tb testing.TB) {
	tb.Helper()
	err := gconfig.Shared.LoadFromFile("/opt/configs/go-ramjet/settings.yml")
	require.NoError(tb, err)
}
