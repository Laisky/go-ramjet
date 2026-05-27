// Package session hosts the per-request agent session, its submit/event
// split, and the append-only transcript. See proposal §3.1 and §3.3.
package session

import (
	"context"
	"crypto/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/oklog/ulid/v2"
)

// Op is the marker interface for everything pushed into Session.Submit.
// Concrete types are exported; the marker method keeps the union closed to
// this package.
type Op interface{ isOp() }

// OpUserTurn is the user-initiated turn that starts a run.
type OpUserTurn struct {
	Text        string
	Attachments []Blob
}

// OpInterrupt cancels the in-flight op without closing the session.
type OpInterrupt struct{}

// OpShutdown terminates the session permanently.
type OpShutdown struct{}

func (OpUserTurn) isOp()  {}
func (OpInterrupt) isOp() {}
func (OpShutdown) isOp()  {}

// EventSink is the narrow write-only surface tools and hooks publish events
// through. The session implementation provides one; callers never read from
// it.
type EventSink interface {
	Emit(Event) error
}

// Session is the per-request agent instance. The driver pushes Ops in and
// subscribers pull Events out — submit and event streams are decoupled.
type Session interface {
	Submit(ctx context.Context, op Op) error
	Events() <-chan Event
	Transcript() Transcript
	Close() error
}

// Config tunes Session behaviour. Logger is required to surface dropped
// events on backpressure; BufferSize controls per-subscriber channel depth.
type Config struct {
	BufferSize int
	Logger     glog.Logger
}

const defaultBufferSize = 64

// subscriberChanBufferRatio sizes each subscriber channel relative to the
// global buffer. The fan-out worker drops messages when a subscriber is
// behind to keep the producer from blocking.
const subscriberChanBufferRatio = 1

// NewSession constructs an in-memory Session. The returned session owns a
// single fan-out goroutine; Close shuts it down.
func NewSession(cfg Config) Session {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = defaultBufferSize
	}
	if cfg.Logger == nil {
		// Logger is required for backpressure warnings; fall back to a
		// console logger if the caller forgot.
		l, err := glog.NewConsoleWithName("agentx_session", glog.LevelInfo)
		if err != nil {
			panic(errors.Wrap(err, "session: bootstrap console logger"))
		}
		cfg.Logger = l
	}
	now := time.Now()
	s := &session{
		cfg:        cfg,
		transcript: NewTranscript(),
		incoming:   make(chan Event, cfg.BufferSize),
		done:       make(chan struct{}),
		stopFanout: make(chan struct{}),
		entropy:    ulid.Monotonic(rand.Reader, 0),
		nowMS:      uint64(now.UnixMilli()),
	}
	// Pre-register the primary subscriber so Events() can be called safely
	// before any event has been emitted.
	s.primary = s.subscribe()
	go s.fanout()
	return s
}

// session is the concrete in-memory implementation.
type session struct {
	cfg        Config
	transcript Transcript

	incoming   chan Event
	done       chan struct{}
	stopFanout chan struct{}
	closeOnce  sync.Once
	closed     atomic.Bool

	subsMu  sync.RWMutex
	subs    []*subscriber
	primary *subscriber

	cancelMu     sync.Mutex
	cancelFn     context.CancelFunc
	cancelSeq    uint64
	cancelSeqGen atomic.Uint64

	entOnce sync.Mutex
	entropy *ulid.MonotonicEntropy
	nowMS   uint64
}

// subscriber owns a buffered channel; the fan-out worker writes to it.
type subscriber struct {
	ch      chan Event
	dropped atomic.Int64
}

func (s *session) subscribe() *subscriber {
	sub := &subscriber{ch: make(chan Event, s.cfg.BufferSize*subscriberChanBufferRatio)}
	s.subsMu.Lock()
	s.subs = append(s.subs, sub)
	s.subsMu.Unlock()
	return sub
}

// Events returns the primary subscriber channel. Phase 1 uses a single
// subscriber (the SSE writer); the fan-out machinery still supports N.
func (s *session) Events() <-chan Event {
	return s.primary.ch
}

// Transcript exposes the append-only transcript. Safe for concurrent reads.
func (s *session) Transcript() Transcript { return s.transcript }

// Submit accepts an op. OpInterrupt cancels the in-flight op's context;
// OpShutdown closes the session; OpUserTurn registers the run context so a
// later OpInterrupt can target it. Phase 1 has no built-in op executor — the
// loop driver consumes ops out-of-band; Submit only manages lifecycle and
// the cancellation handle for interrupts.
func (s *session) Submit(ctx context.Context, op Op) error {
	if s.closed.Load() {
		return errors.New("session.Submit: session is closed")
	}
	switch op.(type) {
	case OpUserTurn:
		// Track the user-turn context so OpInterrupt can cancel it.
		opCtx, cancel := context.WithCancel(ctx)
		seq := s.nextCancelSeq()
		s.cancelMu.Lock()
		// Cancel any prior in-flight op before registering a new one to
		// avoid leaking goroutines.
		if s.cancelFn != nil {
			s.cancelFn()
		}
		s.cancelFn = cancel
		s.cancelSeq = seq
		s.cancelMu.Unlock()
		// Watcher: release the registered cancel when our opCtx is done
		// (caller cancelled, or a later Submit cancelled us), but only if
		// our seq still matches — a newer Submit may have replaced us.
		go func() {
			<-opCtx.Done()
			s.cancelMu.Lock()
			if s.cancelSeq == seq {
				s.cancelFn = nil
			}
			s.cancelMu.Unlock()
		}()
		return nil
	case OpInterrupt:
		s.cancelMu.Lock()
		if s.cancelFn != nil {
			s.cancelFn()
			s.cancelFn = nil
		}
		s.cancelMu.Unlock()
		return nil
	case OpShutdown:
		return s.Close()
	default:
		return errors.Errorf("session.Submit: unknown op type %T", op)
	}
}

func (s *session) nextCancelSeq() uint64 {
	return s.cancelSeqGen.Add(1)
}

// Emit publishes ev to the transcript and broadcasts it to subscribers.
// Implements EventSink so the same session can be passed where a sink is
// expected.
func (s *session) Emit(ev Event) error {
	if s.closed.Load() {
		return errors.New("session.Emit: session is closed")
	}
	if err := s.transcript.Append(ev); err != nil {
		return errors.Wrap(err, "append to transcript")
	}
	select {
	case s.incoming <- ev:
		return nil
	case <-s.stopFanout:
		return errors.New("session.Emit: session is closed")
	}
}

// NextID mints a new monotonic ULID for use as an event ID. Safe for
// concurrent callers.
func (s *session) NextID() string {
	s.entOnce.Lock()
	defer s.entOnce.Unlock()
	// Use the current wall-clock millisecond rebased on the session start so
	// monotonicity holds even if the clock briefly rewinds; the ULID library
	// also enforces monotonic entropy within a millisecond.
	ms := uint64(time.Now().UnixMilli())
	if ms < s.nowMS {
		ms = s.nowMS
	}
	s.nowMS = ms
	id, err := ulid.New(ms, s.entropy)
	if err != nil {
		// Overflow within a single millisecond is the only documented
		// failure mode. Step the timestamp forward and retry once.
		s.nowMS = ms + 1
		id, err = ulid.New(s.nowMS, s.entropy)
		if err != nil {
			panic(errors.Wrap(err, "session.NextID: ulid overflow"))
		}
	}
	return id.String()
}

// Close stops the fan-out worker, drains in-flight ops, and is idempotent.
func (s *session) Close() error {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		s.cancelMu.Lock()
		if s.cancelFn != nil {
			s.cancelFn()
			s.cancelFn = nil
		}
		s.cancelMu.Unlock()
		close(s.stopFanout)
		<-s.done
		s.subsMu.Lock()
		for _, sub := range s.subs {
			close(sub.ch)
		}
		s.subs = nil
		s.primary = nil
		s.subsMu.Unlock()
	})
	return nil
}

// fanout reads from incoming and writes to every subscriber. Slow
// subscribers do not block fast ones: when a subscriber's buffer is full we
// drop the event for that subscriber, increment its drop counter, and log a
// warning at most once per drop.
func (s *session) fanout() {
	defer close(s.done)
	for {
		select {
		case <-s.stopFanout:
			// Drain any pending events so subscribers see them before close.
			for {
				select {
				case ev := <-s.incoming:
					s.broadcast(ev)
				default:
					return
				}
			}
		case ev := <-s.incoming:
			s.broadcast(ev)
		}
	}
}

func (s *session) broadcast(ev Event) {
	s.subsMu.RLock()
	subs := make([]*subscriber, len(s.subs))
	copy(subs, s.subs)
	s.subsMu.RUnlock()
	for _, sub := range subs {
		select {
		case sub.ch <- ev:
		default:
			dropped := sub.dropped.Add(1)
			s.cfg.Logger.Warn("session subscriber buffer full; dropping event",
				zap.String("event_id", ev.EventID()),
				zap.String("kind", ev.Kind()),
				zap.Int64("dropped_total", dropped),
			)
		}
	}
}

// NewEventID is the package-level helper that mints a new monotonic ULID
// using a private entropy source. It is exposed so callers building events
// outside the session (e.g. tools writing through EventSink) can stamp IDs
// without holding a session reference.
func NewEventID() string {
	defaultIDMintMu.Lock()
	defer defaultIDMintMu.Unlock()
	ms := uint64(time.Now().UnixMilli())
	if ms < defaultIDLastMS {
		ms = defaultIDLastMS
	}
	defaultIDLastMS = ms
	id, err := ulid.New(ms, defaultIDEntropy)
	if err != nil {
		defaultIDLastMS = ms + 1
		id, err = ulid.New(defaultIDLastMS, defaultIDEntropy)
		if err != nil {
			panic(errors.Wrap(err, "session.NewEventID: ulid overflow"))
		}
	}
	return id.String()
}

var (
	defaultIDMintMu  sync.Mutex
	defaultIDLastMS  uint64
	defaultIDEntropy = ulid.Monotonic(rand.Reader, 0)
)

// NewBaseEvent stamps a fresh BaseEvent for the given kind/parent. It is the
// shared constructor used by tools and the loop to attach the header to a
// concrete event before emitting it.
func NewBaseEvent(kind, parentID string) BaseEvent {
	return BaseEvent{
		ID:        NewEventID(),
		ParentID:  parentID,
		EventKind: kind,
		At:        time.Now(),
	}
}
