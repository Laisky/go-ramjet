package localstorage

import (
	"context"
	"expvar"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-ramjet/library/log"
)

// maxArchiveSchedulers bounds how many archive lookups proxy requests run at once.
// A lookup is one S3 HEAD that normally takes milliseconds, so the bound is only
// reached when S3 is slow or the unauthenticated proxy is flooded.
const maxArchiveSchedulers = 16

// Archive scheduling outcomes, counted in archiveScheduleOutcomes.
const (
	scheduleQueued       = "queued"
	schedulePending      = "pending"
	scheduleArchived     = "archived"
	scheduleSaturated    = "saturated"
	scheduleQueueFull    = "queue_full"
	scheduleLookupFailed = "lookup_failed"
)

var (
	// archiveSchedulerSlots holds one token per running archive scheduler.
	archiveSchedulerSlots = make(chan struct{}, maxArchiveSchedulers)
	// archiveScheduleTimeout bounds one scheduler's lookup and enqueue; tests replace it.
	archiveScheduleTimeout = 10 * time.Second
	// archiveScheduleOutcomes counts scheduling outcomes in this process, keyed by
	// the schedule* names, following the repository's expvar metrics convention.
	archiveScheduleOutcomes = expvar.NewMap("arweave_archive_schedule_total")
	// errArchiveQueueFull reports that the archive queue had no room for a url.
	errArchiveQueueFull = errors.New("taskChan is full, please wait for a while")
)

// scheduleArchive queues target for archiving in the background after a proxy
// request was admitted, without delaying the answer.
//
// At most maxArchiveSchedulers run at once; a request beyond that skips
// archiving, which is best-effort and retried by later requests for the url.
// That skip happens inside the request and is a WARN on its logger. The
// scheduler runs under detachedArchiveContext(requestCtx, logger), bounded by
// archiveScheduleTimeout and unaffected by the client leaving. Because it ends
// after the request was answered, its failures are recorded by
// logScheduleFailure on the archive logger, the reviewed alert scope.
func scheduleArchive(requestCtx context.Context, logger glog.Logger, target string) {
	select {
	case archiveSchedulerSlots <- struct{}{}:
	default:
		archiveScheduleOutcomes.Add(scheduleSaturated, 1)
		logger.Warn("skip url archiving, archive schedulers are saturated",
			zap.String("stage", "schedule"), zap.String("category", scheduleSaturated),
			zap.String("target_ref", log.ArchiveTargetRef(target)))
		return
	}
	ctx := detachedArchiveContext(requestCtx, logger)
	requestBackgroundWork.Go(func() {
		defer func() { <-archiveSchedulerSlots }()
		scheduleCtx, cancel := context.WithTimeout(ctx, archiveScheduleTimeout)
		defer cancel()
		if err := SaveUrlContent(scheduleCtx, taskItem{url: target}); err != nil {
			logScheduleFailure(ctx, target, err)
		}
	})
}

// detachedArchiveContext returns the context for archive work a proxy request
// leaves behind. It carries only logger and requestCtx's trace id: it has no
// cancellation, and it holds no reference to the request or to its gin context,
// which gin recycles for another request once the handler returns.
func detachedArchiveContext(requestCtx context.Context, logger glog.Logger) context.Context {
	ctx := gmw.SetLogger(context.Background(), logger)
	if traceID := requestCtx.Value(gutils.TracingKey); traceID != nil {
		ctx = context.WithValue(ctx, gutils.TracingKey, traceID)
	}
	return ctx
}

// logScheduleFailure records err, a scheduling failure of target, once on the
// archive logger with the request's trace id. A failed lookup silently stops
// archiving, so it is an ERROR whose outbound alert carries only the bounded
// archive projection (stage "schedule" and a finite category). A full queue is
// expected under bursts, so it is a WARN with category queue_full and stays local.
func logScheduleFailure(ctx context.Context, target string, err error) {
	fields := []zap.Field{
		zap.String("stage", "schedule"),
		zap.String("url", target),
		zap.String("target_ref", log.ArchiveTargetRef(target)),
		zap.Error(err),
	}
	if traceID, ok := ctx.Value(gutils.TracingKey).(gutils.JaegerTracingID); ok {
		fields = append(fields, zap.String("trace_id", traceID.String()))
	}
	if errors.Is(err, errArchiveQueueFull) {
		log.ArchiveLogger().Warn("skip url archiving", append(fields, zap.String("category", scheduleQueueFull))...)
		return
	}
	log.ArchiveLogger().Error("failed to schedule url archiving", fields...)
}
