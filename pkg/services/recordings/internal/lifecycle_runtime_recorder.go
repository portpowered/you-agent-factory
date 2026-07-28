package internal

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	canonicalpkg "github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	"github.com/portpowered/infinite-you/pkg/services/recordings/replay"
)

// lifecycleRuntimeRecorder adapts Factory Runtime's focused recording port to
// the session's Recordings root without becoming a second lifecycle owner.
type lifecycleRuntimeRecorder struct {
	mu            sync.Mutex
	service       recordings.Service
	recordingID   recordings.RecordingID
	scope         recordings.CanonicalEventScope
	target        recordings.RecordingArtifactReference
	flushInterval time.Duration
	now           func() time.Time
	startedAt     time.Time
	initialEvent  factoryruntime.FactoryEvent
	seen          map[string]struct{}
	nextSequence  recordings.CanonicalEventSequence
	finalizeErr   error
	pending       []pendingRuntimeRecording
}

type pendingRuntimeRecording struct {
	event *factoryruntime.FactoryEvent
	err   error
}

var _ recordings.RuntimeRecorder = (*lifecycleRuntimeRecorder)(nil)

// NewLifecycleRuntimeRecorder prepares a runtime recorder whose lifecycle is
// activated only when Factory Sessions binds the composed Recordings root.
func NewLifecycleRuntimeRecorder(
	flushInterval time.Duration,
	loaded factorydefinitions.LoadedFactorySource,
	now func() time.Time,
	recordPath string,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
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
	artifact, err := replay.NewEventLogArtifact(
		recordedAt,
		snapshot,
		&recordings.ReplayWallClockMetadata{StartedAt: recordedAt},
		recordings.ReplayDiagnostics{},
	)
	if err != nil {
		return nil, err
	}
	return &lifecycleRuntimeRecorder{
		target:        recordings.RecordingArtifactReference(recordPath),
		flushInterval: flushInterval,
		now:           now,
		startedAt:     recordedAt,
		initialEvent:  artifact.Events[0],
		seen:          make(map[string]struct{}),
	}, nil
}

func (recorder *lifecycleRuntimeRecorder) BindRecordingService(
	service recordings.Service,
	scope recordings.CanonicalEventScope,
) error {
	if recorder == nil {
		return nil
	}
	if service == nil {
		return fmt.Errorf("Recordings root service is required")
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.service != nil {
		if recorder.service == service && recorder.scope == scope {
			return nil
		}
		return recordings.ErrRecordingBindingConflict
	}
	started, err := service.StartRecording(recordings.StartRecordingRequest{
		Enabled: true,
		Scope:   scope,
		Target: recordings.RecordingTargetRequest{
			Artifact: recorder.target,
		},
		FlushInterval: recorder.flushInterval,
	})
	if err != nil {
		return err
	}
	recorder.service = service
	recorder.recordingID = started.Status.RecordingID
	recorder.scope = scope
	if err := recorder.recordEventLocked(recorder.initialEvent); err != nil {
		return fmt.Errorf("record initial Factory snapshot: %w", err)
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
	if recorder.service == nil {
		return
	}
	_, _ = recorder.service.StopRecording(recordings.StopRecordingRequest{
		RecordingID: recorder.recordingID,
	})
}

func (recorder *lifecycleRuntimeRecorder) RecordEvent(event factoryruntime.FactoryEvent) {
	if recorder == nil {
		return
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.service == nil {
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
	if recorder.service == nil {
		return fmt.Errorf("Recordings root service is not bound")
	}
	if _, exists := recorder.seen[event.Id]; exists {
		return nil
	}
	event.Context.Sequence = int(recorder.nextSequence)
	canonical := canonicalpkg.CanonicalEventFromFactory(event, string(recorder.recordingID))
	canonical.Scope = recorder.scope
	canonical.Sequence = recorder.nextSequence
	canonical.Cursor.Sequence = recorder.nextSequence
	result, err := recorder.service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: recorder.recordingID,
		Event:       canonical,
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
	if recorder.service == nil {
		recorder.pending = append(recorder.pending, pendingRuntimeRecording{err: err})
		return
	}
	recorder.recordErrorLocked("producer_boundary_failed", "Factory event producer failed", err)
}

func (recorder *lifecycleRuntimeRecorder) recordErrorLocked(code, operation string, cause error) {
	if recorder.service == nil || cause == nil {
		return
	}
	_, _ = recorder.service.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: recorder.recordingID,
		Failure: recordings.RecordingFailure{
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
	if recorder.service == nil {
		return fmt.Errorf("Recordings root service is not bound")
	}
	_, err := recorder.service.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: recorder.recordingID,
	})
	return err
}

func (recorder *lifecycleRuntimeRecorder) Err() error {
	if recorder == nil {
		return nil
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return recorder.finalizeErr
}

func (recorder *lifecycleRuntimeRecorder) Finalize(finishedAt time.Time) error {
	if recorder == nil {
		return nil
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.service == nil {
		return fmt.Errorf("Recordings root service is not bound")
	}
	if recorder.finalizeErr != nil {
		return recorder.finalizeErr
	}
	if err := recorder.recordEventLocked(factoryruntime.RunFinishedFactoryEvent(recorder.startedAt, finishedAt)); err != nil {
		recorder.recordErrorLocked("terminal_metadata_failed", "record terminal Factory event", err)
	}
	_, recorder.finalizeErr = recorder.service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: recorder.recordingID,
		FinishedAt:  finishedAt,
	})
	return recorder.finalizeErr
}
