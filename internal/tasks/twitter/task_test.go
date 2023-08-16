// Package twitter implements twitter sync task.
package twitter

import (
	"testing"

	"github.com/Laisky/testify/require"

	"github.com/Laisky/go-ramjet/library/config"
	"github.com/Laisky/go-ramjet/library/log"
)

func Test_syncFromMongodb2Es(t *testing.T) {
	config.LoadTest(t)
	err := syncFromMongodb2Es(log.Logger.Named("syncFromMongodb2Es"))
	require.NoError(t, err)
}
