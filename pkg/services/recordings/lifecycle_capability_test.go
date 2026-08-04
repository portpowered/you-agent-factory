package recordings_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
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
	return newTestRecordingLifecycleWithWriter(t, func(string, []byte) error { return nil })
}

func newTestRecordingLifecycleWithWriter(
	t *testing.T,
	writeFile func(string, []byte) error,
) recordings.RecordingLifecycle {
	t.Helper()
	service, err := recordingswire.NewService(
		lifecycleTestLedger{},
		nil,
		writeFile,
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

// lifecycleWorkEvent builds a subsequent canonical event sharing the same
// stream generation as validLifecycleEvent, for tests that append more than
// one event and need strictly increasing sequences.
func lifecycleWorkEvent(scope recordings.LifecycleScope, sequence int64) recordings.LifecycleEvent {
	recordedAt := time.Unix(1_700_000_000+sequence, 0).UTC()
	return recordings.LifecycleEvent{
		ID:         fmt.Sprintf("work-event-%d", sequence),
		Sequence:   sequence,
		Scope:      scope,
		Kind:       "WORK_REQUEST",
		Payload:    "{}",
		RecordedAt: recordedAt,
		Cursor: recordings.LifecycleEventCursor{
			StreamGenerationID: "lifecycle-capability-test",
			Sequence:           sequence,
		},
	}
}

// capturingLifecycleWriter records every successfully written payload and can
// be armed to fail the very next write, so tests can prove a flush retry
// after a write failure persists durably without inventing or reordering
// events.
type capturingLifecycleWriter struct {
	mu       sync.Mutex
	writes   [][]byte
	failNext bool
}

func (writer *capturingLifecycleWriter) write(_ string, payload []byte) error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.failNext {
		writer.failNext = false
		return errors.New("storage unavailable")
	}
	writer.writes = append(writer.writes, append([]byte(nil), payload...))
	return nil
}

func (writer *capturingLifecycleWriter) armFailure() {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	writer.failNext = true
}

func (writer *capturingLifecycleWriter) snapshot() [][]byte {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return append([][]byte(nil), writer.writes...)
}

func newCapturingRecordingLifecycle(t *testing.T) (*capturingLifecycleWriter, recordings.RecordingLifecycle) {
	t.Helper()
	writer := &capturingLifecycleWriter{}
	return writer, newTestRecordingLifecycleWithWriter(t, writer.write)
}

func TestRecordingLifecycle_MultipleAppendsFlushesRetryAndPersistedOrder(t *testing.T) {
	t.Parallel()

	writer, lifecycle := newCapturingRecordingLifecycle(t)
	scope := recordings.LifecycleScope{FactorySessionID: "session-multi"}
	if _, err := lifecycle.Begin(recordings.BeginRecordingRequest{
		Enabled:     true,
		RecordingID: "multi",
		Scope:       scope,
		Artifact:    "artifact://multi",
	}); err != nil {
		t.Fatalf("Begin() error = %v", err)
	}

	first := validLifecycleEvent(scope, 0)
	appendMultiLifecycleEvent(t, lifecycle, first)
	flushed1 := assertMultiLifecycleFlushSequence(t, lifecycle, 0)
	assertRepeatedMultiLifecycleFlushIsStable(t, lifecycle)

	second := lifecycleWorkEvent(scope, 1)
	appendMultiLifecycleEvent(t, lifecycle, second)
	assertMultiLifecycleFlushRetrySucceedsAfterFailure(t, writer, lifecycle)

	third := lifecycleWorkEvent(scope, 2)
	appendMultiLifecycleEvent(t, lifecycle, third)
	flushed3 := assertMultiLifecycleFlushSequence(t, lifecycle, 2)
	if flushed3.RecordingID != flushed1.RecordingID {
		t.Fatalf(
			"RecordingID changed across repeated flushes: %q vs %q",
			flushed3.RecordingID, flushed1.RecordingID,
		)
	}

	assertPersistedMultiLifecycleEventOrder(t, writer, []string{first.ID, second.ID, third.ID})
}

func appendMultiLifecycleEvent(t *testing.T, lifecycle recordings.RecordingLifecycle, event recordings.LifecycleEvent) {
	t.Helper()
	if _, err := lifecycle.AppendEvent(recordings.AppendLifecycleEventRequest{
		RecordingID: "multi", Event: event,
	}); err != nil {
		t.Fatalf("AppendEvent(%s) error = %v", event.ID, err)
	}
}

func assertMultiLifecycleFlushSequence(
	t *testing.T, lifecycle recordings.RecordingLifecycle, wantSequence int64,
) recordings.LifecycleStatus {
	t.Helper()
	flushed, err := lifecycle.Flush(recordings.FlushLifecycleRequest{RecordingID: "multi"})
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if flushed.Status.FlushedThrough == nil || flushed.Status.FlushedThrough.Sequence != wantSequence {
		t.Fatalf("Flush() FlushedThrough = %#v, want sequence %d", flushed.Status.FlushedThrough, wantSequence)
	}
	return flushed.Status
}

// assertRepeatedMultiLifecycleFlushIsStable proves repeating flush without a
// new append succeeds without inventing or reordering events.
func assertRepeatedMultiLifecycleFlushIsStable(t *testing.T, lifecycle recordings.RecordingLifecycle) {
	t.Helper()
	repeat, err := lifecycle.Flush(recordings.FlushLifecycleRequest{RecordingID: "multi"})
	if err != nil {
		t.Fatalf("Flush(repeat) error = %v", err)
	}
	if repeat.Status.FlushedThrough == nil || repeat.Status.FlushedThrough.Sequence != 0 ||
		repeat.Status.AcceptedEvents != 1 {
		t.Fatalf("Flush(repeat) status = %#v, want unchanged sequence 0 / 1 accepted event", repeat.Status)
	}
}

func assertMultiLifecycleFlushRetrySucceedsAfterFailure(
	t *testing.T, writer *capturingLifecycleWriter, lifecycle recordings.RecordingLifecycle,
) {
	t.Helper()
	writer.armFailure()
	if _, err := lifecycle.Flush(recordings.FlushLifecycleRequest{RecordingID: "multi"}); err == nil {
		t.Fatal("Flush() error = nil, want write failure")
	}
	afterFailure, err := lifecycle.Status(recordings.LifecycleStatusRequest{RecordingID: "multi"})
	if err != nil {
		t.Fatalf("Status() after failed flush error = %v", err)
	}
	if afterFailure.Status.FlushedThrough == nil || afterFailure.Status.FlushedThrough.Sequence != 0 {
		t.Fatalf(
			"Status() after failed flush FlushedThrough = %#v, want unchanged sequence 0",
			afterFailure.Status.FlushedThrough,
		)
	}
	assertMultiLifecycleFlushSequence(t, lifecycle, 1)
}

func assertPersistedMultiLifecycleEventOrder(t *testing.T, writer *capturingLifecycleWriter, wantIDs []string) {
	t.Helper()
	writes := writer.snapshot()
	if len(writes) != 3 {
		t.Fatalf("persisted writes = %d, want 3 (the failed write must not persist)", len(writes))
	}
	var lastArtifact recordings.ReplayArtifact
	if err := json.Unmarshal(writes[2], &lastArtifact); err != nil {
		t.Fatalf("decode persisted artifact: %v", err)
	}
	if len(lastArtifact.Events) != len(wantIDs) {
		t.Fatalf("persisted event count = %d, want %d (%#v)", len(lastArtifact.Events), len(wantIDs), lastArtifact.Events)
	}
	for index, wantID := range wantIDs {
		if lastArtifact.Events[index].Id != wantID {
			t.Fatalf(
				"persisted event[%d].Id = %q, want %q (persisted order = %#v)",
				index, lastArtifact.Events[index].Id, wantID, lastArtifact.Events,
			)
		}
		if lastArtifact.Events[index].Context.Sequence != index {
			t.Fatalf(
				"persisted event[%d].Context.Sequence = %d, want %d",
				index, lastArtifact.Events[index].Context.Sequence, index,
			)
		}
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
