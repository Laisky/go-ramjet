package twitter

import (
	"testing"

	"github.com/Laisky/go-ramjet/library/config"
	"github.com/stretchr/testify/require"
)

func TestService_SyncSearchTweets(t *testing.T) {
	config.LoadTest()
	err := initSvc()
	require.NoError(t, err)

	err = svc.SyncSearchTweets()
	require.NoError(t, err)
}
