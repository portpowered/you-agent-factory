package replay

import (
	"context"
	"fmt"
	"sync"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
)

const (
	// DefaultRecordFlushInterval is the default cadence for flushing dirty
	// replay recordings while a run is active. Finalization still performs a
	// synchronous flush before normal shutdown returns.
	DefaultRecordFlushInterval = 250 * time.Millisecond
)

// Recorder streams replay artifact updates to disk while a factory run is
// active. It owns artifact mutation so service hooks do not share unsynchronized
// mutable recording state.
type Recorder struct {
	path          string
	artifact      *interfaces.ReplayArtifact
	flushInterval time.Duration

	mu       sync.Mutex
	flushErr error
	started  bool
	cancel   context.CancelFunc
	done     chan struct{}
	version  int64
	flushed  int64
}

// RecorderOption configures a replay Recorder.
type RecorderOption func(*Recorder)

// WithFlushInterval overrides how often dirty artifacts are flushed while a
// recorder is running. Non-positive values use DefaultRecordFlushInterval.
func WithFlushInterval(interval time.Duration) RecorderOption {
	return func(r *Recorder) {
		if interval > 0 {
			r.flushInterval = interval
		}
	}
}

// NewRecorder constructs a recorder for an existing artifact shell.
func NewRecorder(path string, artifact *interfaces.ReplayArtifact, opts ...RecorderOption) (*Recorder, error) {
	if path == "" {
		return nil, fmt.Errorf("replay recorder path is required")
	}
	if err := Validate(artifact); err != nil {
		return nil, err
	}

	recorder := &Recorder{
		path:          path,
		artifact:      artifact,
		flushInterval: DefaultRecordFlushInterval,
		version:       1,
	}
	for _, opt := range opts {
		opt(recorder)
	}
	return recorder, nil
}

// Start begins periodic streaming flushes until ctx is canceled. Start is
// idempotent so callers can safely invoke it before every run.
func (r *Recorder) Start(ctx context.Context) {
	if r == nil {
		return
	}

	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	r.started = true
	r.cancel = cancel
	r.done = make(chan struct{})
	interval := r.flushInterval
	done := r.done
	r.mu.Unlock()

	go func() {
		defer close(done)
		r.flushLoop(loopCtx, interval)
	}()
}

// Stop cancels and joins the recorder's periodic flush loop. It is safe to
// call more than once and does not perform the caller-owned final flush.
func (r *Recorder) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	cancel := r.cancel
	done := r.done
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// RecordEvent appends a canonical Factory-owned event and marks the artifact
// for streaming. Producer-specific conversion belongs at the caller boundary.
func (r *Recorder) RecordEvent(event interfaces.FactoryEvent) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if replayEventIndexByID(r.artifact.Events, event.Id) >= 0 {
		return
	}
	event.Payload = append([]byte(nil), event.Payload...)
	event.SchemaVersion = interfaces.FactoryEventSchemaVersionV1
	event.Context.Sequence = len(r.artifact.Events)
	r.artifact.Events = append(r.artifact.Events, event)
	r.version++
}

// RecordError retains an event-producer boundary failure for Err and Flush
// diagnostics without mutating the canonical replay event history.
func (r *Recorder) RecordError(err error) {
	if r == nil || err == nil {
		return
	}
	r.recordFlushError(err)
}

// Finish records final wall-clock metadata before the caller performs its final
// flush.
func (r *Recorder) Finish(finishedAt time.Time) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.artifact.WallClock == nil {
		r.artifact.WallClock = &interfaces.ReplayWallClockMetadata{}
	}
	r.artifact.WallClock.FinishedAt = finishedAt
	if len(r.artifact.Events) > 0 && replayEventIndexByID(r.artifact.Events, replayRunFinishedEventID) < 0 {
		finished := runFinishedEvent(finishedAt, r.artifact.WallClock, r.artifact.Diagnostics)
		finished.Context.Tick = lastEventTick(r.artifact.Events)
		finished.Context.Sequence = len(r.artifact.Events)
		r.artifact.Events = append(r.artifact.Events, finished)
	}
	r.version++
}

func replayEventIndexByID(events []interfaces.FactoryEvent, id string) int {
	for i := range events {
		if events[i].Id == id {
			return i
		}
	}
	return -1
}

// Flush writes the artifact if it has changed since the previous successful
// flush.
func (r *Recorder) Flush() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	data, version, err := r.snapshotLocked()
	r.mu.Unlock()
	if err != nil || version == 0 {
		return err
	}
	return r.writeSnapshot(data, version, false)
}

// Err returns the first asynchronous flush error, if any.
func (r *Recorder) Err() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.flushErr
}

func (r *Recorder) flushLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.mu.Lock()
			data, version, err := r.snapshotLocked()
			r.mu.Unlock()
			if err == nil && version > 0 {
				_ = r.writeSnapshot(data, version, true)
			} else if err != nil {
				r.recordFlushError(err)
			}
		}
	}
}

func (r *Recorder) snapshotLocked() ([]byte, int64, error) {
	if r.flushed == r.version {
		return nil, 0, nil
	}
	data, err := MarshalArtifact(r.artifact)
	if err != nil {
		return nil, 0, err
	}
	return data, r.version, nil
}

func (r *Recorder) writeSnapshot(data []byte, version int64, async bool) error {
	if err := platformreplay.WriteFile(r.path, data); err != nil {
		err = fmt.Errorf("write replay artifact %q: %w", r.path, err)
		r.recordFlushError(err)
		if async {
			return nil
		}
		return err
	}

	r.mu.Lock()
	if r.flushed < version {
		r.flushed = version
	}
	r.mu.Unlock()
	return nil
}

func (r *Recorder) recordFlushError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.flushErr == nil {
		r.flushErr = err
	}
}

func lastEventTick(events []interfaces.FactoryEvent) int {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Context.Tick != 0 {
			return events[i].Context.Tick
		}
	}
	return 0
}
