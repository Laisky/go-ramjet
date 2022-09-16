package twitter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/config"
)

func TestService_SyncSearchTweets(t *testing.T) {
	config.LoadTest()
	err := initSvc(context.Background())
	require.NoError(t, err)

	err = svc.SyncSearchTweets()
	require.NoError(t, err)
}
