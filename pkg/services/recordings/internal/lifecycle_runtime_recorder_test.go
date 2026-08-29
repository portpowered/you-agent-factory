package internal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
)

type runtimeRecorderTestClock struct{ now time.Time }

func (clock runtimeRecorderTestClock) Now() time.Time { return clock.now }

func TestLifecycleRuntimeRecorderUsesComposedRootForBindingFailuresAndFinalization(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 7, 27, 15, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)
	producerErr := errors.New("producer failed")
	finalWriteErr := errors.New("final write failed")
	writeCount := 0
	writer := func(string, recordings.RecordingSnapshot) error {
		writeCount++
		if writeCount == 2 {
			return finalWriteErr
		}
		return nil
	}
	root := NewServiceWithLifecycleEffects(
		NewRuntimeLedger(nil, func() time.Time { return startedAt }, "generation", nil),
		NewProjectionService(),
		nil,
		writer,
		nil,
		nil,
		runtimeRecorderTestClock{now: startedAt},
	)
	recorder := newLifecycleRecorderForTest(t, startedAt, "recording.json")
	recorder.RecordError(producerErr)
	recorder.RecordEvent(factorydefinitions.FactoryEvent{
		Id:   "runtime-event",
		Type: factorydefinitions.FactoryEventTypeWorkRequest,
		Context: factorydefinitions.FactoryEventContext{
			EventTime: startedAt.Add(time.Second),
		},
		Payload: []byte(`{"workId":"work-1"}`),
	})
	scope := recordings.CanonicalEventScope{FactorySessionID: "~default"}
	if err := recorder.BindRecordingLifecycle(root.(recordings.RecordingLifecycle), scope); err != nil {
		t.Fatalf("BindRecordingLifecycle: %v", err)
	}

	if err := recorder.Flush(); err != nil {
		t.Fatalf("initial Flush: %v", err)
	}
	err := recorder.Finalize(finishedAt)
	if !errors.Is(err, producerErr) || !errors.Is(err, finalWriteErr) {
		t.Fatalf("Finalize error = %v, want producer and final-write identities", err)
	}

	status, err := root.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: recordings.RecordingID(recorder.recordingID),
	})
	if err != nil {
		t.Fatalf("QueryRecordingStatus: %v", err)
	}
	assertFailedRuntimeRecordingStatus(t, status.Status, scope, finishedAt)
}

func assertFailedRuntimeRecordingStatus(
	t *testing.T,
	status recordings.RecordingStatusFacts,
	scope recordings.CanonicalEventScope,
	finishedAt time.Time,
) {
	t.Helper()
	if status.Artifact != "recording.json" ||
		status.Scope != scope ||
		status.State != recordings.RecordingFailed ||
		status.AcceptedEvents != 3 ||
		len(status.Failures) != 2 ||
		status.Failures[0].Code != "producer_boundary_failed" ||
		status.Failures[1].Code != "final_flush_failed" ||
		status.FinalizedAt == nil ||
		!status.FinalizedAt.Equal(finishedAt) {
		t.Fatalf("composed lifecycle status = %#v", status)
	}
}

func TestReplayRecordingSnapshotWriterPreservesReplayCompatibility(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 7, 27, 16, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)
	path := filepath.Join(t.TempDir(), "recording.jsonl")
	storage := platformreplay.NewLocal(runtime.GOOS)
	root := NewServiceWithLifecycleEffects(
		NewRuntimeLedger(nil, func() time.Time { return startedAt }, "generation", nil),
		NewProjectionService(),
		nil,
		NewReplayRecordingSnapshotWriter(storage.WriteFile, storage.AppendFile),
		nil,
		nil,
		runtimeRecorderTestClock{now: startedAt},
	)
	recorder := newLifecycleRecorderForTest(t, startedAt, path)
	if err := recorder.BindRecordingLifecycle(root.(recordings.RecordingLifecycle), recordings.CanonicalEventScope{
		FactorySessionID: "00000000-0000-4000-8000-000000000005",
	}); err != nil {
		t.Fatalf("BindRecordingLifecycle: %v", err)
	}
	if err := recorder.Finalize(finishedAt); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	artifact, err := replayimpl.Load(
		storage,
		path,
		func(data []byte) (*factorydefinitions.FactorySnapshot, error) {
			return factorydefinitions.NewFactorySnapshot(
				map[string]any{"id": "factory-from-decoder"},
			)
		},
	)
	if err != nil {
		payload, _ := os.ReadFile(path)
		t.Fatalf("Load replay-compatible lifecycle recording: %v\n%s", err, payload)
	}
	if artifact.WallClock == nil ||
		!artifact.WallClock.StartedAt.Equal(startedAt) ||
		!artifact.WallClock.FinishedAt.Equal(finishedAt) ||
		len(artifact.Events) != 2 {
		t.Fatalf(
			"replay artifact wall clock = %#v, events = %d",
			artifact.WallClock,
			len(artifact.Events),
		)
	}
	for index, event := range artifact.Events {
		if event.Context.SessionID == nil || *event.Context.SessionID != "00000000-0000-4000-8000-000000000005" {
			t.Fatalf("replay artifact event %d session id = %v, want canonical UUID", index, event.Context.SessionID)
		}
	}
}

func newLifecycleRecorderForTest(
	t *testing.T,
	startedAt time.Time,
	path string,
) *lifecycleRuntimeRecorder {
	return newLifecycleRecorderWithIDForTest(t, startedAt, "", path)
}

func newLifecycleRecorderWithIDForTest(
	t *testing.T,
	startedAt time.Time,
	recordingID string,
	path string,
) *lifecycleRuntimeRecorder {
	t.Helper()
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"id": "factory-1",
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	value, err := NewLifecycleRuntimeRecorder(
		time.Hour,
		nil,
		func() time.Time { return startedAt },
		recordingID,
		path,
		func(
			factorydefinitions.FactorySnapshotSource,
			string,
			map[string]string,
		) (*factorydefinitions.FactorySnapshot, error) {
			return snapshot, nil
		},
	)
	if err != nil {
		t.Fatalf("NewLifecycleRuntimeRecorder: %v", err)
	}
	recorder, ok := value.(*lifecycleRuntimeRecorder)
	if !ok {
		t.Fatalf("recorder type = %T", value)
	}
	return recorder
}

// stubRecordingLifecycle is a controllable recordings.RecordingLifecycle fake
// used to prove BindRecordingLifecycle stops (joins) periodic lifecycle work
// when the initial Factory snapshot append fails after Begin has already
// started it, rather than leaking it.
type stubRecordingLifecycle struct {
	beginResult  recordings.RecordingLifecycleResult
	beginErr     error
	beginRequest recordings.BeginRecordingRequest
	appendErr    error
	appendCalls  int
	stopCalls    int
	stopErr      error
}

func (s *stubRecordingLifecycle) Begin(request recordings.BeginRecordingRequest) (recordings.RecordingLifecycleResult, error) {
	s.beginRequest = request
	return s.beginResult, s.beginErr
}

func (s *stubRecordingLifecycle) Bind(recordings.BindLifecycleRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{}, nil
}

func (s *stubRecordingLifecycle) AppendEvent(recordings.AppendLifecycleEventRequest) (recordings.RecordingLifecycleResult, error) {
	s.appendCalls++
	return recordings.RecordingLifecycleResult{}, s.appendErr
}

func (s *stubRecordingLifecycle) RecordFailure(recordings.RecordLifecycleFailureRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{}, nil
}

func (s *stubRecordingLifecycle) Flush(recordings.FlushLifecycleRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{}, nil
}

func (s *stubRecordingLifecycle) Stop(recordings.StopLifecycleRequest) error {
	s.stopCalls++
	return s.stopErr
}

func (s *stubRecordingLifecycle) Finish(recordings.FinishLifecycleRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{}, nil
}

func (s *stubRecordingLifecycle) Status(recordings.LifecycleStatusRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{}, nil
}

var _ recordings.RecordingLifecycle = (*stubRecordingLifecycle)(nil)

func TestLifecycleRuntimeRecorderBindStopsPeriodicWorkOnInitialAppendFailure(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	appendErr := errors.New("initial snapshot append failed")
	stopErr := errors.New("stop cleanup failed")
	lifecycle := &stubRecordingLifecycle{
		beginResult: recordings.RecordingLifecycleResult{
			Status: recordings.LifecycleStatus{RecordingID: "leaked-recording"},
		},
		appendErr: appendErr,
		stopErr:   stopErr,
	}
	recorder := newLifecycleRecorderForTest(t, startedAt, "leak-check.json")
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-leak-check"}

	err := recorder.BindRecordingLifecycle(lifecycle, scope)
	if err == nil {
		t.Fatal("BindRecordingLifecycle() error = nil, want initial append failure")
	}
	if !errors.Is(err, appendErr) {
		t.Fatalf("BindRecordingLifecycle() error = %v, want it to preserve the initiating append cause", err)
	}
	if !errors.Is(err, stopErr) {
		t.Fatalf("BindRecordingLifecycle() error = %v, want it to preserve the cleanup stop cause", err)
	}
	if lifecycle.appendCalls != 1 {
		t.Fatalf("append calls = %d, want exactly 1", lifecycle.appendCalls)
	}
	if lifecycle.stopCalls != 1 {
		t.Fatalf(
			"lifecycle Stop calls = %d, want exactly 1 (periodic work must be stopped/joined on partial-bind failure, not leaked)",
			lifecycle.stopCalls,
		)
	}
	if recorderErr := recorder.Err(); !errors.Is(recorderErr, stopErr) {
		t.Fatalf("recorder.Err() = %v, want it to observe the preserved stop cleanup cause", recorderErr)
	}
}

func TestLifecycleRuntimeRecorderBindsConcreteRecordingIdentity(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 8, 4, 13, 0, 0, 0, time.UTC)
	const recordingID recordings.LifecycleRecordingID = "runtime-recording-identity"
	lifecycle := &stubRecordingLifecycle{
		beginResult: recordings.RecordingLifecycleResult{
			Status: recordings.LifecycleStatus{RecordingID: recordingID},
		},
	}
	recorder := newLifecycleRecorderWithIDForTest(
		t, startedAt, string(recordingID), "identity-check.json",
	)
	recorder.canonicalSessionID = "550e8400-e29b-41d4-a716-446655440000"

	if err := recorder.BindRecordingLifecycle(lifecycle, recordings.CanonicalEventScope{
		FactorySessionID: "session-identity-check",
	}); err != nil {
		t.Fatalf("BindRecordingLifecycle: %v", err)
	}
	if lifecycle.beginRequest.RecordingID != recordingID {
		t.Fatalf("Begin recording identity = %q, want %q", lifecycle.beginRequest.RecordingID, recordingID)
	}
	if lifecycle.beginRequest.CanonicalSessionID != recorder.canonicalSessionID ||
		lifecycle.beginRequest.ReportedSessionID != "session-identity-check" {
		t.Fatalf("Begin recording identities = canonical=%q reported=%q, want canonical=%q reported=%q",
			lifecycle.beginRequest.CanonicalSessionID,
			lifecycle.beginRequest.ReportedSessionID,
			recorder.canonicalSessionID,
			"session-identity-check",
		)
	}
	if recorder.recordingID != recordingID {
		t.Fatalf("bound recording identity = %q, want %q", recorder.recordingID, recordingID)
	}
}

func TestLifecycleRuntimeRecorderStopAndIdempotentRecordEvent(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 7, 27, 16, 0, 0, 0, time.UTC)
	root := NewServiceWithLifecycleEffects(
		NewRuntimeLedger(nil, func() time.Time { return startedAt }, "generation", nil),
		NewProjectionService(),
		nil,
		nil,
		nil,
		nil,
		runtimeRecorderTestClock{now: startedAt},
	)
	recorder := newLifecycleRecorderForTest(t, startedAt, "recording-stop.json")
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-stop"}
	if err := recorder.BindRecordingLifecycle(root.(recordings.RecordingLifecycle), scope); err != nil {
		t.Fatalf("BindRecordingLifecycle: %v", err)
	}
	event := factorydefinitions.FactoryEvent{
		Id:   "dup-event",
		Type: factorydefinitions.FactoryEventTypeWorkRequest,
		Context: factorydefinitions.FactoryEventContext{
			EventTime: startedAt,
		},
		Payload: []byte(`{}`),
	}
	recorder.Start(context.Background())
	recorder.RecordEvent(event)
	recorder.RecordEvent(event)
	recorder.Stop()
	recorder.Finish(startedAt.Add(time.Minute))
}

const (
	resumeSourceSessionID = "550e8400-e29b-41d4-a716-446655440000"
	otherSourceSessionID  = "7d9d3fb4-6bc9-4df5-a67f-0f504f8ea3ba"
)

func TestResumeSourceCanonicalSessionIDUsesRecordingMetadataOverAliases(t *testing.T) {
	metadata := recordings.ReplayInputMetadata{FactorySessionID: resumeSourceSessionID}
	alias := "~default"
	input := recordings.LoadReplayInputResult{
		Metadata: &metadata,
		Legacy: &factorydefinitions.ReplayArtifact{
			Events: []factorydefinitions.FactoryEvent{
				{Context: factorydefinitions.FactoryEventContext{SessionID: &alias}},
			},
		},
	}

	got, err := resumeSourceCanonicalSessionID(input)
	if err != nil {
		t.Fatalf("resumeSourceCanonicalSessionID() error = %v", err)
	}
	if got != resumeSourceSessionID {
		t.Fatalf("source canonical session ID = %q, want %q", got, resumeSourceSessionID)
	}
}

func TestResumeSourceCanonicalSessionIDReadsLegacyEventIdentity(t *testing.T) {
	sessionID := resumeSourceSessionID
	input := recordings.LoadReplayInputResult{
		Legacy: &factorydefinitions.ReplayArtifact{
			Events: []factorydefinitions.FactoryEvent{
				{Context: factorydefinitions.FactoryEventContext{SessionID: &sessionID}},
				{Context: factorydefinitions.FactoryEventContext{SessionID: &sessionID}},
			},
		},
	}

	got, err := resumeSourceCanonicalSessionID(input)
	if err != nil {
		t.Fatalf("resumeSourceCanonicalSessionID() error = %v", err)
	}
	if got != resumeSourceSessionID {
		t.Fatalf("source canonical session ID = %q, want %q", got, resumeSourceSessionID)
	}
}

func TestResumeSourceCanonicalSessionIDDoesNotPromoteAlias(t *testing.T) {
	alias := "~default"
	input := recordings.LoadReplayInputResult{
		Legacy: &factorydefinitions.ReplayArtifact{
			Events: []factorydefinitions.FactoryEvent{
				{Context: factorydefinitions.FactoryEventContext{SessionID: &alias}},
			},
		},
	}

	got, err := resumeSourceCanonicalSessionID(input)
	if err != nil {
		t.Fatalf("resumeSourceCanonicalSessionID() error = %v", err)
	}
	if got != "" {
		t.Fatalf("source canonical session ID = %q, want empty for alias-only input", got)
	}
}

func TestResumeSourceCanonicalSessionIDUsesAutomaticRecordingPath(t *testing.T) {
	alias := "~default"
	input := recordings.LoadReplayInputResult{
		Legacy: &factorydefinitions.ReplayArtifact{
			Events: []factorydefinitions.FactoryEvent{
				{Context: factorydefinitions.FactoryEventContext{SessionID: &alias}},
			},
		},
	}
	path := filepath.Join(
		t.TempDir(), ".you-agent-factory", "recordings", "2026", "08", "29",
		resumeSourceSessionID+".json",
	)

	got, err := resumeSourceCanonicalSessionIDForPath(input, path)
	if err != nil {
		t.Fatalf("resumeSourceCanonicalSessionIDForPath() error = %v", err)
	}
	if got != resumeSourceSessionID {
		t.Fatalf("source canonical session ID = %q, want %q from automatic path", got, resumeSourceSessionID)
	}
}

func TestResumeSourceCanonicalSessionIDIgnoresUUIDInExplicitPath(t *testing.T) {
	alias := "~default"
	input := recordings.LoadReplayInputResult{
		Legacy: &factorydefinitions.ReplayArtifact{
			Events: []factorydefinitions.FactoryEvent{
				{Context: factorydefinitions.FactoryEventContext{SessionID: &alias}},
			},
		},
	}

	got, err := resumeSourceCanonicalSessionIDForPath(input, filepath.Join(t.TempDir(), resumeSourceSessionID+".json"))
	if err != nil {
		t.Fatalf("resumeSourceCanonicalSessionIDForPath() error = %v", err)
	}
	if got != "" {
		t.Fatalf("source canonical session ID = %q, want empty for arbitrary explicit path", got)
	}
}

func TestResumeSourceCanonicalSessionIDRejectsConflictingIdentities(t *testing.T) {
	first := resumeSourceSessionID
	second := otherSourceSessionID
	input := recordings.LoadReplayInputResult{
		Legacy: &factorydefinitions.ReplayArtifact{
			Events: []factorydefinitions.FactoryEvent{
				{Context: factorydefinitions.FactoryEventContext{SessionID: &first}},
				{Context: factorydefinitions.FactoryEventContext{SessionID: &second}},
			},
		},
	}

	_, err := resumeSourceCanonicalSessionID(input)
	if err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("resumeSourceCanonicalSessionID() error = %v, want conflicting identity error", err)
	}
}

func TestResumeSourceCanonicalSessionIDAllowsAliasAlongsideCanonicalEventIdentity(t *testing.T) {
	alias := "~default"
	canonical := resumeSourceSessionID
	input := recordings.LoadReplayInputResult{
		Legacy: &factorydefinitions.ReplayArtifact{
			Events: []factorydefinitions.FactoryEvent{
				{Context: factorydefinitions.FactoryEventContext{SessionID: &alias}},
				{Context: factorydefinitions.FactoryEventContext{SessionID: &canonical}},
			},
		},
	}

	got, err := resumeSourceCanonicalSessionID(input)
	if err != nil {
		t.Fatalf("resumeSourceCanonicalSessionID() error = %v", err)
	}
	if got != canonical {
		t.Fatalf("source canonical session ID = %q, want %q", got, canonical)
	}
}

func TestLoadResumeInputCarriesV2MetadataIdentityWithoutChangingHistory(t *testing.T) {
	fullInput := recordings.LoadReplayInputResult{
		Legacy: &factorydefinitions.ReplayArtifact{
			Events: []factorydefinitions.FactoryEvent{{Id: "resume-event"}},
		},
	}
	metadataInput := recordings.LoadReplayInputResult{
		Metadata: &recordings.ReplayInputMetadata{FactorySessionID: resumeSourceSessionID},
	}
	loader := replayInputLoaderFunc(func(request recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
		if request.MetadataOnly {
			return metadataInput, nil
		}
		return fullInput, nil
	})
	service := &combinedService{replayInputs: loader}

	got, err := service.LoadResumeInput(recordings.LoadResumeInputRequest{Path: "source.recording.jsonl"})
	if err != nil {
		t.Fatalf("LoadResumeInput() error = %v", err)
	}
	if got.SourceCanonicalSessionID != resumeSourceSessionID {
		t.Fatalf("source canonical session ID = %q, want %q", got.SourceCanonicalSessionID, resumeSourceSessionID)
	}
	if got.Input.Legacy == nil || len(got.Input.Legacy.Events) != 1 || got.Input.Legacy.Events[0].Id != "resume-event" {
		t.Fatalf("resume history = %#v, want unchanged legacy history", got.Input.Legacy)
	}
}

func TestLoadResumeInputUsesAutomaticPathIdentityForLegacyAliasHistory(t *testing.T) {
	alias := "~default"
	fullInput := recordings.LoadReplayInputResult{
		Legacy: &factorydefinitions.ReplayArtifact{
			Events: []factorydefinitions.FactoryEvent{{
				Id:      "resume-event",
				Context: factorydefinitions.FactoryEventContext{SessionID: &alias},
			}},
		},
	}
	loader := replayInputLoaderFunc(func(request recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
		if request.MetadataOnly {
			return recordings.LoadReplayInputResult{
				Metadata: &recordings.ReplayInputMetadata{FactorySessionID: alias},
			}, nil
		}
		return fullInput, nil
	})
	service := &combinedService{replayInputs: loader}
	path := filepath.Join(
		t.TempDir(), ".you-agent-factory", "recordings", "2026", "08", "29",
		resumeSourceSessionID+".json",
	)

	got, err := service.LoadResumeInput(recordings.LoadResumeInputRequest{Path: path})
	if err != nil {
		t.Fatalf("LoadResumeInput() error = %v", err)
	}
	if got.SourceCanonicalSessionID != resumeSourceSessionID {
		t.Fatalf("source canonical session ID = %q, want %q from automatic path", got.SourceCanonicalSessionID, resumeSourceSessionID)
	}
}

type replayInputLoaderFunc func(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error)

func (loader replayInputLoaderFunc) LoadReplayInput(
	request recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	return loader(request)
}
