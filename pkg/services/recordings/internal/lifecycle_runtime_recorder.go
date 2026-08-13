package internal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	canonicalpkg "github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
)

// lifecycleRuntimeRecorder adapts Factory Runtime's focused recording port to
// the session's Recordings-owned RecordingLifecycle capability without
// becoming a second lifecycle owner.
type lifecycleRuntimeRecorder struct {
	mu                 sync.Mutex
	lifecycle          recordings.RecordingLifecycle
	requestedID        recordings.LifecycleRecordingID
	recordingID        recordings.LifecycleRecordingID
	scope              recordings.CanonicalEventScope
	target             recordings.LifecycleArtifactReference
	flushInterval      time.Duration
	now                func() time.Time
	startedAt          time.Time
	streamGenerationID string
	initialEvent       factoryruntime.FactoryEvent
	seen               map[string]struct{}
	nextSequence       recordings.CanonicalEventSequence
	finalizeErr        error
	stopErr            error
	pending            []pendingRuntimeRecording
}

type pendingRuntimeRecording struct {
	event *factoryruntime.FactoryEvent
	err   error
}

var _ recordings.RuntimeRecorder = (*lifecycleRuntimeRecorder)(nil)
var _ recordings.RuntimeRecordingBinder = (*lifecycleRuntimeRecorder)(nil)

// NewLifecycleRuntimeRecorder prepares a runtime recorder whose lifecycle is
// activated only when Factory Sessions binds a RecordingLifecycle capability.
func NewLifecycleRuntimeRecorder(
	flushInterval time.Duration,
	loaded factorydefinitions.LoadedFactorySource,
	now func() time.Time,
	recordingID string,
	recordPath string,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	streamGenerationIDs ...string,
) (recordings.RuntimeRecorder, error) {
	if strings.TrimSpace(recordPath) == "" {
		return nil, nil
	}
	if now == nil {
		return nil, fmt.Errorf("recording clock is required")
	}
	if captureLoadedFactorySnapshot == nil {
		return nil, fmt.Errorf("loaded Factory snapshot capturer is required")
	}
	recordedAt := now().UTC()
	sourceDirectory := ""
	if loaded != nil {
		sourceDirectory = loaded.FactoryDir()
	}
	snapshot, err := captureLoadedFactorySnapshot(loaded, sourceDirectory, nil)
	if err != nil {
		return nil, fmt.Errorf("build replay artifact config: %w", err)
	}
	artifact, err := replayimpl.NewEventLogArtifact(
		recordedAt,
		snapshot,
		&recordings.ReplayWallClockMetadata{StartedAt: recordedAt},
		recordings.ReplayDiagnostics{},
	)
	if err != nil {
		return nil, err
	}
	streamGenerationID := strings.TrimSpace(recordingID)
	if len(streamGenerationIDs) > 0 && strings.TrimSpace(streamGenerationIDs[0]) != "" {
		streamGenerationID = strings.TrimSpace(streamGenerationIDs[0])
	}
	return &lifecycleRuntimeRecorder{
		requestedID:        recordings.LifecycleRecordingID(strings.TrimSpace(recordingID)),
		target:             recordings.LifecycleArtifactReference(recordPath),
		flushInterval:      flushInterval,
		now:                now,
		startedAt:          recordedAt,
		streamGenerationID: streamGenerationID,
		initialEvent:       artifact.Events[0],
		seen:               make(map[string]struct{}),
	}, nil
}

// BindRecordingLifecycle implements recordings.RuntimeRecordingBinder by
// binding this runtime recorder to an already-constructed RecordingLifecycle
// capability rather than the broad Recordings Service.
func (recorder *lifecycleRuntimeRecorder) BindRecordingLifecycle(
	lifecycle recordings.RecordingLifecycle,
	scope recordings.CanonicalEventScope,
) error {
	if recorder == nil {
		return nil
	}
	if lifecycle == nil {
		return fmt.Errorf("Recordings lifecycle capability is required")
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.lifecycle != nil {
		if recorder.lifecycle == lifecycle && recorder.scope == scope {
			return nil
		}
		return recordings.ErrRecordingBindingConflict
	}
	result, err := lifecycle.Begin(recordings.BeginRecordingRequest{
		Enabled:       true,
		RecordingID:   recorder.requestedID,
		Scope:         recordings.LifecycleScope{FactorySessionID: scope.FactorySessionID},
		Artifact:      recorder.target,
		FlushInterval: recorder.flushInterval,
	})
	if err != nil {
		return err
	}
	recorder.lifecycle = lifecycle
	recorder.recordingID = result.Status.RecordingID
	recorder.scope = scope
	if err := recorder.recordEventLocked(recorder.initialEvent); err != nil {
		appendErr := fmt.Errorf("record initial Factory snapshot: %w", err)
		recorder.stopErr = recorder.lifecycle.Stop(recordings.StopLifecycleRequest{
			RecordingID: recorder.recordingID,
		})
		return errors.Join(appendErr, recorder.stopErr)
	}
	for _, pending := range recorder.pending {
		if pending.event != nil {
			if err := recorder.recordEventLocked(*pending.event); err != nil {
				recorder.recordErrorLocked("producer_boundary_failed", "accept Factory event", err)
			}
		} else {
			recorder.recordErrorLocked(
				"producer_boundary_failed",
				"Factory event producer failed",
				pending.err,
			)
		}
	}
	recorder.pending = nil
	return nil
}

func (recorder *lifecycleRuntimeRecorder) Start(context.Context) {}

func (recorder *lifecycleRuntimeRecorder) Stop() {
	if recorder == nil {
		return
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.lifecycle == nil {
		return
	}
	recorder.stopErr = recorder.lifecycle.Stop(recordings.StopLifecycleRequest{
		RecordingID: recorder.recordingID,
	})
}

func (recorder *lifecycleRuntimeRecorder) RecordEvent(event factoryruntime.FactoryEvent) {
	if recorder == nil {
		return
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.lifecycle == nil {
		event.Payload = append([]byte(nil), event.Payload...)
		recorder.pending = append(recorder.pending, pendingRuntimeRecording{event: &event})
		return
	}
	if err := recorder.recordEventLocked(event); err != nil {
		recorder.recordErrorLocked("producer_boundary_failed", "accept Factory event", err)
	}
}

func (recorder *lifecycleRuntimeRecorder) recordEventLocked(
	event factoryruntime.FactoryEvent,
) error {
	if recorder.lifecycle == nil {
		return fmt.Errorf("Recordings lifecycle capability is not bound")
	}
	if _, exists := recorder.seen[event.Id]; exists {
		return nil
	}
	nextSequence := recorder.nextSequence
	if status, err := recorder.lifecycle.Status(recordings.LifecycleStatusRequest{
		RecordingID: recorder.recordingID,
	}); err == nil && status.Status.LastEvent != nil {
		currentNext := recordings.CanonicalEventSequence(status.Status.LastEvent.Sequence + 1)
		if currentNext > nextSequence {
			nextSequence = currentNext
		}
	}
	event.Context.Sequence = int(nextSequence)
	streamGenerationID := recorder.streamGenerationID
	if streamGenerationID == "" {
		streamGenerationID = string(recorder.recordingID)
	}
	canonical := canonicalpkg.CanonicalEventFromFactory(event, streamGenerationID)
	canonical.Scope = recorder.scope
	canonical.Sequence = nextSequence
	canonical.Cursor.Sequence = nextSequence
	result, err := recorder.lifecycle.AppendEvent(recordings.AppendLifecycleEventRequest{
		RecordingID: recorder.recordingID,
		Event:       lifecycleEventFromCanonical(canonical),
	})
	if err != nil {
		return err
	}
	recorder.seen[event.Id] = struct{}{}
	recorder.nextSequence = recordings.CanonicalEventSequence(result.Status.AcceptedEvents)
	return nil
}

func (recorder *lifecycleRuntimeRecorder) RecordError(err error) {
	if recorder == nil || err == nil {
		return
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.lifecycle == nil {
		recorder.pending = append(recorder.pending, pendingRuntimeRecording{err: err})
		return
	}
	recorder.recordErrorLocked("producer_boundary_failed", "Factory event producer failed", err)
}

func (recorder *lifecycleRuntimeRecorder) recordErrorLocked(code, operation string, cause error) {
	if recorder.lifecycle == nil || cause == nil {
		return
	}
	_, _ = recorder.lifecycle.RecordFailure(recordings.RecordLifecycleFailureRequest{
		RecordingID: recorder.recordingID,
		Failure: recordings.LifecycleFailure{
			Code:       code,
			Message:    operation + ": " + cause.Error(),
			RecordedAt: recorder.now().UTC(),
		},
		Cause: cause,
	})
}

func (recorder *lifecycleRuntimeRecorder) Finish(finishedAt time.Time) {
	_ = recorder.Finalize(finishedAt)
}

func (recorder *lifecycleRuntimeRecorder) Flush() error {
	if recorder == nil {
		return nil
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.lifecycle == nil {
		return fmt.Errorf("Recordings lifecycle capability is not bound")
	}
	_, err := recorder.lifecycle.Flush(recordings.FlushLifecycleRequest{
		RecordingID: recorder.recordingID,
	})
	return err
}

// Err reports the last preserved cleanup or finalize failure. Stop failures
// surface here even before Finalize runs, so startup callers that must stop
// periodic work on an early failure (before finish) can still observe and
// join the cleanup cause rather than discarding it.
func (recorder *lifecycleRuntimeRecorder) Err() error {
	if recorder == nil {
		return nil
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return errors.Join(recorder.stopErr, recorder.finalizeErr)
}

func (recorder *lifecycleRuntimeRecorder) Finalize(finishedAt time.Time) error {
	if recorder == nil {
		return nil
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.lifecycle == nil {
		return fmt.Errorf("Recordings lifecycle capability is not bound")
	}
	if recorder.finalizeErr != nil {
		return recorder.finalizeErr
	}
	if err := recorder.recordEventLocked(factoryruntime.RunFinishedFactoryEvent(recorder.startedAt, finishedAt)); err != nil {
		recorder.recordErrorLocked("terminal_metadata_failed", "record terminal Factory event", err)
	}
	_, recorder.finalizeErr = recorder.lifecycle.Finish(recordings.FinishLifecycleRequest{
		RecordingID: recorder.recordingID,
		FinishedAt:  finishedAt,
	})
	return recorder.finalizeErr
}

func lifecycleEventFromCanonical(event recordings.CanonicalEvent) recordings.LifecycleEvent {
	return recordings.LifecycleEvent{
		ID:          string(event.ID),
		Sequence:    int64(event.Sequence),
		FactoryTick: event.FactoryTick,
		Scope:       recordings.LifecycleScope{FactorySessionID: event.Scope.FactorySessionID},
		Cursor: recordings.LifecycleEventCursor{
			StreamGenerationID: event.Cursor.StreamGenerationID,
			Sequence:           int64(event.Cursor.Sequence),
		},
		RecordedAt:    event.RecordedAt,
		Kind:          string(event.Kind),
		Payload:       event.Payload,
		SourceContext: event.SourceContext,
	}
}
