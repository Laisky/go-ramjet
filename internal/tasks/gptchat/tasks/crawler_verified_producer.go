package tasks

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	rlibs "github.com/Laisky/laisky-blog-graphql/library/db/redis"
)

var (
	admitHTMLCrawlerURL            = crawleregress.AdmitURL
	enqueueAdmittedHTMLCrawlerTask = enqueueHTMLCrawlerTaskToRedis
	loadVerifiedHTMLCrawlerTask    = getHTMLCrawlerTaskResult
)

// submitAdmittedHTMLCrawlerTask admits target before mutation and returns the
// submitted task snapshot, which the result consumer uses to verify acquisition.
func submitAdmittedHTMLCrawlerTask(ctx context.Context, target string, outputMarkdown bool) (*rlibs.HTMLCrawlerTask, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	admission, err := admitHTMLCrawlerURL(ctx, target)
	if err != nil {
		return nil, errors.Wrap(err, "admit crawler target")
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.WithStack(err)
	}
	policy := admission.Policy(crawleregress.DefaultMaxRedirects, false)
	task := rlibs.NewHTMLCrawlerTaskWithEgress(target, "", outputMarkdown, &policy)
	task.AllowProviderFallback = true
	if err := enqueueAdmittedHTMLCrawlerTask(ctx, task); err != nil {
		return nil, errors.Wrap(err, "enqueue admitted crawler task")
	}
	return task, nil
}

// waitVerifiedHTMLCrawlerTask polls submitted and returns content only after
// verifying evidence against its immutable policy and identity snapshot.
func waitVerifiedHTMLCrawlerTask(ctx context.Context, submitted *rlibs.HTMLCrawlerTask) ([]byte, error) {
	if submitted == nil || submitted.Egress == nil {
		return nil, crawlerFailure("policy_missing", "admission", nil)
	}
	expected := *submitted
	expectedPolicy := crawleregress.ClonePolicy(*submitted.Egress)
	expected.Egress = &expectedPolicy
	submitted = &expected
	for {
		if err := ctx.Err(); err != nil {
			return nil, errors.WithStack(err)
		}
		task, err := loadVerifiedHTMLCrawlerTask(ctx, submitted.TaskID)
		if err != nil {
			return nil, errors.Wrap(err, "get crawler task result")
		}
		if task == nil {
			return nil, crawlerFailure("result_invalid", "verification", nil)
		}
		switch task.Status {
		case rlibs.TaskStatusSuccess:
			if task.TaskID != submitted.TaskID || task.Url != submitted.Url || task.OutputMarkdown != submitted.OutputMarkdown || task.Egress == nil {
				return nil, crawlerFailure("result_invalid", "verification", nil)
			}
			if crawleregress.PolicyDigest(task.Url, *task.Egress) != crawleregress.PolicyDigest(submitted.Url, *submitted.Egress) {
				return nil, crawlerFailure("result_invalid", "verification", nil)
			}
			if task.AllowProviderFallback && !submitted.AllowProviderFallback {
				return nil, crawlerFailure("result_invalid", "verification", nil)
			}
			if task.Acquisition != nil && task.Acquisition.Method == crawleregress.AcquisitionProvider && (!task.AllowProviderFallback || len(task.RequestChain) != 0) {
				return nil, crawlerFailure("result_invalid", "verification", nil)
			}
			if err := crawleregress.VerifyAcquisition(ctx, submitted.TaskID, submitted.Url, *submitted.Egress,
				submitted.AllowProviderFallback, task.Acquisition, task.EgressReceipt); err != nil {
				return nil, crawlerFailure("evidence_rejected", "verification", err)
			}
			if len(task.ResultHTML) > rendererMaxResponseBytes || len(task.ResultMarkdown) > rendererMaxResponseBytes {
				return nil, crawlerFailure("result_too_large", "verification", nil)
			}
			if submitted.OutputMarkdown {
				if len(task.ResultMarkdown) == 0 {
					return nil, crawlerFailure("empty_markdown", "conversion", nil)
				}
				return task.ResultMarkdown, nil
			}
			if len(task.ResultHTML) == 0 {
				return nil, crawlerFailure("empty_body", "render", nil)
			}
			return task.ResultHTML, nil
		case rlibs.TaskStatusPending, rlibs.TaskStatusRunning:
			select {
			case <-ctx.Done():
				return nil, errors.WithStack(ctx.Err())
			case <-time.After(time.Second):
			}
		case rlibs.TaskStatusFailed:
			// FailedReason is untrusted Redis audit data; do not echo it to callers.
			return nil, crawlerFailure("render_failed", "render", nil)
		default:
			return nil, crawlerFailure("result_invalid", "verification", nil)
		}
	}
}
