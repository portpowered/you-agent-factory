package recordings_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

type lifecycleTestLedger struct{}

func (lifecycleTestLedger) CanonicalEvents() []factorydefinitions.FactoryEvent { return nil }

func (lifecycleTestLedger) Subscribe(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (factorydefinitions.FactoryEventStream, error) {
	return factorydefinitions.FactoryEventStream{}, nil
}

func (lifecycleTestLedger) StreamGenerationID() string { return "lifecycle-capability-test" }

func (lifecycleTestLedger) AddEventRecorder(func(factorydefinitions.FactoryEvent)) {}

func (lifecycleTestLedger) AddEventTypeRecorder(func(factorydefinitions.FactoryEventType)) {}

func (lifecycleTestLedger) AppendRecordedEvent(factorydefinitions.FactoryEvent) {}

func newTestRecordingLifecycle(t *testing.T) recordings.RecordingLifecycle {
	t.Helper()
	service, err := recordingswire.NewService(
		lifecycleTestLedger{},
		nil,
		func(string, []byte) error { return nil },
		func(string, os.FileMode) error { return nil },
		func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.Rename,
		os.ReadFile,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	lifecycle, ok := service.(recordings.RecordingLifecycle)
	if !ok {
		t.Fatal("recordings.Service does not implement recordings.RecordingLifecycle")
	}
	return lifecycle
}

// validLifecycleEvent builds a minimally realistic Factory run-request event:
// the durable JSONL replay-artifact writer (exercised by Flush) requires the
// first recorded event to carry decodable Factory snapshot config.
func validLifecycleEvent(scope recordings.LifecycleScope, sequence int64) recordings.LifecycleEvent {
	recordedAt := time.Now().UTC()
	payload := `{"factory":{"id":"lifecycle-capability-test"},"recordedAt":"` +
		recordedAt.Format(time.RFC3339Nano) + `"}`
	return recordings.LifecycleEvent{
		ID:         "event-" + recordedAt.Format("150405.000000000"),
		Sequence:   sequence,
		Scope:      scope,
		Kind:       string(recordings.FactoryEventTypeRunRequest),
		Payload:    payload,
		RecordedAt: recordedAt,
		Cursor: recordings.LifecycleEventCursor{
			StreamGenerationID: "lifecycle-capability-test",
			Sequence:           sequence,
		},
	}
}

func TestRecordingLifecycle_BeginDisabledIsInert(t *testing.T) {
	t.Parallel()
	lifecycle := newTestRecordingLifecycle(t)

	result, err := lifecycle.Begin(recordings.BeginRecordingRequest{Enabled: false})
	if err != nil {
		t.Fatalf("Begin(disabled) error = %v, want nil", err)
	}
	if result.Status.RecordingID != "" || result.Status.State != "" || len(result.Status.Failures) != 0 {
		t.Fatalf("Begin(disabled) = %#v, want zero result", result)
	}
}

func TestRecordingLifecycle_SuccessPath(t *testing.T) {
	t.Parallel()
	lifecycle := newTestRecordingLifecycle(t)

	begin, err := lifecycle.Begin(recordings.BeginRecordingRequest{
		Enabled:     true,
		RecordingID: "success-path",
		Scope:       recordings.LifecycleScope{FactorySessionID: "session-1"},
		Artifact:    "artifact://success-path",
	})
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if begin.Status.RecordingID != "success-path" {
		t.Fatalf("Begin() RecordingID = %q, want %q", begin.Status.RecordingID, "success-path")
	}
	if begin.Status.State != recordings.LifecycleStateActive {
		t.Fatalf("Begin() State = %q, want ACTIVE", begin.Status.State)
	}

	appended, err := lifecycle.AppendEvent(recordings.AppendLifecycleEventRequest{
		RecordingID: "success-path",
		Event:       validLifecycleEvent(recordings.LifecycleScope{FactorySessionID: "session-1"}, 0),
	})
	if err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}
	if appended.Status.AcceptedEvents != 1 {
		t.Fatalf("AppendEvent() AcceptedEvents = %d, want 1", appended.Status.AcceptedEvents)
	}
	if appended.Status.LastEvent == nil || appended.Status.LastEvent.Sequence != 0 {
		t.Fatalf("AppendEvent() LastEvent = %#v, want sequence 0", appended.Status.LastEvent)
	}

	flushed, err := lifecycle.Flush(recordings.FlushLifecycleRequest{RecordingID: "success-path"})
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if flushed.Status.FlushedThrough == nil || flushed.Status.FlushedThrough.Sequence != 0 {
		t.Fatalf("Flush() FlushedThrough = %#v, want sequence 0", flushed.Status.FlushedThrough)
	}

	finishedAt := time.Now().UTC()
	finished, err := lifecycle.Finish(recordings.FinishLifecycleRequest{
		RecordingID: "success-path",
		FinishedAt:  finishedAt,
	})
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if finished.Status.State != recordings.LifecycleStateFinalized {
		t.Fatalf("Finish() State = %q, want FINALIZED", finished.Status.State)
	}
	if finished.Status.FinalizedAt == nil || !finished.Status.FinalizedAt.Equal(finishedAt) {
		t.Fatalf("Finish() FinalizedAt = %v, want %v", finished.Status.FinalizedAt, finishedAt)
	}

	if err := lifecycle.Stop(recordings.StopLifecycleRequest{RecordingID: "success-path"}); err != nil {
		t.Fatalf("Stop() error = %v, want nil after finish", err)
	}
}

func TestRecordingLifecycle_InvalidInput(t *testing.T) {
	t.Parallel()
	lifecycle := newTestRecordingLifecycle(t)

	_, err := lifecycle.Begin(recordings.BeginRecordingRequest{
		Enabled:     true,
		RecordingID: "missing-target",
		Scope:       recordings.LifecycleScope{FactorySessionID: "session-1"},
	})
	var lifecycleErr *recordings.LifecycleError
	if !errors.As(err, &lifecycleErr) || lifecycleErr.Kind != recordings.LifecycleErrorInvalidTarget {
		t.Fatalf("Begin(missing target) error = %v, want LifecycleErrorInvalidTarget", err)
	}
	if !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("Begin(missing target) error does not unwrap to ErrMissingRecordingTarget: %v", err)
	}

	_, err = lifecycle.Bind(recordings.BindLifecycleRequest{
		RecordingID: "invalid-scope",
		Artifact:    "artifact://invalid-scope",
		Scope:       recordings.LifecycleScope{FactorySessionID: "   "},
	})
	if !errors.As(err, &lifecycleErr) || lifecycleErr.Kind != recordings.LifecycleErrorInvalidScope {
		t.Fatalf("Bind(invalid scope) error = %v, want LifecycleErrorInvalidScope", err)
	}

	if _, err := lifecycle.Begin(recordings.BeginRecordingRequest{
		Enabled:     true,
		RecordingID: "invalid-event",
		Scope:       recordings.LifecycleScope{FactorySessionID: "session-1"},
		Artifact:    "artifact://invalid-event",
	}); err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	_, err = lifecycle.AppendEvent(recordings.AppendLifecycleEventRequest{
		RecordingID: "invalid-event",
		Event:       recordings.LifecycleEvent{},
	})
	if !errors.As(err, &lifecycleErr) || lifecycleErr.Kind != recordings.LifecycleErrorInvalidEvent {
		t.Fatalf("AppendEvent(invalid event) error = %v, want LifecycleErrorInvalidEvent", err)
	}
}

func TestRecordingLifecycle_IdempotentBindAndConflict(t *testing.T) {
	t.Parallel()
	lifecycle := newTestRecordingLifecycle(t)

	first, err := lifecycle.Bind(recordings.BindLifecycleRequest{
		RecordingID: "idempotent-bind",
		Artifact:    "artifact://idempotent-bind",
		Scope:       recordings.LifecycleScope{FactorySessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("Bind() error = %v", err)
	}

	second, err := lifecycle.Bind(recordings.BindLifecycleRequest{
		RecordingID: "idempotent-bind",
		Artifact:    "artifact://idempotent-bind",
		Scope:       recordings.LifecycleScope{FactorySessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("Bind() repeat error = %v, want nil (idempotent)", err)
	}
	if second.Status.RecordingID != first.Status.RecordingID ||
		second.Status.Artifact != first.Status.Artifact ||
		second.Status.Scope != first.Status.Scope ||
		second.Status.State != first.Status.State ||
		second.Status.AcceptedEvents != first.Status.AcceptedEvents {
		t.Fatalf("Bind() repeat status = %#v, want unchanged %#v", second.Status, first.Status)
	}

	_, err = lifecycle.Bind(recordings.BindLifecycleRequest{
		RecordingID: "idempotent-bind",
		Artifact:    "artifact://different-artifact",
		Scope:       recordings.LifecycleScope{FactorySessionID: "session-1"},
	})
	var lifecycleErr *recordings.LifecycleError
	if !errors.As(err, &lifecycleErr) || lifecycleErr.Kind != recordings.LifecycleErrorBindingConflict {
		t.Fatalf("Bind(conflicting facts) error = %v, want LifecycleErrorBindingConflict", err)
	}
	if !errors.Is(err, recordings.ErrRecordingBindingConflict) {
		t.Fatalf("Bind(conflicting facts) error does not unwrap to ErrRecordingBindingConflict: %v", err)
	}
}

func TestRecordingLifecycle_DetachedResultsAndFailureFacts(t *testing.T) {
	t.Parallel()
	lifecycle := newTestRecordingLifecycle(t)

	if _, err := lifecycle.Bind(recordings.BindLifecycleRequest{
		RecordingID: "detached-results",
		Artifact:    "artifact://detached-results",
		Scope:       recordings.LifecycleScope{FactorySessionID: "session-1"},
	}); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}

	failed, err := lifecycle.RecordFailure(recordings.RecordLifecycleFailureRequest{
		RecordingID: "detached-results",
		Failure: recordings.LifecycleFailure{
			Code:    "boundary_failed",
			Message: "producer boundary failed",
		},
		Cause: errors.New("write failed"),
	})
	if err != nil {
		t.Fatalf("RecordFailure() error = %v", err)
	}
	if failed.Status.State != recordings.LifecycleStateFailed {
		t.Fatalf("RecordFailure() State = %q, want FAILED", failed.Status.State)
	}
	if len(failed.Status.Failures) != 1 {
		t.Fatalf("RecordFailure() Failures = %#v, want one recorded failure", failed.Status.Failures)
	}

	// Mutating the returned detached slice must not affect subsequently
	// observed status.
	failed.Status.Failures[0].Code = "mutated"

	status, err := lifecycle.Status(recordings.LifecycleStatusRequest{RecordingID: "detached-results"})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(status.Status.Failures) != 1 || status.Status.Failures[0].Code != "boundary_failed" {
		t.Fatalf("Status() Failures = %#v, want independent copy with Code=boundary_failed", status.Status.Failures)
	}
}

// narrowLifecycleFake implements only recordings.RecordingLifecycle, proving
// peers can fake the capability without implementing replay, artifact
// export, projection query, or event subscription behavior from the broader
// recordings.Service surface.
type narrowLifecycleFake struct {
	status recordings.LifecycleStatus
}

var _ recordings.RecordingLifecycle = (*narrowLifecycleFake)(nil)

func (fake *narrowLifecycleFake) Begin(request recordings.BeginRecordingRequest) (recordings.RecordingLifecycleResult, error) {
	if !request.Enabled {
		return recordings.RecordingLifecycleResult{}, nil
	}
	fake.status = recordings.LifecycleStatus{RecordingID: request.RecordingID, State: recordings.LifecycleStateActive}
	return recordings.RecordingLifecycleResult{Status: fake.status}, nil
}

func (fake *narrowLifecycleFake) Bind(recordings.BindLifecycleRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{Status: fake.status}, nil
}

func (fake *narrowLifecycleFake) AppendEvent(recordings.AppendLifecycleEventRequest) (recordings.RecordingLifecycleResult, error) {
	fake.status.AcceptedEvents++
	return recordings.RecordingLifecycleResult{Status: fake.status}, nil
}

func (fake *narrowLifecycleFake) RecordFailure(recordings.RecordLifecycleFailureRequest) (recordings.RecordingLifecycleResult, error) {
	fake.status.State = recordings.LifecycleStateFailed
	return recordings.RecordingLifecycleResult{Status: fake.status}, nil
}

func (fake *narrowLifecycleFake) Flush(recordings.FlushLifecycleRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{Status: fake.status}, nil
}

func (fake *narrowLifecycleFake) Stop(recordings.StopLifecycleRequest) error { return nil }

func (fake *narrowLifecycleFake) Finish(recordings.FinishLifecycleRequest) (recordings.RecordingLifecycleResult, error) {
	fake.status.State = recordings.LifecycleStateFinalized
	return recordings.RecordingLifecycleResult{Status: fake.status}, nil
}

func (fake *narrowLifecycleFake) Status(recordings.LifecycleStatusRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{Status: fake.status}, nil
}

func TestRecordingLifecycle_NarrowFakeConsumption(t *testing.T) {
	t.Parallel()
	var lifecycle recordings.RecordingLifecycle = &narrowLifecycleFake{}

	if _, err := lifecycle.Begin(recordings.BeginRecordingRequest{Enabled: true, RecordingID: "narrow-fake"}); err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if _, err := lifecycle.AppendEvent(recordings.AppendLifecycleEventRequest{RecordingID: "narrow-fake"}); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}
	result, err := lifecycle.Finish(recordings.FinishLifecycleRequest{RecordingID: "narrow-fake", FinishedAt: time.Now()})
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if result.Status.State != recordings.LifecycleStateFinalized {
		t.Fatalf("Finish() State = %q, want FINALIZED", result.Status.State)
	}
}
