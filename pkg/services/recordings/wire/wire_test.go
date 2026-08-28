package wire_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type stubLedger struct{}

func (stubLedger) CanonicalEvents() []factorydefinitions.FactoryEvent { return nil }

func (stubLedger) Subscribe(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (factorydefinitions.FactoryEventStream, error) {
	return factorydefinitions.FactoryEventStream{}, nil
}

func (stubLedger) StreamGenerationID() string { return "wire-test-generation" }

func (stubLedger) AddEventRecorder(func(factorydefinitions.FactoryEvent)) {}

func (stubLedger) AddEventTypeRecorder(func(factorydefinitions.FactoryEventType)) {}

func (stubLedger) AppendRecordedEvent(factorydefinitions.FactoryEvent) {}

type recordingLedger struct {
	subscribeCalls int
}

func (ledger *recordingLedger) CanonicalEvents() []factorydefinitions.FactoryEvent {
	return nil
}

func (ledger *recordingLedger) Subscribe(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (factorydefinitions.FactoryEventStream, error) {
	ledger.subscribeCalls++
	panic("ledger subscription started during inert construction")
}

func (ledger *recordingLedger) StreamGenerationID() string { return "wire-test-generation" }

func (ledger *recordingLedger) AddEventRecorder(func(factorydefinitions.FactoryEvent)) {
	panic("ledger event recorder registered during inert construction")
}

func (ledger *recordingLedger) AddEventTypeRecorder(func(factorydefinitions.FactoryEventType)) {
	panic("ledger event-type recorder registered during inert construction")
}

func (ledger *recordingLedger) AppendRecordedEvent(factorydefinitions.FactoryEvent) {
	panic("ledger append during inert construction")
}

func TestNewServiceConstructsInertRoot(t *testing.T) {
	t.Parallel()

	ledger := &recordingLedger{}
	writeCalls := 0
	writeFile := func(string, []byte) error {
		writeCalls++
		panic("snapshot write during inert construction")
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	makeDirectories, createTemporaryFile, removePath, renamePath, readFile := testPublicationEffects()
	service, err := recordingswire.NewService(
		ledger,
		nil,
		writeFile,
		makeDirectories,
		createTemporaryFile,
		removePath,
		renamePath,
		readFile,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	if ledger.subscribeCalls != 0 {
		t.Fatalf("construction started ledger subscriptions %d times, want inert construction", ledger.subscribeCalls)
	}
	if writeCalls != 0 {
		t.Fatalf("construction wrote snapshots %d times, want inert construction", writeCalls)
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	if leaked := runtime.NumGoroutine() - baseline; leaked > 4 {
		t.Fatalf(
			"goroutine leak after construction: baseline=%d current=%d delta=%d, want no flush ticker goroutines",
			baseline, runtime.NumGoroutine(), leaked,
		)
	}

	var root recordings.Service = service
	if _, err := root.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: "missing-after-inert-construction",
	}); !errors.Is(err, recordings.ErrReplayRecordingNotFound) {
		t.Fatalf("LoadReplayRecording() = %v, want ErrReplayRecordingNotFound after inert construction", err)
	}
	_, err = root.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{RecordingID: "missing-after-inert-construction"},
	})
	var historicalErr *recordings.HistoricalRecordingQueryError
	if !errors.As(err, &historicalErr) || historicalErr.Kind != recordings.HistoricalRecordingQueryErrorInvalidRequest {
		t.Fatalf("QueryHistoricalRecording() = %v, want typed invalid-request failure", err)
	}
}

func TestNewServiceRejectsMissingRequiredDependencies(t *testing.T) {
	t.Parallel()

	validLedger := stubLedger{}
	validWriteFile := func(string, []byte) error { return nil }
	tests := []struct {
		name      string
		ledger    recordings.Ledger
		writeFile func(string, []byte) error
		wantErr   string
	}{
		{
			name:      "ledger",
			ledger:    nil,
			writeFile: validWriteFile,
			wantErr:   "construct Recordings: ledger is required",
		},
		{
			name:      "snapshot write function",
			ledger:    validLedger,
			writeFile: nil,
			wantErr:   "construct Recordings: snapshot write function is required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			makeDirectories, createTemporaryFile, removePath, renamePath, readFile := testPublicationEffects()
			service, err := recordingswire.NewService(
				test.ledger,
				nil,
				test.writeFile,
				makeDirectories,
				createTemporaryFile,
				removePath,
				renamePath,
				readFile,
			)
			if err == nil {
				t.Fatalf("NewService() error = nil, want missing %s dependency", test.name)
			}
			if err.Error() != test.wantErr {
				t.Fatalf("NewService() error = %q, want %q", err.Error(), test.wantErr)
			}
			if service != nil {
				t.Fatalf("NewService() = %#v, want nil service", service)
			}
		})
	}
}

func TestNewServiceConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	service, err := recordingswire.NewService(
		stubLedger{},
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
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root recordings.Service = service
	if _, err := root.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: "missing-wire-root",
	}); !errors.Is(err, recordings.ErrReplayRecordingNotFound) {
		t.Fatalf("LoadReplayRecording() = %v, want ErrReplayRecordingNotFound", err)
	}
}

func TestNewServiceRejectsMissingArtifactPublicationEffects(t *testing.T) {
	t.Parallel()

	service, err := recordingswire.NewService(
		stubLedger{},
		nil,
		func(string, []byte) error { return nil },
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("NewService() error = nil, want missing artifact publication effects")
	}
	if err.Error() != "construct Recordings publication: portable artifact publication operations are required" {
		t.Fatalf("NewService() error = %q, want missing publication effects", err.Error())
	}
	if service != nil {
		t.Fatalf("NewService() = %#v, want nil service", service)
	}
}

func testPublicationEffects() (
	recordings.RecordingMakeDirectories,
	recordings.RecordingCreateTemporaryFile,
	recordings.RecordingRemovePath,
	recordings.RecordingRenamePath,
	recordings.RecordingReadFile,
) {
	return os.MkdirAll,
		func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.Rename,
		os.ReadFile
}

// TestRecordingsProjectionServiceReturnsDashboardAndWorkstationData keeps
// detached projection results observable through the Recordings root.
func TestRecordingsProjectionServiceReturnsDashboardAndWorkstationData(t *testing.T) {
	t.Parallel()
	scope, view := functionalRecordingsWorldStateView(t)
	service := newFunctionalRecordingsRoot(t)
	dashboard, err := service.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{WorldState: view})
	if err != nil {
		t.Fatalf("QuerySimpleDashboard: %v", err)
	}
	if dashboard.Data.InFlightDispatchCount != 1 || !dashboard.Data.Session.HasData {
		t.Fatalf("dashboard data = %#v, want one active customer dispatch", dashboard.Data)
	}
	workstation, err := service.QueryWorkstationRequests(recordings.WorkstationRequestsQueryRequest{WorldState: view})
	if err != nil {
		t.Fatalf("QueryWorkstationRequests: %v", err)
	}
	if workstation.Projection.WorkstationRequestsByDispatchId == nil {
		t.Fatal("workstation projection is nil, want the active dispatch")
	}
	if _, ok := (*workstation.Projection.WorkstationRequestsByDispatchId)["dispatch-recordings-wire"]; !ok {
		t.Fatalf("workstation projection = %#v, want dispatch-recordings-wire", workstation.Projection)
	}
	events := functionalRecordingsCanonicalEvents(scope)
	if err := service.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{
		Events: events, Cursor: events[0].Cursor, Scope: scope,
	}); err != nil {
		t.Fatalf("ValidateReconnectReplayFrom: %v", err)
	}
}
func TestRecordingsProjectionServiceRejectsInvalidReplayInputs(t *testing.T) {
	t.Parallel()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-recordings-wire"}
	projection := recordingswire.NewProjectionService()
	missingSequence := 99
	legacyEvents := []factorydefinitions.FactoryEvent{
		functionalRecordingsFactoryEvent("event-0", scope.FactorySessionID, 0),
		functionalRecordingsFactoryEvent("event-1", scope.FactorySessionID, 1),
	}
	if err := projection.ValidateReconnectReplay(
		legacyEvents,
		factorydefinitions.FactoryEventReconnectCursor{AfterSequence: &missingSequence},
		factorydefinitions.FactoryEventReconnectScope{SessionID: scope.FactorySessionID},
	); !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf("ValidateReconnectReplay(missing cursor) = %v, want ErrReconnectCursorNotFound", err)
	}
	if _, err := projection.ReconstructFactoryWorldState(nil, -1); !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("ReconstructFactoryWorldState(negative tick) = %v, want ErrInvalidProjectionInput", err)
	}
}
func TestRecordingsWorkSnapshotReaderReturnsDetachedAdmission(t *testing.T) {
	t.Parallel()
	scope, view := functionalRecordingsWorldStateView(t)
	reader := recordingswire.NewWorkSnapshotReader(&functionalWorkReadRoot{
		events: []recordings.CanonicalEvent{functionalWorkRequestEvent(scope)},
		view:   view,
	})
	snapshot, err := reader.ReadWorkSnapshot(context.Background(), scope.FactorySessionID)
	if err != nil {
		t.Fatalf("ReadWorkSnapshot: %v", err)
	}
	if len(snapshot.Items) != 1 || snapshot.Items[0].WorkID != "work-recordings-wire" {
		t.Fatalf("work snapshot items = %#v, want one detached work item", snapshot.Items)
	}
	if len(snapshot.Admissions) != 1 || snapshot.Admissions[0].WorkID != "work-recordings-wire" || snapshot.Admissions[0].Name != "Review recording" {
		t.Fatalf("work snapshot admissions = %#v, want canonical admission", snapshot.Admissions)
	}
	if recordingswire.NewWorkSnapshotReader(nil) != nil {
		t.Fatal("NewWorkSnapshotReader(nil) returned a reader")
	}
}
func functionalRecordingsWorldStateView(t *testing.T) (recordings.CanonicalEventScope, recordings.WorldStateView) {
	t.Helper()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-recordings-wire"}
	workItem := work.FactoryWorkItem{
		ID:          "work-recordings-wire",
		WorkTypeID:  "task",
		DisplayName: "Review recording",
		State:       "review",
		TraceID:     "trace-recordings-wire",
	}
	state := recordings.FactoryWorldState{
		Tick: 4,
		Topology: recordings.InitialStructurePayload{
			WorkTypes: []recordings.FactoryWorkType{{
				ID:   "task",
				Name: "task",
				States: []recordings.FactoryStateDefinition{{
					Value: "review", Category: work.StateTypeProcessing,
				}},
			}},
			Places: []recordings.FactoryPlace{{
				ID: "task:review", TypeID: "task", State: "review", Category: work.StateTypeProcessing,
			}},
			Workstations: []recordings.FactoryWorkstation{{
				ID: "review", Name: "Review", InputPlaceIDs: []string{"task:review"},
			}},
		},
		WorkItemsByID:       map[string]work.FactoryWorkItem{workItem.ID: workItem},
		ActiveWorkItemsByID: map[string]work.FactoryWorkItem{workItem.ID: workItem},
		ActiveDispatches: map[string]recordings.FactoryWorldDispatch{
			"dispatch-recordings-wire": {
				DispatchID:   "dispatch-recordings-wire",
				TransitionID: "review",
				Workstation:  recordings.FactoryWorkstationRef{ID: "review", Name: "Review"},
				StartedAt:    time.Date(2026, time.August, 23, 12, 0, 1, 0, time.UTC),
				WorkItemIDs:  []string{workItem.ID},
				TraceIDs:     []string{workItem.TraceID},
			},
		},
		PlaceOccupancyByID: map[string]recordings.FactoryPlaceOccupancy{
			"task:review": {PlaceID: "task:review", WorkItemIDs: []string{workItem.ID}, TokenCount: 1},
		},
	}
	statePayload, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal world state: %v", err)
	}
	return scope, recordings.WorldStateView{
		SchemaVersion: recordings.WorldStateViewSchemaV1,
		Scope:         scope,
		SelectedTick:  4,
		Payload:       string(statePayload),
	}
}
func newFunctionalRecordingsRoot(t *testing.T) recordings.Service {
	t.Helper()
	ledger := recordingswire.NewRuntimeLedger(nil, time.Now, "functional-recordings-wire", nil)
	if ledger == nil {
		t.Fatal("NewRuntimeLedger returned nil")
	}
	service, err := recordingswire.NewServiceWithProjectionAndEffects(
		ledger,
		recordingswire.NewProjectionService(),
		nil,
		func(string, []byte) error { return nil },
		os.MkdirAll,
		func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.Rename,
		os.ReadFile,
	)
	if err != nil {
		t.Fatalf("NewServiceWithProjectionAndEffects: %v", err)
	}
	return service
}
func functionalRecordingsCanonicalEvents(scope recordings.CanonicalEventScope) []recordings.CanonicalEvent {
	when := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
	return []recordings.CanonicalEvent{
		{
			ID: "event-0", Sequence: 0, Scope: scope,
			Cursor:     recordings.CanonicalEventCursor{StreamGenerationID: "functional-recordings-wire", Sequence: 0},
			RecordedAt: when, Kind: recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeRunRequest),
			Payload: `{"recordedAt":"2026-08-23T12:00:00Z","factory":{"name":"wire-factory","workTypes":[],"resources":[],"workers":[],"workstations":[]}}`,
		},
		{
			ID: "event-1", Sequence: 1, Scope: scope,
			Cursor:     recordings.CanonicalEventCursor{StreamGenerationID: "functional-recordings-wire", Sequence: 1},
			RecordedAt: when.Add(time.Second), Kind: recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeRunResponse), Payload: "{}",
		},
	}
}
func functionalRecordingsFactoryEvent(id, sessionID string, sequence int) factorydefinitions.FactoryEvent {
	payload := json.RawMessage(`{}`)
	if sequence == 0 {
		payload = json.RawMessage(`{"recordedAt":"2026-08-23T12:00:00Z","factory":{"name":"wire-factory","workTypes":[],"resources":[],"workers":[],"workstations":[]}}`)
	}
	return factorydefinitions.FactoryEvent{
		Id:            id,
		SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1,
		Type:          factorydefinitions.FactoryEventTypeRunRequest,
		Context: factorydefinitions.FactoryEventContext{
			EventTime:       time.Date(2026, time.August, 23, 12, 0, 0, sequence, time.UTC),
			Sequence:        sequence,
			SessionID:       stringPointer(sessionID),
			SessionSequence: intPointer(sequence),
		},
		Payload: payload,
	}
}
func functionalWorkRequestEvent(scope recordings.CanonicalEventScope) recordings.CanonicalEvent {
	payload, _ := json.Marshal(work.WorkRequestEventPayload{
		Type:  work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.WorkRequestEventWork{{Name: "Review recording"}},
	})
	workIDs := []string{"work-recordings-wire"}
	contextPayload, _ := json.Marshal(factorydefinitions.FactoryEventContext{WorkIDs: &workIDs})
	return recordings.CanonicalEvent{
		ID: "work-request-recordings-wire", Sequence: 0, FactoryTick: 4, Scope: scope,
		Cursor:     recordings.CanonicalEventCursor{StreamGenerationID: "functional-work-snapshot", Sequence: 0},
		RecordedAt: time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC),
		Kind:       recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkRequest),
		Payload:    string(payload), SourceContext: string(contextPayload),
	}
}

type functionalWorkReadRoot struct {
	events []recordings.CanonicalEvent
	view   recordings.WorldStateView
}

func (root *functionalWorkReadRoot) SubscribeFrom(
	_ context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	if request.Scope.FactorySessionID == "" {
		return recordings.SubscribeResult{}, recordings.ErrInvalidProjectionScope
	}
	events := append([]recordings.CanonicalEvent(nil), root.events...)
	index := 0
	return recordings.SubscribeResult{
		Subscription: func(context.Context) recordings.SubscriptionOutcome {
			if index >= len(events) {
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
			}
			outcome := recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: events[index]}
			index++
			return outcome
		},
	}, nil
}
func (root *functionalWorkReadRoot) ReconstructWorldState(
	recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	return recordings.ReconstructWorldStateResult{WorldState: root.view}, nil
}
func v2RecordingFixture(t *testing.T) []byte {
	t.Helper()
	fixturePath := testutil.MustRepoPath(t, filepath.Join(
		"tests", "functional", "work", "watch", "testdata", "production-retry-ledger.replay.json",
	))
	artifact := testutil.LoadReplayArtifact(t, fixturePath)
	if len(artifact.Events) == 0 {
		t.Fatal("replay fixture has no events")
	}
	var data bytes.Buffer
	encodeV2Record(t, &data, map[string]any{
		"recordType":    "header",
		"schemaVersion": "agent-factory.replay.v2",
		"recordedAt":    artifact.RecordedAt,
		"sessionId":     "00000000-0000-4000-8000-000000000001",
		"factoryIdentity": map[string]string{
			"id": "functional-v2-factory", "name": "functional-v2-factory",
			"factoryDirectory": "functional-v2-factory", "sourceDirectory": "functional-v2-factory",
		},
		"hashes": map[string]string{
			"factory_hash":        "sha256:functional-factory",
			"workers_hash":        "sha256:functional-workers",
			"workstations_hash":   "sha256:functional-workstations",
			"runtime_config_hash": "sha256:functional-runtime",
		},
	})
	// The first run-request event contains the Factory snapshot required by
	// world-state reconstruction. A one-event prefix is enough to exercise the
	// v2 framing reader while keeping this compatibility probe deterministic.
	for _, event := range artifact.Events[:1] {
		encodeV2Record(t, &data, map[string]any{"recordType": "event", "event": event})
	}
	encodeV2Record(t, &data, map[string]any{
		"recordType": "terminal", "finishedAt": artifact.RecordedAt.Add(time.Hour),
		"terminalState": "FINALIZED", "flushDiagnostics": map[string]any{},
	})
	return data.Bytes()
}

// TestReplayArtifactLoaderReadsV2ThroughRecordingWire proves the lower-level
// replay loader selected by Recordings composition also decodes the same
// append-only artifact. Historical query intentionally consumes the canonical
// event projection; this check keeps the replay-input boundary observable for
// callers that need the normalized replay model.
func TestReplayArtifactLoaderReadsV2ThroughRecordingWire(t *testing.T) {
	t.Parallel()
	loader := recordingswire.NewReplayArtifactLoader(
		functionalReplayArtifactStorage{payload: v2RecordingFixture(t)},
		decodeFunctionalFactorySnapshot,
	)
	artifact, err := loader("historical-recording.jsonl")
	if err != nil {
		t.Fatalf("ReplayArtifactLoader(v2): %v", err)
	}
	if artifact == nil || len(artifact.Events) != 1 {
		t.Fatalf("ReplayArtifactLoader(v2) artifact = %#v, want one event", artifact)
	}
	if artifact.Factory == nil {
		t.Fatal("ReplayArtifactLoader(v2) factory snapshot is nil")
	}
	if artifact.WallClock == nil || artifact.WallClock.FinishedAt.IsZero() {
		t.Fatalf("ReplayArtifactLoader(v2) wall clock = %#v, want terminal finish", artifact.WallClock)
	}
}

// TestReplayArtifactLoaderReadsLegacyArtifactAndClockThroughRecordingWire
// keeps the v1 compatibility path and its deterministic replay clock
// observable alongside the v2 loader.
func TestReplayArtifactLoaderReadsLegacyArtifactAndClockThroughRecordingWire(t *testing.T) {
	t.Parallel()
	fixturePath := testutil.MustRepoPath(t, filepath.Join(
		"tests", "functional", "work", "watch", "testdata", "production-retry-ledger.replay.json",
	))
	payload, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read legacy replay fixture: %v", err)
	}
	loader := recordingswire.NewReplayArtifactLoader(
		functionalReplayArtifactStorage{payload: payload},
		decodeFunctionalFactorySnapshot,
	)
	artifact, err := loader("legacy-recording.json")
	if err != nil {
		t.Fatalf("ReplayArtifactLoader(legacy): %v", err)
	}
	if artifact == nil || len(artifact.Events) == 0 || artifact.Factory == nil {
		t.Fatalf("ReplayArtifactLoader(legacy) artifact = %#v, want events and factory snapshot", artifact)
	}
	clock := recordingswire.NewReplayClock(artifact)
	if clock == nil || !clock.Now().Equal(artifact.RecordedAt) {
		t.Fatalf("NewReplayClock(legacy) = %#v, want recorded time %s", clock, artifact.RecordedAt)
	}
	if recordingswire.NewReplayClock(nil) != nil {
		t.Fatal("NewReplayClock(nil) returned a clock")
	}
}

// TestReplayArtifactLoaderNormalizesHistoricalFailureDetailsThroughWire
// proves legacy failure aliases are converted at the replay read boundary and
// do not leak the old fields to callers of the normalized artifact.
func TestReplayArtifactLoaderNormalizesHistoricalFailureDetailsThroughWire(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		fields        map[string]any
		wantReason    string
		wantMessage   string
		wantCanonical bool
	}{
		{
			name:        "legacy reason and message",
			fields:      map[string]any{"failureReason": "timeout", "failureMessage": "provider timed out"},
			wantReason:  "timeout",
			wantMessage: "provider timed out",
		},
		{
			name:        "legacy error class uses safe fallback",
			fields:      map[string]any{"errorClass": "provider failure"},
			wantReason:  "unknown",
			wantMessage: "Failure details were not recorded in this historical event.",
		},
		{
			name: "canonical detail wins",
			fields: map[string]any{
				"failureDetail": map[string]any{"reason": "throttled", "message": "rate limited"},
				"failureReason": "timeout",
			},
			wantReason:    "throttled",
			wantMessage:   "rate limited",
			wantCanonical: true,
		},
		{
			name:        "expected artifacts alias",
			fields:      map[string]any{"failureReason": "expected artifacts unsatisfied", "failureMessage": "missing output"},
			wantReason:  "EXPECTED_ARTIFACTS_UNSATISFIED",
			wantMessage: "missing output",
		},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			payload := legacyReplayFailureFixture(t, testCase.fields)
			loader := recordingswire.NewReplayArtifactLoader(
				functionalReplayArtifactStorage{payload: payload},
				decodeFunctionalFactorySnapshot,
			)
			artifact, err := loader("legacy-failure-recording.json")
			if err != nil {
				t.Fatalf("ReplayArtifactLoader(legacy failure): %v", err)
			}
			if len(artifact.Events) == 0 {
				t.Fatal("normalized artifact has no events")
			}
			var eventPayload map[string]any
			if err := json.Unmarshal(artifact.Events[0].Payload, &eventPayload); err != nil {
				t.Fatalf("decode normalized event payload: %v", err)
			}
			detail, ok := eventPayload["failureDetail"].(map[string]any)
			if !ok || detail["reason"] != testCase.wantReason || detail["message"] != testCase.wantMessage {
				t.Fatalf("normalized failureDetail = %#v, want reason=%q message=%q", detail, testCase.wantReason, testCase.wantMessage)
			}
			for _, field := range []string{"failureReason", "failureMessage", "errorClass"} {
				if _, exists := eventPayload[field]; exists {
					t.Fatalf("normalized event payload still contains %q: %#v", field, eventPayload)
				}
			}
			if testCase.wantCanonical && detail["reason"] != "throttled" {
				t.Fatalf("canonical detail was not retained: %#v", detail)
			}
		})
	}
}
func legacyReplayFailureFixture(t *testing.T, fields map[string]any) []byte {
	t.Helper()
	fixturePath := testutil.MustRepoPath(t, filepath.Join(
		"tests", "functional", "work", "watch", "testdata", "production-retry-ledger.replay.json",
	))
	payload, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read replay fixture: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode replay fixture: %v", err)
	}
	events, ok := document["events"].([]any)
	if !ok || len(events) == 0 {
		t.Fatal("replay fixture has no events")
	}
	event, ok := events[0].(map[string]any)
	if !ok {
		t.Fatal("replay fixture first event is not an object")
	}
	eventPayload, ok := event["payload"].(map[string]any)
	if !ok {
		t.Fatal("replay fixture first event payload is not an object")
	}
	for key, value := range fields {
		eventPayload[key] = value
	}
	event["payload"] = eventPayload
	document["events"] = events
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode replay fixture: %v", err)
	}
	return encoded
}

// TestReplayArtifactLoaderRejectsInvalidV2FramingThroughRecordingWire keeps
// malformed append-only records on the v2 path. The loader must report the
// framing error rather than silently treating the input as a legacy artifact.
func TestReplayArtifactLoaderRejectsInvalidV2FramingThroughRecordingWire(t *testing.T) {
	t.Parallel()
	fixture := v2RecordingFixture(t)
	header := v2RecordLine(t, fixture, "header")
	event := v2RecordLine(t, fixture, "event")
	terminal := v2RecordLine(t, fixture, "terminal")
	invalidHeader := v2HeaderWithoutHashes(t, header)
	cases := map[string][]byte{
		"unsupported record":        appendV2Lines([]byte(`{"recordType":"unknown","schemaVersion":"agent-factory.replay.v2"}`), nil),
		"event before header":       appendV2Lines([]byte(`{"recordType":"event","schemaVersion":"agent-factory.replay.v2"}`), nil),
		"terminal before header":    appendV2Lines([]byte(`{"recordType":"terminal","schemaVersion":"agent-factory.replay.v2"}`), nil),
		"malformed complete record": appendV2Lines(header, []byte("{\"recordType\"}\n")),
		"invalid header metadata":   appendV2Lines(invalidHeader, nil),
		"duplicate header":          appendV2Lines(header, header),
		"duplicate terminal":        appendV2Lines(header, terminal, terminal),
		"event after terminal":      appendV2Lines(header, terminal, event),
		"duplicate event":           appendV2Lines(header, event, event),
	}
	for name, payload := range cases {
		name, payload := name, payload
		t.Run(name, func(t *testing.T) {
			loader := recordingswire.NewReplayArtifactLoader(
				functionalReplayArtifactStorage{payload: payload},
				decodeFunctionalFactorySnapshot,
			)
			if _, err := loader("invalid-history.jsonl"); err == nil {
				t.Fatal("ReplayArtifactLoader() error = nil, want v2 framing error")
			}
		})
	}
}
func encodeV2Record(t *testing.T, data *bytes.Buffer, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal replay v2 record: %v", err)
	}
	data.Write(payload)
	data.WriteByte('\n')
}
func v2RecordLine(t *testing.T, payload []byte, recordType string) []byte {
	t.Helper()
	marker := []byte(`"recordType":"` + recordType + `"`)
	for _, line := range bytes.Split(payload, []byte("\n")) {
		if bytes.Contains(line, marker) {
			return append(append([]byte(nil), line...), '\n')
		}
	}
	t.Fatalf("replay v2 fixture has no %s record", recordType)
	return nil
}
func v2HeaderWithoutHashes(t *testing.T, header []byte) []byte {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(header), &value); err != nil {
		t.Fatalf("decode replay v2 header: %v", err)
	}
	delete(value, "hashes")
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode replay v2 header: %v", err)
	}
	return append(data, '\n')
}
func appendV2Lines(lines ...[]byte) []byte {
	var data []byte
	for _, line := range lines {
		data = append(data, line...)
	}
	return data
}

type functionalReplayArtifactStorage struct {
	payload []byte
}

func (storage functionalReplayArtifactStorage) WriteFile(string, []byte) error {
	return nil
}
func (storage functionalReplayArtifactStorage) ReadFile(string) ([]byte, error) {
	return append([]byte(nil), storage.payload...), nil
}
func stringPointer(value string) *string { return &value }
func intPointer(value int) *int          { return &value }

// TestHistoricalReplayV2ArtifactRemainsReadableThroughRecordingsRoot proves
// the published historical query path recognizes append-only replay artifacts
// without rewriting them into the legacy whole-file representation.
func TestHistoricalReplayV2ArtifactRemainsReadableThroughRecordingsRoot(t *testing.T) {
	t.Parallel()
	artifactPath := filepath.Join(t.TempDir(), "historical-recording.jsonl")
	payload := v2RecordingFixture(t)
	if err := os.WriteFile(artifactPath, payload, 0o600); err != nil {
		t.Fatalf("write replay v2 fixture: %v", err)
	}
	service := newFunctionalRecordingsRoot(t)
	result, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{
			RecordingID: "functional-v2-recording",
			Artifact:    recordings.RecordingArtifactReference(artifactPath),
		},
	})
	if err != nil {
		t.Fatalf("QueryHistoricalRecording(v2): %v", err)
	}
	if result.Status.State != recordings.RecordingFinalized {
		t.Fatalf("historical v2 status = %q, want %q", result.Status.State, recordings.RecordingFinalized)
	}
	if len(result.Events) == 0 || result.WorldState.Payload == "" {
		t.Fatalf("historical v2 result = %#v, want events and reconstructed state", result)
	}
	if !bytes.Contains(payload, []byte(`"schemaVersion":"agent-factory.replay.v2"`)) {
		t.Fatal("v2 fixture lost its framing schema")
	}
}
func decodeFunctionalFactorySnapshot(payload []byte) (*factorydefinitions.FactorySnapshot, error) {
	return factorydefinitions.NewFactorySnapshot(json.RawMessage(payload))
}
