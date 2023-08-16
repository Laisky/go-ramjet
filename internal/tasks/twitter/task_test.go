// Package twitter implements twitter sync task.
package twitter

import (
	"testing"

	"github.com/Laisky/go-ramjet/library/config"
	"github.com/Laisky/go-ramjet/library/log"
	"github.com/Laisky/testify/require"
)

func Test_syncFromMongodb2Es(t *testing.T) {
	config.LoadTest(t)
	err := syncFromMongodb2Es(log.Logger.Named("syncFromMongodb2Es"))
	require.NoError(t, err)
}
