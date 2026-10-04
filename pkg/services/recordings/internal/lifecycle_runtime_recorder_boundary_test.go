package internal

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/internal/events"
	canonicalledger "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger"
	projectionquerywire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query/wire"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type runtimeRoot interface {
	recordings.Service
	recordings.RuntimeScopeService
}

// TestLifecycleRuntimeRecorderRecordsRuntimeEventsAndTerminalEvent proves the
// recorder accepts Factory event vocabulary from a runtime producer and
// preserves identity, kind, and payload fields in the lifecycle append
// requests, including the terminal run-finished event it appends itself.
//
// The recorder no longer depends on Factory Runtime at all; that constraint is
// enforced repo-wide by the cross-service cycle ratchet (cmd/servicecyclecheck)
// rather than by an import-shape assertion here.
func TestLifecycleRuntimeRecorderRecordsRuntimeEventsAndTerminalEvent(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 7, 27, 17, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(2 * time.Minute)
	lifecycle := &stubRecordingLifecycle{beginResult: recordings.RecordingLifecycleResult{Status: recordings.LifecycleStatus{RecordingID: "runtime-recording"}}}
	recorder := newLifecycleRecorderForTest(t, startedAt, "runtime-root-finished.json")
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-runtime-root"}
	if err := recorder.BindRecordingLifecycle(lifecycle, scope); err != nil {
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

	if len(lifecycle.appendRequests) != 3 {
		t.Fatalf("append requests = %d, want initial, work request, and terminal events", len(lifecycle.appendRequests))
	}
	assertRuntimeRecorderAppendRequests(t, lifecycle.appendRequests, recorder.recordingID, scope)

	recordedWorkEvent := lifecycle.appendRequests[1].Event
	if recordedWorkEvent.ID != runtimeEvent.Id {
		t.Fatalf("recorded work event id = %q, want %q", recordedWorkEvent.ID, runtimeEvent.Id)
	}
	if recordedWorkEvent.Kind != string(runtimeEvent.Type) {
		t.Fatalf("recorded work event kind = %q, want %q", recordedWorkEvent.Kind, runtimeEvent.Type)
	}
	if recordedWorkEvent.Payload != string(runtimeEvent.Payload) || recordedWorkEvent.RecordedAt != runtimeEvent.Context.EventTime {
		t.Fatalf("recorded work event payload = %q, want runtime-root work id", recordedWorkEvent.Payload)
	}

	finishedEvent := lifecycle.appendRequests[2].Event
	if finishedEvent.ID != recordingevents.RunFinishedFactoryEventID {
		t.Fatalf("finished event id = %q, want %q", finishedEvent.ID, recordingevents.RunFinishedFactoryEventID)
	}
	if finishedEvent.Kind != string(recordings.FactoryEventTypeRunResponse) {
		t.Fatalf("finished event kind = %q, want %q", finishedEvent.Kind, recordings.FactoryEventTypeRunResponse)
	}
	if !strings.Contains(finishedEvent.Payload, string(recordings.FactoryStateCompleted)) {
		t.Fatalf("finished event payload = %q, want completed state", finishedEvent.Payload)
	}

	assertTerminalRunPayload(t, finishedEvent.Payload, startedAt, finishedAt)
}

func assertRuntimeRecorderAppendRequests(t *testing.T, requests []recordings.AppendLifecycleEventRequest,
	id recordings.LifecycleRecordingID, scope recordings.CanonicalEventScope,
) {
	t.Helper()
	for index, request := range requests {
		if request.RecordingID != id || request.Event.Scope.FactorySessionID != scope.FactorySessionID ||
			request.Event.Sequence != int64(index) || request.Event.Cursor.Sequence != int64(index) ||
			request.Event.Cursor.StreamGenerationID != string(id) {
			t.Fatalf("append request %d = %#v", index, request)
		}
	}
}

func TestRuntimeOpeningRedactsDeclaredFactoryPathsInFinalArtifact(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"credential": "runtime-secret",
		"items":      []any{map[string]any{"token": "item-secret", "label": "visible"}},
		"a/b":        map[string]any{"~key": "escaped-secret"},
		"scalar":     "leaf",
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	root := NewRuntimeRoot(
		nil, nil, nil, nil,
		func(
			factorydefinitions.FactorySnapshotSource,
			string,
			map[string]string,
		) (*factorydefinitions.FactorySnapshot, error) {
			return snapshot, nil
		},
		nil, nil, nil, nil,
		runtimeRecorderTestClock{now: startedAt},
	)
	opening, ok := root.(recordings.RuntimeScopeService)
	if !ok {
		t.Fatal("Recordings root does not expose RuntimeScopeService")
	}
	opened, err := opening.OpenRuntime(context.Background(), recordings.RuntimeScopeRequest{
		Topology:         runtimeOpeningTopology{},
		LoadedFactory:    invocationSensitiveLoadedFactory{pointers: []string{"/credential", "/items/0/token", "/a~1b/~0key", "/missing", "/items/not-an-index", "/scalar/child", "/a~", "/a~2"}},
		Now:              func() time.Time { return startedAt },
		RecordingID:      "runtime-provenance",
		RecordPath:       "runtime-provenance.json",
		FactorySessionID: "session-runtime-provenance",
	})
	if err != nil {
		t.Fatalf("OpenRuntime: %v", err)
	}
	if err := opened.Recorder.Finalize(startedAt.Add(time.Minute)); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	built, err := root.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: "runtime-provenance",
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifact: %v", err)
	}
	if len(built.Artifact.Events) < 2 {
		t.Fatalf("final artifact events = %d, want initial and terminal events", len(built.Artifact.Events))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(built.Artifact.Events[0].Payload), &payload); err != nil {
		t.Fatalf("decode initial event payload: %v", err)
	}
	factory, ok := payload["factory"].(map[string]any)
	if !ok {
		t.Fatalf("initial event factory payload = %#v", payload["factory"])
	}
	assertRedactedRuntimeValue(t, factory["credential"], "credential")
	items, ok := factory["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("initial event items = %#v, want one item", factory["items"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("initial event item = %#v", items[0])
	}
	assertRedactedRuntimeValue(t, item["token"], "items/0/token")
	escaped, ok := factory["a/b"].(map[string]any)
	if !ok {
		t.Fatalf("initial event escaped object = %#v", factory["a/b"])
	}
	assertRedactedRuntimeValue(t, escaped["~key"], "a/b/~key")
	if factory["scalar"] != "leaf" {
		t.Fatalf("unclassified scalar = %#v, want leaf", factory["scalar"])
	}
	if _, exists := factory["missing"]; exists {
		t.Fatalf("missing declared path changed the Factory payload: %#v", factory["missing"])
	}
}

func assertRedactedRuntimeValue(t *testing.T, value any, path string) {
	t.Helper()
	marker, ok := value.(map[string]any)
	if !ok || marker["redacted"] != true || marker["provenance"] != string(recordings.RecordingSecretProvenanceDeclared) {
		t.Fatalf("redacted %s value = %#v, want typed redaction marker", path, value)
	}
	if _, hasOriginalValue := marker["value"]; hasOriginalValue {
		t.Fatalf("redacted %s value retained original content: %#v", path, marker)
	}
}

type invocationSensitiveLoadedFactory struct {
	pointers []string
}

func (source invocationSensitiveLoadedFactory) FactoryDir() string { return "" }

func (source invocationSensitiveLoadedFactory) FactoryConfig() *factorydefinitions.FactoryConfig {
	return nil
}

func (source invocationSensitiveLoadedFactory) RuntimeBaseDir() string { return "" }

func (source invocationSensitiveLoadedFactory) Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	return nil, false
}

func (source invocationSensitiveLoadedFactory) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}

func (source invocationSensitiveLoadedFactory) InvocationSensitiveJSONPointers() []string {
	return append([]string(nil), source.pointers...)
}

var _ factorydefinitions.LoadedFactorySource = invocationSensitiveLoadedFactory{}

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

func TestProjectionOwnerReconstructsCanonicalFactoryWorldStateFromFixture(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	projection := projectionquerywire.NewService()

	factorySnapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"name": "roundtrip-factory",
		"workTypes": []any{
			map[string]any{
				"name": "task",
				"states": []any{
					map[string]any{"name": "init", "type": "INITIAL"},
					map[string]any{"name": "done", "type": "TERMINAL"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}

	const workID = "work-roundtrip"
	const traceID = "trace-roundtrip"
	restored, err := projection.ReconstructFactoryWorldState(
		roundtripFixtureEvents(t, base, factorySnapshot),
		1,
	)
	if err != nil {
		t.Fatalf("ReconstructCanonicalFactoryWorldState: %v", err)
	}
	item, ok := restored.WorkItemsByID[workID]
	if !ok {
		t.Fatalf("reconstructed Work = %#v, want %s", restored.WorkItemsByID, workID)
	}
	if item.State != "init" || item.TraceID != traceID {
		t.Fatalf("reconstructed Work state/trace = (%q, %q), want init/%s", item.State, item.TraceID, traceID)
	}
	if got := restored.PlaceOccupancyByID["task:init"].WorkItemIDs; len(got) != 1 || got[0] != workID {
		t.Fatalf("reconstructed Work occupancy = %#v, want [%s]", got, workID)
	}
}

func roundtripFixtureEvents(t *testing.T, base time.Time, factorySnapshot *factorydefinitions.FactorySnapshot) []factorydefinitions.FactoryEvent {
	t.Helper()
	requestID := "request-roundtrip"
	traceID := "trace-roundtrip"
	workID := "work-roundtrip"
	workIDs := []string{workID}
	traceIDs := []string{traceID}
	return []factorydefinitions.FactoryEvent{
		{Id: "run-roundtrip", Type: factorydefinitions.FactoryEventTypeRunRequest,
			Payload: mustMarshalRoundtripTest(t, factorydefinitions.RunRequestEventPayload{Factory: factorySnapshot, RecordedAt: base}),
			Context: factorydefinitions.FactoryEventContext{Tick: 0, Sequence: 0, EventTime: base}},
		{Id: "work-roundtrip", Type: factorydefinitions.FactoryEventTypeWorkRequest,
			Payload: mustMarshalRoundtripTest(t, work.WorkRequestEventPayload{Type: work.WorkRequestTypeFactoryRequestBatch,
				Works: []work.WorkRequestEventWork{{Name: "Roundtrip work", WorkID: workID, WorkTypeID: "task",
					State: &work.WorkEventState{Name: "init"}, TraceID: traceID,
					Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "fixture payload"}}}}}),
			Context: factorydefinitions.FactoryEventContext{Tick: 1, Sequence: 1, EventTime: base.Add(time.Second),
				RequestID: &requestID, TraceIDs: &traceIDs, WorkIDs: &workIDs}},
	}
}

func mustMarshalRoundtripTest(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal roundtrip fixture: %v", err)
	}
	return encoded
}

func TestRuntimeRootKeepsConcurrentLedgersIsolatedAndReleasesRoutes(t *testing.T) {
	t.Parallel()
	router := newRuntimeLedgerRouter(nil)
	root := NewCombinedService(router, nil, nil, nil, nil, nil, nil,
		staticRecordingClock{}, logging.NoopLogger{}, router, nil, nil, nil, nil).(*combinedService)
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
	t.Cleanup(func() { _ = first.Recorder.Finalize(now().Add(time.Second)) })
	second, err := root.OpenRuntime(context.Background(), recordings.RuntimeScopeRequest{
		Topology:         topology,
		Now:              now,
		RecordingID:      "recording-two",
		FactorySessionID: "session-two",
	})
	if err != nil {
		t.Fatalf("OpenRuntime(second): %v", err)
	}
	t.Cleanup(func() { _ = second.Recorder.Finalize(now().Add(time.Second)) })
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
	if _, err := router.Subscribe(context.Background(), nil, factorydefinitions.FactoryEventReconnectScope{
		SessionID: "session-one",
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

type runtimeOpeningProjection struct {
	recordings.ProjectionService
	events []recordings.FactoryEvent
	tick   int
	result recordings.FactoryWorldState
	err    error
}

func (projection *runtimeOpeningProjection) ReconstructFactoryWorldState(events []recordings.FactoryEvent, tick int) (recordings.FactoryWorldState, error) {
	projection.events = events
	projection.tick = tick
	return projection.result, projection.err
}

func TestRuntimeRootForwardsCanonicalWorldStateAndProjectionCause(t *testing.T) {
	t.Parallel()
	ownerErr := errors.New("projection failed")
	projection := &runtimeOpeningProjection{result: recordings.FactoryWorldState{Tick: 7}, err: ownerErr}
	root := NewCombinedService(nil, projection, nil, nil, nil, nil, nil,
		staticRecordingClock{}, logging.NoopLogger{}, nil, nil, nil, nil, nil).(*combinedService)
	events := []recordings.FactoryEvent{{Id: "selected-event", Type: recordings.FactoryEventTypeWorkRequest}}
	state, err := root.ReconstructCanonicalFactoryWorldState(events, 7)
	if !errors.Is(err, ownerErr) || !reflect.DeepEqual(state, projection.result) ||
		!reflect.DeepEqual(projection.events, events) || projection.tick != 7 {
		t.Fatalf("projection forwarding = (%#v, %v), inputs = (%#v, %d)", state, err, projection.events, projection.tick)
	}
	projection.err = nil
	state, err = root.ReconstructCanonicalFactoryWorldState(events, 7)
	if err != nil || !reflect.DeepEqual(state, projection.result) {
		t.Fatalf("projection success = (%#v, %v), want supplied result", state, err)
	}
}

func TestRuntimeRootActiveRecordingOwnsOpaqueScopeAndFinalizesOnce(t *testing.T) {
	t.Parallel()
	owner := &activeRuntimeLifecycle{status: recordings.RecordingStatusFacts{
		RecordingID: "recording-active", Scope: recordings.CanonicalEventScope{FactorySessionID: "session-active"},
	}}
	canonical := &activeRuntimeCanonical{}
	root, opened, now := openActiveRuntime(t, owner, canonical)
	t.Cleanup(func() { _ = opened.Recorder.Finalize(now().Add(time.Second)) })
	if opened.Scope.IsZero() {
		t.Fatal("OpenRuntime(active) returned a zero scope")
	}
	if owner.started.RecordingID != owner.status.RecordingID || owner.started.Scope != owner.status.Scope ||
		!owner.started.Enabled || owner.started.Target.Artifact != "recording.json" || len(owner.events) != 1 {
		t.Fatalf("active start = %#v, events = %#v, want selected binding and initial event", owner.started, owner.events)
	}
	initialCursor := owner.events[0].Event.Cursor
	owner.status.LastEvent = &initialCursor
	owner.status.AcceptedEvents = 1
	opened.Ledger.RecordRunRequest()
	scopeStatus := queryActiveScope(t, root, opened.Scope)
	assertActiveRuntimeQueryDetached(t, scopeStatus, owner)
	scopeStatus = queryActiveScope(t, root, opened.Scope)
	owner.status.AcceptedEvents = 2
	appendActiveScopeEvent(t, root, opened.Scope, scopeStatus)
	if canonical.event.ID != "runtime-scope-event" || len(owner.events) != 2 ||
		owner.events[1].RecordingID != owner.status.RecordingID || owner.events[1].Event != canonical.event {
		t.Fatalf("canonical event = %#v, lifecycle requests = %#v, want selected event forwarded", canonical.event, owner.events)
	}
	finalizeActiveRuntime(t, opened.Recorder, now)
	if len(owner.finishes) != 1 || owner.finishes[0].RecordingID != owner.status.RecordingID ||
		!owner.finishes[0].FinishedAt.Equal(now().Add(time.Second)) || len(owner.events) != 3 {
		t.Fatalf("finish = %#v, events = %#v, want one selected finalization", owner.finishes, owner.events)
	}
	assertTerminalRunPayload(t, owner.events[2].Event.Payload, now(), now().Add(time.Second))
	assertActiveScopeClosed(t, root, opened.Scope)
}

func assertActiveRuntimeQueryDetached(t *testing.T, result recordings.QueryRecordingScopeResult, owner *activeRuntimeLifecycle) {
	t.Helper()
	status := owner.status
	if owner.statusRequest.RecordingID != status.RecordingID {
		t.Fatalf("status request = %#v, want selected recording %q", owner.statusRequest, status.RecordingID)
	}
	if result.Status.EventScope != status.Scope || result.Status.AcceptedEvents != status.AcceptedEvents ||
		*result.Status.LastEvent != *status.LastEvent {
		t.Fatalf("scope status = %#v, want selected owner's supplied status %#v", result, status)
	}
	// Query returns detached cursors, leaving the owner's supplied facts intact.
	want := *status.LastEvent
	result.Status.LastEvent.Sequence++
	if *status.LastEvent != want {
		t.Fatal("scope query cursor aliases the owner status")
	}
}

// Completed-owner doubles capture the root adapter's requests. Native lifecycle
// and public functional witnesses own terminal state and persistence policy.
type activeRuntimeLifecycle struct {
	recordinglifecycle.Service
	status        recordings.RecordingStatusFacts
	started       recordings.StartRecordingRequest
	events        []recordings.RecordRecordingEventRequest
	finishes      []recordings.FinishRecordingRequest
	statusRequest recordings.RecordingStatusRequest
}

func (owner *activeRuntimeLifecycle) StartRecording(request recordings.StartRecordingRequest) (recordings.StartRecordingResult, error) {
	owner.started = request
	return recordings.StartRecordingResult{Enabled: true, Status: owner.status}, nil
}

func (owner *activeRuntimeLifecycle) RecordRecordingEvent(request recordings.RecordRecordingEventRequest) (recordings.RecordRecordingEventResult, error) {
	owner.events = append(owner.events, request)
	return recordings.RecordRecordingEventResult{Status: owner.status}, nil
}

func (owner *activeRuntimeLifecycle) QueryRecordingStatus(request recordings.RecordingStatusRequest) (recordings.RecordingStatusResult, error) {
	owner.statusRequest = request
	return recordings.RecordingStatusResult{Status: owner.status}, nil
}

func (owner *activeRuntimeLifecycle) FinishRecording(request recordings.FinishRecordingRequest) (recordings.FinishRecordingResult, error) {
	owner.finishes = append(owner.finishes, request)
	return recordings.FinishRecordingResult{Status: owner.status}, nil
}

type activeRuntimeCanonical struct {
	canonicalledger.Service
	event recordings.CanonicalEvent
}

func (owner *activeRuntimeCanonical) AppendWithValidation(request recordings.AppendRecordedEventRequest, validate func(recordings.CanonicalEvent) error) (recordings.AppendRecordedEventResult, error) {
	owner.event = request.Event
	if err := validate(request.Event); err != nil {
		return recordings.AppendRecordedEventResult{}, err
	}
	return recordings.AppendRecordedEventResult{Event: request.Event}, nil
}

func openActiveRuntime(t *testing.T, owner *activeRuntimeLifecycle, canonical *activeRuntimeCanonical) (runtimeRoot, recordings.RuntimeScopeResult, func() time.Time) {
	t.Helper()
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"id": "runtime-opening-test",
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	now := func() time.Time { return time.Unix(1_700_000_100, 0).UTC() }
	service := NewCombinedService(nil, nil, owner, nil, nil, canonical, nil,
		runtimeRecorderTestClock{now: now()}, logging.NoopLogger{}, newRuntimeLedgerRouter(now),
		func(
			factorydefinitions.FactorySnapshotSource,
			string,
			map[string]string,
		) (*factorydefinitions.FactorySnapshot, error) {
			return snapshot, nil
		},
		nil, nil, nil,
	)
	root, ok := service.(runtimeRoot)
	if !ok || root == nil {
		t.Fatal("NewCombinedService() did not expose runtime opening")
	}
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
