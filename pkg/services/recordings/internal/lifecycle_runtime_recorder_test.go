package internal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/internal/events"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
)

type runtimeRecorderTestClock struct{ now time.Time }

func (clock runtimeRecorderTestClock) Now() time.Time { return clock.now }

func TestLifecycleRuntimeRecorderForwardsFailuresFlushAndFinalization(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 7, 27, 15, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)
	producerErr := errors.New("producer failed")
	finalWriteErr := errors.New("final write failed")
	lifecycle := &stubRecordingLifecycle{
		beginResult: recordings.RecordingLifecycleResult{Status: recordings.LifecycleStatus{RecordingID: "runtime-recording"}},
		finishErr:   errors.Join(producerErr, finalWriteErr),
	}
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
	if err := recorder.BindRecordingLifecycle(lifecycle, scope); err != nil {
		t.Fatalf("BindRecordingLifecycle: %v", err)
	}

	if err := recorder.Flush(); err != nil {
		t.Fatalf("initial Flush: %v", err)
	}
	err := recorder.Finalize(finishedAt)
	if !errors.Is(err, producerErr) || !errors.Is(err, finalWriteErr) {
		t.Fatalf("Finalize error = %v, want producer and final-write identities", err)
	}
	assertRuntimeRecorderLifecycleRequests(t, lifecycle, recorder.recordingID, producerErr, startedAt, finishedAt)
	if err := recorder.Finalize(finishedAt); !errors.Is(err, finalWriteErr) || lifecycle.finishCalls != 1 {
		t.Fatalf("repeated Finalize = %v, finish calls = %d", err, lifecycle.finishCalls)
	}
	if !errors.Is(recorder.Err(), producerErr) || !errors.Is(recorder.Err(), finalWriteErr) {
		t.Fatalf("recorder.Err = %v, want preserved owner causes", recorder.Err())
	}
}

func assertRuntimeRecorderLifecycleRequests(t *testing.T, lifecycle *stubRecordingLifecycle,
	id recordings.LifecycleRecordingID, producerErr error, startedAt, finishedAt time.Time,
) {
	t.Helper()
	if len(lifecycle.failureRequests) != 1 {
		t.Fatalf("failure requests = %#v, want one pending producer failure", lifecycle.failureRequests)
	}
	failure := lifecycle.failureRequests[0]
	if failure.RecordingID != id || failure.Cause != producerErr ||
		failure.Failure.Code != "producer_boundary_failed" || !failure.Failure.RecordedAt.Equal(startedAt) {
		t.Fatalf("producer failure request = %#v", failure)
	}
	if lifecycle.flushRequest.RecordingID != id || lifecycle.flushCalls != 1 ||
		lifecycle.finishRequest.RecordingID != id ||
		lifecycle.finishRequest.FinishedAt != finishedAt || lifecycle.finishCalls != 1 {
		t.Fatalf("flush/finish requests = %#v / %#v", lifecycle.flushRequest, lifecycle.finishRequest)
	}
}

func TestReplayRecordingSnapshotWriterPreservesReplayCompatibility(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 7, 27, 16, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)
	path := filepath.Join(t.TempDir(), "recording.jsonl")
	storage := platformreplay.NewLocal(runtime.GOOS)
	factory, err := factorydefinitions.NewFactorySnapshot(map[string]any{"id": "factory-1"})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := replayimpl.NewEventLogArtifact(startedAt, factory, nil, recordings.ReplayDiagnostics{})
	if err != nil {
		t.Fatal(err)
	}
	sessionID := "00000000-0000-4000-8000-000000000005"
	events := []factorydefinitions.FactoryEvent{initial.Events[0], recordingevents.RunFinishedFactoryEvent(startedAt, finishedAt)}
	snapshot := recordings.RecordingSnapshot{
		Status:             recordings.RecordingStatusFacts{State: recordings.RecordingFinalized, FinalizedAt: &finishedAt},
		CanonicalSessionID: sessionID,
	}
	for index, event := range events {
		event.Context.Sequence = index
		event.Context.SessionID = &sessionID
		snapshot.Events = append(snapshot.Events, canonical.CanonicalEventFromFactory(event, "generation"))
	}
	writer := NewReplayRecordingSnapshotWriter(storage.WriteFile, storage.AppendFile, nil)
	if err := writer(path, snapshot); err != nil {
		t.Fatalf("write replay-compatible snapshot: %v", err)
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
	beginResult     recordings.RecordingLifecycleResult
	beginErr        error
	beginRequest    recordings.BeginRecordingRequest
	appendErr       error
	appendCalls     int
	stopCalls       int
	stopErr         error
	appendRequests  []recordings.AppendLifecycleEventRequest
	failureRequests []recordings.RecordLifecycleFailureRequest
	flushRequest    recordings.FlushLifecycleRequest
	flushCalls      int
	finishRequest   recordings.FinishLifecycleRequest
	finishCalls     int
	finishErr       error
}

func (s *stubRecordingLifecycle) Begin(request recordings.BeginRecordingRequest) (recordings.RecordingLifecycleResult, error) {
	s.beginRequest = request
	return s.beginResult, s.beginErr
}

func (s *stubRecordingLifecycle) Bind(recordings.BindLifecycleRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{}, nil
}

func (s *stubRecordingLifecycle) AppendEvent(request recordings.AppendLifecycleEventRequest) (recordings.RecordingLifecycleResult, error) {
	s.appendCalls++
	s.appendRequests = append(s.appendRequests, request)
	return recordings.RecordingLifecycleResult{Status: recordings.LifecycleStatus{AcceptedEvents: s.appendCalls}}, s.appendErr
}

func (s *stubRecordingLifecycle) RecordFailure(request recordings.RecordLifecycleFailureRequest) (recordings.RecordingLifecycleResult, error) {
	s.failureRequests = append(s.failureRequests, request)
	return recordings.RecordingLifecycleResult{}, nil
}

func (s *stubRecordingLifecycle) Flush(request recordings.FlushLifecycleRequest) (recordings.RecordingLifecycleResult, error) {
	s.flushRequest = request
	s.flushCalls++
	return recordings.RecordingLifecycleResult{}, nil
}

func (s *stubRecordingLifecycle) Stop(recordings.StopLifecycleRequest) error {
	s.stopCalls++
	return s.stopErr
}

func (s *stubRecordingLifecycle) Finish(request recordings.FinishLifecycleRequest) (recordings.RecordingLifecycleResult, error) {
	s.finishRequest = request
	s.finishCalls++
	return recordings.RecordingLifecycleResult{}, s.finishErr
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

func TestRuntimeScopeKeepsPublicSessionSelectorSeparateFromCanonicalIdentity(t *testing.T) {
	t.Parallel()

	got := requestScope(recordings.RuntimeScopeRequest{
		FactorySessionID:   "~default",
		CanonicalSessionID: "550e8400-e29b-41d4-a716-446655440000",
	})
	if got.FactorySessionID != "~default" {
		t.Fatalf("runtime event scope = %q, want public selector ~default", got.FactorySessionID)
	}
}

func TestLifecycleRuntimeRecorderStopAndIdempotentRecordEvent(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 7, 27, 16, 0, 0, 0, time.UTC)
	lifecycle := &stubRecordingLifecycle{beginResult: recordings.RecordingLifecycleResult{Status: recordings.LifecycleStatus{RecordingID: "stop-recording"}}}
	recorder := newLifecycleRecorderForTest(t, startedAt, "recording-stop.json")
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-stop"}
	if err := recorder.BindRecordingLifecycle(lifecycle, scope); err != nil {
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
	if lifecycle.stopCalls != 1 || lifecycle.finishCalls != 1 || len(lifecycle.appendRequests) != 3 {
		t.Fatalf("stop/finish/appends = %d/%d/%d, want 1/1/3", lifecycle.stopCalls, lifecycle.finishCalls, len(lifecycle.appendRequests))
	}
	if lifecycle.appendRequests[1].Event.ID != event.Id || lifecycle.appendRequests[1].Event.Sequence != 1 {
		t.Fatalf("deduplicated event request = %#v", lifecycle.appendRequests[1])
	}
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
	recordedAt := time.Date(2026, 9, 19, 18, 0, 0, 0, time.UTC)
	factorySnapshot := factorydefinitions.FactorySnapshot(`{"name":"recorded-factory"}`)
	fullInput := recordings.LoadReplayInputResult{
		ArtifactDigest: "sha256:source-recording-digest",
		Legacy: &factorydefinitions.ReplayArtifact{
			RecordedAt: recordedAt,
			Factory:    &factorySnapshot,
			Events:     []factorydefinitions.FactoryEvent{{Id: "resume-event"}},
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
	definitionDigest := sha256.Sum256([]byte(factorySnapshot))
	if got.RecoveryMetadata.SourceRecordingID != fullInput.ArtifactDigest ||
		got.RecoveryMetadata.RecordedDefinitionID != "sha256:"+hex.EncodeToString(definitionDigest[:]) ||
		!got.RecoveryMetadata.PreviousRecordedAt.Equal(recordedAt) {
		t.Fatalf("resume recovery metadata = %#v, want source digest, recorded definition digest, and recorded time", got.RecoveryMetadata)
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
