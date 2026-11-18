package tasks

import (
	"context"
	"testing"

	"github.com/Laisky/errors/v2"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-ramjet/library/egress"
	"github.com/Laisky/go-ramjet/library/log"
)

// TestCrawlerAdmissionErrorPreservesCause verifies wrapped diagnostic attribution retains cancellation identity.
func TestCrawlerAdmissionErrorPreservesCause(t *testing.T) {
	cause := errors.Wrap(context.Canceled, "synthetic resolver cancellation")
	wrapped := errors.Wrap(&crawlerAdmissionError{cause: cause}, "submit crawler task")
	require.ErrorIs(t, wrapped, context.Canceled)
	var diagnostic log.FailureStageError
	require.ErrorAs(t, wrapped, &diagnostic)
	require.Equal(t, "admission", diagnostic.CrawlerFailureStage())
}

// TestSubmitAdmittedHTMLCrawlerTaskAttributesRefusal runs the production
// admission seam: a non-public target is refused before any queue mutation,
// and the error carries both the admission stage and its typed reason.
func TestSubmitAdmittedHTMLCrawlerTaskAttributesRefusal(t *testing.T) {
	enqueue := enqueueAdmittedHTMLCrawlerTask
	t.Cleanup(func() { enqueueAdmittedHTMLCrawlerTask = enqueue })
	enqueueAdmittedHTMLCrawlerTask = func(context.Context, *rlibs.HTMLCrawlerTask) error {
		t.Error("refused target must not be enqueued")
		return nil
	}
	for target, want := range map[string]egress.Reason{
		"http://127.0.0.1:8080/private?token=q": egress.ReasonDestinationDenied,
		"http://localhost/":                     egress.ReasonDestinationDenied,
		"ftp://example.com/":                    egress.ReasonInvalidTarget,
	} {
		_, err := submitAdmittedHTMLCrawlerTask(t.Context(), target, false)
		require.Error(t, err)
		var staged log.FailureStageError
		require.ErrorAs(t, err, &staged)
		require.Equal(t, "admission", staged.CrawlerFailureStage())
		reason, ok := egress.ReasonOf(err)
		require.True(t, ok, target)
		require.Equal(t, want, reason, target)
		require.True(t, egress.IsExpectedRefusal(err), target)
	}
}
