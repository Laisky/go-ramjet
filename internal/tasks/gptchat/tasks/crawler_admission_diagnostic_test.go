package tasks

import (
	"context"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestCrawlerAdmissionErrorPreservesCause verifies wrapped diagnostic attribution retains cancellation identity.
func TestCrawlerAdmissionErrorPreservesCause(t *testing.T) {
	cause := errors.Wrap(context.Canceled, "synthetic resolver cancellation")
	wrapped := errors.Wrap(&crawlerAdmissionError{cause: cause}, "submit crawler task")
	require.ErrorIs(t, wrapped, context.Canceled)
	var diagnostic interface{ CrawlerFailureStage() string }
	require.ErrorAs(t, wrapped, &diagnostic)
	require.Equal(t, "admission", diagnostic.CrawlerFailureStage())
}
