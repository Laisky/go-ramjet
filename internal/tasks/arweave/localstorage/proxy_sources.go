package localstorage

import (
	"context"
	"io"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
)

// contentSource is one way to obtain the content the proxy answers with.
type contentSource struct {
	// name identifies the source in errors and logs.
	name string
	// status is reported in X-Laisky-Cache-Status when this source answers.
	status string
	// delay is the head start earlier sources get before this source starts.
	delay time.Duration
	// load opens the content of target; the caller owns the returned body.
	load func(ctx context.Context, target string) (io.ReadCloser, error)
}

// sourceOutcome is the single report every source makes when it finishes.
type sourceOutcome struct {
	index int
	body  io.ReadCloser
	err   error
}

// firstAvailableContent runs sources concurrently for target and returns the body
// and status of the first source that succeeds. The caller must close the body.
//
// Every source runs under its own context derived from ctx and reports exactly
// one outcome into a channel buffered for all sources, so a result can never be
// lost because nobody was waiting for it yet, and no source can block after this
// function has returned. Once a source wins, every other source is canceled at
// once and any body a loser still produces is closed in the background. The
// winner's context lives until its body is closed, so the caller can stream it.
//
// When every source fails, the first source's error is returned, because later
// sources are fallbacks; their failures are logged at debug level through logger.
// When ctx ends before any source succeeds, ctx's error is returned.
func firstAvailableContent(ctx context.Context, logger glog.Logger, target string, sources []contentSource) (io.ReadCloser, string, error) {
	if len(sources) == 0 {
		return nil, "", errors.New("no content sources")
	}

	outcomes := make(chan sourceOutcome, len(sources))
	cancels := make([]context.CancelFunc, len(sources))
	for i, source := range sources {
		sourceCtx, cancel := context.WithCancel(ctx)
		cancels[i] = cancel
		go func() {
			body, err := runContentSource(sourceCtx, target, source)
			outcomes <- sourceOutcome{index: i, body: body, err: err}
		}()
	}
	cancelOthers := func(winner int) {
		for i, cancel := range cancels {
			if i != winner {
				cancel()
			}
		}
	}

	errs := make([]error, len(sources))
	for pending := len(sources); pending > 0; pending-- {
		select {
		case outcome := <-outcomes:
			if outcome.err != nil {
				errs[outcome.index] = outcome.err
				cancels[outcome.index]()
				continue
			}
			cancelOthers(outcome.index)
			go discardLosingContent(logger, outcomes, pending-1)
			winner := &cancelOnClose{ReadCloser: outcome.body, cancel: cancels[outcome.index]}
			return winner, sources[outcome.index].status, nil
		case <-ctx.Done():
			cancelOthers(-1)
			go discardLosingContent(logger, outcomes, pending)
			return nil, "", errors.WithStack(ctx.Err())
		}
	}

	for i, err := range errs[1:] {
		logger.Debug("fallback content source failed", zap.String("source", sources[i+1].name), zap.Error(err))
	}
	return nil, "", errs[0]
}

// runContentSource waits out source's head start, unless ctx ends first, and then
// loads target. It returns either a non-nil body or a non-nil error, never both.
func runContentSource(ctx context.Context, target string, source contentSource) (io.ReadCloser, error) {
	if source.delay > 0 {
		timer := time.NewTimer(source.delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, errors.Wrapf(ctx.Err(), "wait to load content from %s", source.name)
		}
	}

	body, err := source.load(ctx, target)
	if err != nil {
		return nil, errors.Wrapf(err, "load content from %s", source.name)
	}
	if body == nil {
		return nil, errors.Errorf("load content from %s: no body", source.name)
	}
	return body, nil
}

// discardLosingContent receives the outcomes still pending after a race has been
// decided and closes every body a losing source produced. It ends once all
// sources have reported, which is prompt because losing sources are canceled.
func discardLosingContent(logger glog.Logger, outcomes <-chan sourceOutcome, pending int) {
	for range pending {
		outcome := <-outcomes
		if outcome.err != nil {
			logger.Debug("content source lost the race", zap.Error(outcome.err))
			continue
		}
		if err := outcome.body.Close(); err != nil {
			logger.Debug("close losing content source body", zap.Error(err))
		}
	}
}

// cancelOnClose ends the winning source's context once its body is closed.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

// Close closes the body and then releases the source's context.
func (b *cancelOnClose) Close() error {
	defer b.cancel()
	return errors.WithStack(b.ReadCloser.Close())
}
