package internal

import (
	"context"
	"encoding/json"
	"errors"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/internal/events"
	"strings"
	"testing"
	"time"
)

type runtimeRoot interface {
	recordings.Service
	recordings.RuntimeOpening
}

// TestLifecycleRuntimeRecorderRecordsRuntimeEventsAndTerminalEvent proves the
// recorder accepts Factory event vocabulary from a runtime producer and
// preserves observable identity, kind, and payload fields through to the
// portable artifact, including the terminal run-finished event it appends
// itself.
//
// The recorder no longer depends on Factory Runtime at all; that constraint is
// enforced repo-wide by the cross-service cycle ratchet (cmd/servicecyclecheck)
// rather than by an import-shape assertion here.
func TestLifecycleRuntimeRecorderRecordsRuntimeEventsAndTerminalEvent(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 7, 27, 17, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(2 * time.Minute)
	root := NewServiceWithLifecycleEffects(
		NewRuntimeLedger(nil, func() time.Time { return startedAt }, "generation", nil),
		NewProjectionService(),
		nil,
		nil,
		nil,
		nil,
		runtimeRecorderTestClock{now: startedAt},
	)
	recorder := newLifecycleRecorderForTest(t, startedAt, "runtime-root-finished.json")
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-runtime-root"}
	if err := recorder.BindRecordingLifecycle(root.(recordings.RecordingLifecycle), scope); err != nil {
		t.Fatalf("BindRecordingLifecycle: %v", err)
	}

	runtimeEvent := recordings.FactoryEvent{
		Id:   "runtime-root-work-event",
		Type: recordings.FactoryEventTypeWorkRequest,
		Context: recordings.FactoryEventContext{
			EventTime: startedAt.Add(time.Second),
		},
		Payload: []byte(`{"workId":"work-runtime-root"}`),
	}
	recorder.RecordEvent(runtimeEvent)
	if err := recorder.Finalize(finishedAt); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	status, err := root.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: recordings.RecordingID(recorder.recordingID),
	})
	if err != nil {
		t.Fatalf("QueryRecordingStatus: %v", err)
	}
	if status.Status.AcceptedEvents < 3 {
		t.Fatalf("accepted events = %d, want run-started, work request, and run-finished", status.Status.AcceptedEvents)
	}

	built, err := root.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: recordings.RecordingID(recorder.recordingID),
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifact: %v", err)
	}
	if len(built.Artifact.Events) < 3 {
		t.Fatalf("portable events = %d, want at least three", len(built.Artifact.Events))
	}

	recordedWorkEvent := built.Artifact.Events[len(built.Artifact.Events)-2]
	if recordedWorkEvent.ID != recordings.CanonicalEventID(runtimeEvent.Id) {
		t.Fatalf("recorded work event id = %q, want %q", recordedWorkEvent.ID, runtimeEvent.Id)
	}
	if recordedWorkEvent.Kind != recordings.CanonicalEventKind(runtimeEvent.Type) {
		t.Fatalf("recorded work event kind = %q, want %q", recordedWorkEvent.Kind, runtimeEvent.Type)
	}
	if !strings.Contains(recordedWorkEvent.Payload, "work-runtime-root") {
		t.Fatalf("recorded work event payload = %q, want runtime-root work id", recordedWorkEvent.Payload)
	}

	finishedEvent := built.Artifact.Events[len(built.Artifact.Events)-1]
	if finishedEvent.ID != recordings.CanonicalEventID(recordingevents.RunFinishedFactoryEventID) {
		t.Fatalf("finished event id = %q, want %q", finishedEvent.ID, recordingevents.RunFinishedFactoryEventID)
	}
	if finishedEvent.Kind != recordings.CanonicalEventKind(recordings.FactoryEventTypeRunResponse) {
		t.Fatalf("finished event kind = %q, want %q", finishedEvent.Kind, recordings.FactoryEventTypeRunResponse)
	}
	if !strings.Contains(finishedEvent.Payload, string(recordings.FactoryStateCompleted)) {
		t.Fatalf("finished event payload = %q, want completed state", finishedEvent.Payload)
	}

	assertTerminalRunPayload(t, finishedEvent.Payload, startedAt, finishedAt)
}

// assertTerminalRunPayload checks the decoded body of the terminal run event.
// The terminal event is the only record of how long a run took, so its wall
// clock has to survive into the portable artifact with both ends intact.
func assertTerminalRunPayload(t *testing.T, rawPayload string, startedAt, finishedAt time.Time) {
	t.Helper()

	var payload recordings.RunResponseEventPayload
	if err := json.Unmarshal([]byte(rawPayload), &payload); err != nil {
		t.Fatalf("decode finished event payload %q: %v", rawPayload, err)
	}
	if payload.State == nil || *payload.State != recordings.FactoryStateCompleted {
		t.Fatalf("finished event state = %#v, want completed", payload.State)
	}
	if payload.WallClock == nil {
		t.Fatalf("finished event has no wall clock, want %s..%s", startedAt, finishedAt)
	}
	if payload.WallClock.StartedAt == nil || !payload.WallClock.StartedAt.Equal(startedAt) {
		t.Fatalf("finished event started at = %v, want %s", payload.WallClock.StartedAt, startedAt)
	}
	if payload.WallClock.FinishedAt == nil || !payload.WallClock.FinishedAt.Equal(finishedAt) {
		t.Fatalf("finished event finished at = %v, want %s", payload.WallClock.FinishedAt, finishedAt)
	}
}

func TestRuntimeRootKeepsConcurrentLedgersIsolatedAndReleasesRoutes(t *testing.T) {
	service := NewRuntimeRoot(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	root, ok := service.(runtimeRoot)
	if !ok || root == nil {
		t.Fatal("NewRuntimeRoot() returned nil")
	}
	topology := runtimeOpeningTopology{}
	now := func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }

	first, err := root.OpenRuntime(context.Background(), recordings.RuntimeScopeRequest{
		Topology:         topology,
		Now:              now,
		RecordingID:      "recording-one",
		FactorySessionID: "session-one",
	})
	if err != nil {
		t.Fatalf("OpenRuntime(first): %v", err)
	}
	second, err := root.OpenRuntime(context.Background(), recordings.RuntimeScopeRequest{
		Topology:         topology,
		Now:              now,
		RecordingID:      "recording-two",
		FactorySessionID: "session-two",
	})
	if err != nil {
		t.Fatalf("OpenRuntime(second): %v", err)
	}
	if first.Ledger == second.Ledger {
		t.Fatal("OpenRuntime returned the same ledger for concurrent sessions")
	}

	first.Ledger.RecordRunRequest()
	if got := len(first.Ledger.CanonicalEvents()); got != 1 {
		t.Fatalf("first ledger events = %d, want 1", got)
	}
	if got := len(second.Ledger.CanonicalEvents()); got != 0 {
		t.Fatalf("second ledger events = %d, want 0 before its own append", got)
	}

	finishedAt := now().Add(time.Second)
	if err := first.Recorder.Finalize(finishedAt); err != nil {
		t.Fatalf("Finalize(first): %v", err)
	}
	if err := first.Recorder.Finalize(finishedAt.Add(time.Second)); err != nil {
		t.Fatalf("Finalize(first) second call: %v", err)
	}
	if _, err := root.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: "session-one"},
	}); !errors.Is(err, recordings.ErrReconnectCursorUnavailable) {
		t.Fatalf("Subscribe(closed first session) = %v, want isolated route failure", err)
	}

	second.Ledger.RecordRunRequest()
	if got := len(second.Ledger.CanonicalEvents()); got != 1 {
		t.Fatalf("second ledger events = %d, want 1", got)
	}
	if err := second.Recorder.Finalize(finishedAt); err != nil {
		t.Fatalf("Finalize(second): %v", err)
	}
}

func TestRuntimeRootActiveRecordingOwnsOpaqueScopeAndFinalizesOnce(t *testing.T) {
	root, opened, now := openActiveRuntime(t)
	if opened.Scope.IsZero() {
		t.Fatal("OpenRuntime(active) returned a zero scope")
	}
	assertActiveRecordingStarted(t, root)
	opened.Ledger.RecordRunRequest()
	scopeStatus := queryActiveScope(t, root, opened.Scope)
	appendActiveScopeEvent(t, root, opened.Scope, scopeStatus)
	finalizeActiveRuntime(t, opened.Recorder, now)
	assertActiveRecordingFinalized(t, root)
	assertActiveScopeClosed(t, root, opened.Scope)
}

func openActiveRuntime(t *testing.T) (runtimeRoot, recordings.RuntimeScopeResult, func() time.Time) {
	t.Helper()
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"id": "runtime-opening-test",
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	service := NewRuntimeRoot(
		nil,
		nil,
		nil,
		nil,
		func(
			factorydefinitions.FactorySnapshotSource,
			string,
			map[string]string,
		) (*factorydefinitions.FactorySnapshot, error) {
			return snapshot, nil
		},
		nil,
		nil,
		nil,
		nil,
	)
	root, ok := service.(runtimeRoot)
	if !ok || root == nil {
		t.Fatal("NewRuntimeRoot() did not expose runtime opening")
	}
	now := func() time.Time { return time.Unix(1_700_000_100, 0).UTC() }
	opened, err := root.OpenRuntime(context.Background(), recordings.RuntimeScopeRequest{
		Topology:         runtimeOpeningTopology{},
		Now:              now,
		RecordingID:      "recording-active",
		RecordPath:       "recording.json",
		FactorySessionID: "session-active",
	})
	if err != nil {
		t.Fatalf("OpenRuntime(active): %v", err)
	}
	return root, opened, now
}

func assertActiveRecordingStarted(t *testing.T, root runtimeRoot) {
	t.Helper()
	status, err := root.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: "recording-active",
	})
	if err != nil {
		t.Fatalf("QueryRecordingStatus(active): %v", err)
	}
	if status.Status.AcceptedEvents != 1 {
		t.Fatalf("active recording events = %d, want initial snapshot event", status.Status.AcceptedEvents)
	}
}

func queryActiveScope(
	t *testing.T,
	root runtimeRoot,
	scope recordings.RecordingScopeRef,
) recordings.QueryRecordingScopeResult {
	t.Helper()
	scopeStatus, err := root.QueryRecordingScope(context.Background(), recordings.QueryRecordingScopeRequest{
		Scope: scope,
	})
	if err != nil || scopeStatus.Status.LastEvent == nil {
		t.Fatalf("QueryRecordingScope(active) = (%#v, %v), want initial cursor", scopeStatus, err)
	}
	return scopeStatus
}

func appendActiveScopeEvent(
	t *testing.T,
	root runtimeRoot,
	scope recordings.RecordingScopeRef,
	scopeStatus recordings.QueryRecordingScopeResult,
) {
	t.Helper()
	nextSequence := scopeStatus.Status.LastEvent.Sequence + 1
	scopeEvent := scopedScopeEvent("runtime-scope-event", nextSequence, scopeStatus.Status.EventScope)
	scopeEvent.Cursor.StreamGenerationID = scopeStatus.Status.LastEvent.StreamGenerationID
	appended, err := root.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{
		Scope: scope,
		Event: scopeEvent,
	})
	if err != nil || appended.Status.AcceptedEvents != 2 {
		t.Fatalf("AppendRecordingScopeEvent(active) = (%#v, %v), want second accepted event", appended, err)
	}
}

func finalizeActiveRuntime(t *testing.T, recorder recordings.RuntimeRecorder, now func() time.Time) {
	t.Helper()
	finishedAt := now().Add(time.Second)
	if err := recorder.Finalize(finishedAt); err != nil {
		t.Fatalf("Finalize(active): %v", err)
	}
	if err := recorder.Finalize(finishedAt.Add(time.Second)); err != nil {
		t.Fatalf("Finalize(active) second call: %v", err)
	}
}

func assertActiveRecordingFinalized(t *testing.T, root runtimeRoot) {
	t.Helper()
	status, err := root.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: "recording-active",
	})
	if err != nil {
		t.Fatalf("QueryRecordingStatus(finalized): %v", err)
	}
	if status.Status.State != recordings.RecordingFinalized || status.Status.AcceptedEvents != 3 {
		t.Fatalf("finalized active recording = %#v, want FINALIZED with initial, scoped, and terminal events", status.Status)
	}
}

func assertActiveScopeClosed(t *testing.T, root runtimeRoot, scope recordings.RecordingScopeRef) {
	t.Helper()
	if _, err := root.QueryRecordingScope(context.Background(), recordings.QueryRecordingScopeRequest{
		Scope: scope,
	}); !errors.Is(err, recordings.ErrRecordingScopeClosed) {
		t.Fatalf("QueryRecordingScope(closed): %v, want closed-scope error", err)
	}
}

type runtimeOpeningTopology struct{}

func (runtimeOpeningTopology) RecordingInitialStructure(
	...factorydefinitions.RuntimeDefinitionLookup,
) recordings.InitialStructurePayload {
	return recordings.InitialStructurePayload{}
}

var _ recordings.InitialStructureSource = runtimeOpeningTopology{}
