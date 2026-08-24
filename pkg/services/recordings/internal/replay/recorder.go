package replay

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordingcontracts "github.com/portpowered/infinite-you/pkg/services/recordings/internal/contracts"
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
	storage         platformreplay.Storage
	path            string
	artifact        *interfaces.ReplayArtifact
	flushInterval   time.Duration
	declaredSecrets []recordingcontracts.RecordingSecret
	appender        platformreplay.Appender
	v2              bool

	mu       sync.Mutex
	flushErr error
	started  bool
	cancel   context.CancelFunc
	done     chan struct{}
	version  int64
	flushed  int64

	v2FlushMu         sync.Mutex
	v2HeaderEmitted   bool
	v2PersistedEvents int
	v2TerminalEmitted bool

	finalizeOnce sync.Once
	finalizeErr  error
}

// NewRecorder constructs a recorder for an existing artifact shell.
func NewRecorder(
	storage platformreplay.Storage,
	path string,
	artifact *interfaces.ReplayArtifact,
	flushInterval time.Duration,
	declaredSecrets ...[]recordingcontracts.RecordingSecret,
) (*Recorder, error) {
	if storage == nil {
		return nil, fmt.Errorf("replay artifact storage is required")
	}
	if path == "" {
		return nil, fmt.Errorf("replay recorder path is required")
	}
	if err := Validate(artifact); err != nil {
		return nil, err
	}
	v2 := strings.HasSuffix(strings.ToLower(strings.TrimSpace(path)), ".jsonl")
	var appender platformreplay.Appender
	if v2 {
		var ok bool
		appender, ok = storage.(platformreplay.Appender)
		if !ok {
			return nil, fmt.Errorf("replay v2 recorder requires append-capable storage")
		}
	}

	if flushInterval <= 0 {
		flushInterval = DefaultRecordFlushInterval
	}
	return &Recorder{
		storage:         storage,
		path:            path,
		artifact:        artifact,
		flushInterval:   flushInterval,
		declaredSecrets: flattenRecordingSecretGroups(declaredSecrets),
		appender:        appender,
		v2:              v2,
		version:         1,
	}, nil
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
	if r.v2 {
		return r.flushV2(false)
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

// Finalize owns the terminal recording sequence so runtime callers cannot
// accidentally reorder stop, terminal metadata, and the final flush.
func (r *Recorder) Finalize(finishedAt time.Time) error {
	if r == nil {
		return nil
	}
	r.finalizeOnce.Do(func() {
		r.Stop()
		r.Finish(finishedAt)
		r.finalizeErr = errors.Join(r.Flush(), r.Err())
	})
	return r.finalizeErr
}

func (r *Recorder) flushLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if r.v2 {
				_ = r.flushV2(true)
				continue
			}
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

func (r *Recorder) flushV2(async bool) error {
	r.v2FlushMu.Lock()
	defer r.v2FlushMu.Unlock()

	r.mu.Lock()
	artifact := cloneReplayArtifactForV2(r.artifact)
	headerPending := !r.v2HeaderEmitted
	firstPendingEvent := r.v2PersistedEvents
	terminalPending := artifact.WallClock != nil &&
		!artifact.WallClock.FinishedAt.IsZero() && !r.v2TerminalEmitted
	r.mu.Unlock()

	if !headerPending && firstPendingEvent >= len(artifact.Events) && !terminalPending {
		return nil
	}
	if err := r.flushV2Header(artifact, headerPending); err != nil {
		return r.handleV2FlushError(err, async)
	}
	if err := r.flushV2Events(artifact, firstPendingEvent); err != nil {
		return r.handleV2FlushError(err, async)
	}
	if err := r.flushV2Terminal(artifact, terminalPending); err != nil {
		return r.handleV2FlushError(err, async)
	}
	return nil
}

func (r *Recorder) flushV2Header(
	artifact *interfaces.ReplayArtifact,
	pending bool,
) error {
	if !pending {
		return nil
	}
	line, err := MarshalReplayV2Header(artifact, v2ArtifactSessionID(r.path, artifact))
	if err != nil {
		return err
	}
	if err := r.appendV2Line(line); err != nil {
		return err
	}
	r.mu.Lock()
	r.v2HeaderEmitted = true
	r.mu.Unlock()
	return nil
}

func (r *Recorder) flushV2Events(
	artifact *interfaces.ReplayArtifact,
	firstPendingEvent int,
) error {
	for index := firstPendingEvent; index < len(artifact.Events); index++ {
		line, err := MarshalReplayV2Event(artifact.Events[index])
		if err != nil {
			return err
		}
		if err := r.appendV2Line(line); err != nil {
			return err
		}
		r.mu.Lock()
		if r.v2PersistedEvents < index+1 {
			r.v2PersistedEvents = index + 1
		}
		r.mu.Unlock()
	}
	return nil
}

func (r *Recorder) flushV2Terminal(
	artifact *interfaces.ReplayArtifact,
	pending bool,
) error {
	if !pending || artifact.WallClock == nil {
		return nil
	}
	diagnostics := ReplayV2FlushDiagnostics{}
	if r.Err() != nil {
		diagnostics = ReplayV2FlushDiagnostics{
			FailureCount: 1,
			FailureCodes: []string{"flush_failed"},
		}
	}
	line, err := MarshalReplayV2Terminal(
		artifact.WallClock.FinishedAt,
		string(interfaces.FactoryStateCompleted),
		diagnostics,
	)
	if err != nil {
		return err
	}
	if err := r.appendV2Line(line); err != nil {
		return err
	}
	r.mu.Lock()
	r.v2TerminalEmitted = true
	r.mu.Unlock()
	return nil
}

func cloneReplayArtifactForV2(artifact *interfaces.ReplayArtifact) *interfaces.ReplayArtifact {
	if artifact == nil {
		return &interfaces.ReplayArtifact{}
	}
	clone := *artifact
	clone.Events = append([]interfaces.FactoryEvent(nil), artifact.Events...)
	clone.Factory = artifact.Factory.Clone()
	if artifact.WallClock != nil {
		wallClock := *artifact.WallClock
		clone.WallClock = &wallClock
	}
	return &clone
}

func v2ArtifactSessionID(path string, artifact *interfaces.ReplayArtifact) string {
	if artifact != nil && len(artifact.Events) > 0 {
		if sessionID := artifact.Events[0].Context.SessionID; sessionID != nil && strings.TrimSpace(*sessionID) != "" {
			return strings.TrimSpace(*sessionID)
		}
	}
	// A recorder can be constructed without an initial event. Keep that empty
	// recording valid under the v2 header contract while allowing a later
	// runtime event session ID to remain authoritative when one is present.
	return uuid.NewSHA1(uuid.Nil, []byte(path)).String()
}

func (r *Recorder) appendV2Line(line []byte) error {
	if r.appender == nil {
		return fmt.Errorf("replay v2 append effect is required")
	}
	if err := r.appender.AppendFile(r.path, line); err != nil {
		return fmt.Errorf("append replay artifact %q: %w", r.path, err)
	}
	return nil
}

func (r *Recorder) handleV2FlushError(err error, async bool) error {
	if err == nil {
		return nil
	}
	err = fmt.Errorf("write replay v2 artifact %q: %w", r.path, err)
	r.recordFlushError(err)
	if async {
		return nil
	}
	return err
}

func (r *Recorder) snapshotLocked() ([]byte, int64, error) {
	if r.flushed == r.version {
		return nil, 0, nil
	}
	data, err := MarshalArtifact(r.artifact, r.declaredSecrets)
	if err != nil {
		return nil, 0, err
	}
	return data, r.version, nil
}

func (r *Recorder) writeSnapshot(data []byte, version int64, async bool) error {
	if err := r.storage.WriteFile(r.path, data); err != nil {
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
