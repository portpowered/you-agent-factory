package runtimeopening

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRestoreCurrentBoardStatePreservesDetachedMixedWorkProjection(t *testing.T) {
	t.Parallel()

	scope := recordings.CanonicalEventScope{FactorySessionID: "~default"}
	want := factorydefinitions.FactoryWorldState{
		Tick: 19,
		WorkRequestsByID: map[string]factorydefinitions.WorkRequestPayload{
			"request-batch-1": {
				RequestID: "request-batch-1",
				WorkItems: []work.FactoryWorkItem{{ID: "work-init"}, {ID: "work-processing"}, {ID: "work-awaiting-ci"}},
			},
		},
		WorkItemsByID: map[string]work.FactoryWorkItem{
			"work-init": {
				ID: "work-init", WorkTypeID: "task", State: "init", DisplayName: "Initialize",
				TraceID: "trace-batch", Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "first"}},
				Tags: map[string]string{"lane": "one"}, StructuredResult: nil, StructuredResultPresent: true,
			},
			"work-processing": {
				ID: "work-processing", WorkTypeID: "task", State: "PROCESSING", DisplayName: "Process",
				TraceID: "trace-batch", ParentID: "work-init", StructuredResult: map[string]any{"step": 2},
			},
			"work-awaiting-ci": {
				ID: "work-awaiting-ci", WorkTypeID: "task", State: "awaiting-ci", DisplayName: "Await CI",
				TraceID: "trace-batch", Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "third"}},
			},
		},
		RelationsByWorkID: map[string][]work.FactoryRelation{
			"work-processing": {{
				Type: string(work.WorkRelationDependsOn), TargetWorkID: "work-init", RequiredState: "done",
			}},
		},
		PlaceOccupancyByID: map[string]factorydefinitions.FactoryPlaceOccupancy{
			"task:init":        {PlaceID: "task:init", WorkItemIDs: []string{"work-init"}},
			"task:PROCESSING":  {PlaceID: "task:PROCESSING", WorkItemIDs: []string{"work-processing"}},
			"task:awaiting-ci": {PlaceID: "task:awaiting-ci", WorkItemIDs: []string{"work-awaiting-ci"}},
		},
	}
	payload, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal world state: %v", err)
	}
	var expected factorydefinitions.FactoryWorldState
	if err := json.Unmarshal(payload, &expected); err != nil {
		t.Fatalf("unmarshal expected world state: %v", err)
	}
	reader := &historicalBoardReaderStub{
		result: recordings.HistoricalRecordingQueryResult{
			WorldState: recordings.WorldStateView{
				SchemaVersion: recordings.WorldStateViewSchemaV1,
				Scope:         scope,
				Payload:       string(payload),
			},
		},
	}

	first, err := restoreCurrentBoardState(reader, "board.json", "~default", false)
	if err != nil {
		t.Fatalf("first restore: %v", err)
	}
	if first == nil || !reflect.DeepEqual(*first, expected) {
		t.Fatalf("first restored state = %#v, want %#v", first, expected)
	}
	first.WorkItemsByID["work-init"] = work.FactoryWorkItem{ID: "mutated"}

	second, err := restoreCurrentBoardState(reader, "board.json", "~default", false)
	if err != nil {
		t.Fatalf("second restore: %v", err)
	}
	if second == nil || !reflect.DeepEqual(*second, expected) {
		t.Fatalf("second restored state = %#v, want original state", second)
	}
	if reader.calls != 2 {
		t.Fatalf("history query calls = %d, want 2", reader.calls)
	}
	if reader.request.Recording.Scope != scope ||
		reader.request.Recording.Artifact != "board.json" ||
		reader.request.Recording.RecordingID != "current-board/~default" {
		t.Fatalf("history query request = %#v", reader.request)
	}
}

func TestRestoreCurrentBoardStateFailsClosedForCorruptHistory(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		stub historicalBoardReaderStub
		want string
	}{
		{
			name: "query failure",
			stub: historicalBoardReaderStub{err: errors.New("corrupt canonical history")},
			want: "restore current Factory Session board from \"board.json\"",
		},
		{
			name: "incompatible view",
			stub: historicalBoardReaderStub{result: recordings.HistoricalRecordingQueryResult{
				WorldState: recordings.WorldStateView{SchemaVersion: "unknown", Scope: recordings.CanonicalEventScope{FactorySessionID: "~default"}, Payload: "{}"},
			}},
			want: "incompatible world-state view",
		},
		{
			name: "invalid payload",
			stub: historicalBoardReaderStub{result: recordings.HistoricalRecordingQueryResult{
				WorldState: recordings.WorldStateView{SchemaVersion: recordings.WorldStateViewSchemaV1, Scope: recordings.CanonicalEventScope{FactorySessionID: "~default"}, Payload: "not-json"},
			}},
			want: "decode world state",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := restoreCurrentBoardState(&tc.stub, "board.json", "~default", false)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("restore error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestRestoreCurrentBoardStateTreatsMissingArtifactAsInitialOpen(t *testing.T) {
	t.Parallel()

	state, err := restoreCurrentBoardState(&historicalBoardReaderStub{err: &recordings.HistoricalRecordingQueryError{
		Kind: recordings.HistoricalRecordingQueryErrorMissingHistory,
	}}, "board.json", "~default", true)
	if err != nil {
		t.Fatalf("missing history restore error = %v, want nil", err)
	}
	if state != nil {
		t.Fatalf("missing history state = %#v, want nil", state)
	}
}

func TestRestoreCurrentBoardStateRejectsMissingArtifactAfterDurableState(t *testing.T) {
	t.Parallel()

	allowMissing, err := currentBoardHistoryMayBeUninitialized(
		context.Background(),
		&durableSessionStateStub{hasDurableState: true},
		"~default",
	)
	if err != nil {
		t.Fatalf("inspect durable session state: %v", err)
	}
	if allowMissing {
		t.Fatal("durable session state marked prior state as uninitialized")
	}
	_, err = restoreCurrentBoardState(&historicalBoardReaderStub{err: &recordings.HistoricalRecordingQueryError{
		Kind: recordings.HistoricalRecordingQueryErrorMissingHistory,
	}}, "board.json", "~default", allowMissing)
	if err == nil || !strings.Contains(err.Error(), "durable state exists but recording history is missing") {
		t.Fatalf("missing history after durable state error = %v, want fail-closed diagnostic", err)
	}
}

func TestCurrentBoardHistoryMayBeUninitializedUsesPersistenceBackedStateProbe(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		hasDurableState   bool
		wantUninitialized bool
	}{
		{name: "fresh factory", hasDurableState: false, wantUninitialized: true},
		{name: "durable factory", hasDurableState: true, wantUninitialized: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := durableSessionStateStub{hasDurableState: tc.hasDurableState}
			got, err := currentBoardHistoryMayBeUninitialized(context.Background(), &stub, "~default")
			if err != nil {
				t.Fatalf("currentBoardHistoryMayBeUninitialized: %v", err)
			}
			if got != tc.wantUninitialized {
				t.Fatalf("uninitialized = %t, want %t", got, tc.wantUninitialized)
			}
		})
	}
}

func TestCurrentBoardHistoryMayBeUninitializedUsesFreshPersistentOwnerBeforeMissingBoardRead(t *testing.T) {
	t.Parallel()
	const sessionID = "~default"
	projectRoot := t.TempDir()
	store, err := runtimepersist.NewDirectoryStore(
		runtimepersist.DirForProjectRoot(projectRoot),
		platformfilesystem.Local{},
	)
	if err != nil {
		t.Fatalf("NewDirectoryStore: %v", err)
	}
	firstOwner := newRuntimeOpeningPersistentOwner(projectRoot, store)
	if err := firstOwner.RecordPetriTokenMutations(sessionID, []factorydefinitions.TokenMutationRecord{{}}); err != nil {
		t.Fatalf("RecordPetriTokenMutations: %v", err)
	}

	// The fresh owner has no in-memory session state. The missing board reader
	// must therefore be evaluated against the snapshot left by the first owner.
	freshOwner := newRuntimeOpeningPersistentOwner(projectRoot, store)
	allowMissing, err := currentBoardHistoryMayBeUninitialized(
		context.Background(),
		freshOwner,
		sessionID,
	)
	if err != nil {
		t.Fatalf("inspect fresh persistent owner: %v", err)
	}
	if allowMissing {
		t.Fatal("missing board history after a prior owner was accepted as initial open")
	}
	_, err = restoreCurrentBoardState(&historicalBoardReaderStub{err: &recordings.HistoricalRecordingQueryError{
		Kind: recordings.HistoricalRecordingQueryErrorMissingHistory,
	}}, "missing-board.json", sessionID, allowMissing)
	if err == nil || !strings.Contains(err.Error(), "durable state exists but recording history is missing") {
		t.Fatalf("missing board history error = %v, want fail-closed diagnostic", err)
	}
}

func newRuntimeOpeningPersistentOwner(
	projectRoot string,
	store runtimepersist.Store,
) *factorysessionexecution.JavaScriptRuntimeService {
	return factorysessionexecution.NewJavaScriptRuntimeService(
		projectRoot,
		factorysessionexecution.ChildExecutorModeFake,
		nil,
		store,
		openingCoordinatorClock{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		factoryruntime.JavaScriptWorkerSettings{},
		nil,
		func() string { return "runtime-opening-test-session" },
		nil,
		nil,
		nil,
	)
}

type durableSessionStateStub struct {
	hasDurableState bool
}

func (stub *durableSessionStateStub) HasDurableState(
	_ context.Context,
	_ string,
) (bool, error) {
	return stub.hasDurableState, nil
}

type historicalBoardReaderStub struct {
	result  recordings.HistoricalRecordingQueryResult
	err     error
	calls   int
	request recordings.HistoricalRecordingQueryRequest
}

func (stub *historicalBoardReaderStub) QueryHistoricalRecording(
	request recordings.HistoricalRecordingQueryRequest,
) (recordings.HistoricalRecordingQueryResult, error) {
	stub.calls++
	stub.request = request
	return stub.result, stub.err
}

func TestRuntimeActivationUsesEngineServiceForDetachedHandoff(t *testing.T) {
	t.Parallel()

	proxy := &activationServiceFake{}
	engine := &activationServiceFake{}
	products := runtimeProducts{
		application: roles.OpenedApplicationRuntime{
			FactoryRuntime: proxy,
		},
		engine: engine,
	}

	if got := runtimeEngineService(products); got != engine {
		t.Fatalf("runtimeEngineService() = %T, want concrete engine %T", got, engine)
	}
	activation, err := newRuntimeActivation(products)
	if err != nil {
		t.Fatalf("newRuntimeActivation() error = %v", err)
	}
	if activation.WorkAndEventIngress != factoryruntime.APIFactory(engine) {
		t.Fatalf("published ingress = %T, want concrete engine %T", activation.WorkAndEventIngress, engine)
	}
	if _, err := activation.WorkAndEventIngress.SubmitWorkRequest(context.Background(), work.WorkRequest{}); err != nil {
		t.Fatalf("SubmitWorkRequest() error = %v", err)
	}
	if got := engine.submitCalls.Load(); got != 1 {
		t.Fatalf("engine SubmitWorkRequest calls = %d, want 1", got)
	}
	if got := proxy.submitCalls.Load(); got != 0 {
		t.Fatalf("session proxy SubmitWorkRequest calls = %d, want 0", got)
	}
	if _, ok := activation.Service.(factoryruntime.APIFactory); ok {
		t.Fatal("published activation service must not expose the migration-only Work and event ingress")
	}
}

func TestRuntimeActivationRejectsEngineWithoutDeclaredWorkAndEventIngress(t *testing.T) {
	t.Parallel()

	products := runtimeProducts{engine: controlOnlyEngineFake{}}
	if _, err := newRuntimeActivation(products); err == nil {
		t.Fatal("newRuntimeActivation() error = nil, want a missing-ingress failure")
	}
}

// controlOnlyEngineFake serves the Runtime Service contract without the
// migration-only Work submission and event subscription operations.
type controlOnlyEngineFake struct {
	factoryruntime.Service
}

func TestRuntimeBindingPublicationErrorPreservesPrimaryAndCleanupFailures(t *testing.T) {
	t.Parallel()

	bindErr := errors.New("bind failed")
	cleanupErr := errors.New("cleanup failed")
	err := runtimeBindingPublicationError(bindErr, cleanupErr)
	if !errors.Is(err, bindErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("runtimeBindingPublicationError() = %v, want both causes", err)
	}
}

func TestActivationCloserDeactivatesConcurrentCallsExactlyOnce(t *testing.T) {
	t.Parallel()

	service := &activationServiceFake{}
	var calls atomic.Int32
	binding := factoryruntime.RuntimeBinding{}.New(
		"runtime-1",
		service,
		func(context.Context) (factoryruntime.RuntimeDeactivationResult, error) {
			calls.Add(1)
			return factoryruntime.RuntimeDeactivationResult{}, nil
		},
	)
	closer := (&Factory{}).activationCloser(binding, "runtime-1")

	const callers = 16
	var wait sync.WaitGroup
	wait.Add(callers)
	errs := make(chan error, callers)
	for range callers {
		go func() {
			defer wait.Done()
			errs <- closer()
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("activation closer error = %v, want nil", err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("deactivation calls = %d, want exactly once", got)
	}
	if err := closer(); err != nil {
		t.Fatalf("second activation closer call = %v, want nil", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("deactivation calls after second close = %d, want exactly once", got)
	}
}

type activationServiceFake struct {
	factoryruntime.Service
	submitCalls    atomic.Int32
	subscribeCalls atomic.Int32
}

func (service *activationServiceFake) SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	service.submitCalls.Add(1)
	return work.WorkRequestSubmitResult{}, nil
}

func (service *activationServiceFake) SubscribeFactoryEvents(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	service.subscribeCalls.Add(1)
	return nil, nil
}

func TestActivationRequestCarriesExplicitRuntimeInputs(t *testing.T) {
	t.Parallel()

	skipPermissions := true
	request := &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{
			Directory:        "/factory",
			SourcePath:       "/source",
			ExecutionBaseDir: "/runtime",
		},
		FactoryRuntime: factoryruntime.RuntimeOpeningRequest{Verbose: true},
		FactorySession: factorysessions.SessionRuntimeOpeningRequest{
			BackendScopeID: "scope",
			Host: factorysessions.RuntimeHostRequest{
				Host: "127.0.0.1",
				Port: 8080,
			},
		},
		Workers: workers.RuntimeOpeningRequest{
			RunnerID:                          "runner",
			InvocationSkipPermissionsOverride: &skipPermissions,
		},
	}
	factory := &Factory{
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		factoryDefinitions:        activationDefinitionsStub{snapshot: activationSnapshot()},
	}
	activation, err := factory.activationRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("activationRequest() error = %v", err)
	}
	if activation.RuntimeID != "runtime-1" || activation.FactorySessionID != factorysessions.DefaultSessionID {
		t.Fatalf("activation identity = %#v, want runtime-1/%q", activation, factorysessions.DefaultSessionID)
	}
	if activation.Inputs.Definition.SourcePath != "/source" || activation.Inputs.Session.BackendScopeID != "scope" {
		t.Fatalf("activation inputs lost source or session values: %#v", activation.Inputs)
	}
	if activation.Inputs.Workers.InvocationSkipPermissionsOverride == nil || !*activation.Inputs.Workers.InvocationSkipPermissionsOverride {
		t.Fatal("activation inputs lost worker permission override")
	}
}

func TestRuntimeOpeningRequestRoundTripsResumePathToRecordingsContract(t *testing.T) {
	t.Parallel()

	const resumePath = "source.recording.json"
	resumeInput := recordings.LoadResumeInputResult{
		Input: recordings.LoadReplayInputResult{
			Legacy: &factorydefinitions.ReplayArtifact{
				Events: []factorydefinitions.FactoryEvent{{
					Id:      "resume-event",
					Context: factorydefinitions.FactoryEventContext{Tick: 7},
				}},
			},
		},
	}
	request := factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: "/factory"},
		Recordings: recordings.RuntimeOpeningRequest{
			RecordPath: "successor.recording.json",
			ResumePath: resumePath,
		},
	}

	activation := factoryruntime.RuntimeActivationRequest{
		Snapshot: activationSnapshot(),
		Inputs:   runtimeActivationInputs(request, &resumeInput),
	}
	opening, err := runtimeOpeningRequestFromActivation(activation)
	if err != nil {
		t.Fatalf("runtimeOpeningRequestFromActivation() error = %v", err)
	}
	if opening.Recordings.ResumePath != resumePath {
		t.Fatalf("Recordings resume path = %q, want %q", opening.Recordings.ResumePath, resumePath)
	}
	if opening.Recordings.RecordPath != request.Recordings.RecordPath {
		t.Fatalf("Recordings successor path = %q, want %q", opening.Recordings.RecordPath, request.Recordings.RecordPath)
	}
	if opening.Recordings.ResumeInput != resumeInput {
		t.Fatalf("Recordings resume input = %#v, want %#v", opening.Recordings.ResumeInput, resumeInput)
	}
}

func TestActivationRequestDetachesMockWorkerInputs(t *testing.T) {
	t.Parallel()

	request := &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: "/factory"},
		Workers: workers.RuntimeOpeningRequest{
			MockWorkers: &workers.MockWorkersConfig{
				MockWorkers: []workers.MockWorkerConfig{{
					RunType: workers.MockWorkerRunTypeScript,
					ScriptConfig: &workers.MockWorkerScriptConfig{
						Command: "run",
						Env:     map[string]string{"TOKEN": "one"},
					},
				}},
			},
		},
	}
	factory := &Factory{
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		factoryDefinitions:        activationDefinitionsStub{snapshot: activationSnapshot()},
	}
	activation, err := factory.activationRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("activationRequest() error = %v", err)
	}
	request.Workers.MockWorkers.MockWorkers[0].ScriptConfig.Env["TOKEN"] = "caller-mutated"
	request.Workers.MockWorkers.MockWorkers[0].ScriptConfig.Args = []string{"caller-mutated"}
	got := activation.Inputs.Workers.MockWorkers.MockWorkers[0]
	if got.ScriptConfig.Env["TOKEN"] != "one" || len(got.ScriptConfig.Args) != 0 {
		t.Fatalf("activation inputs retained caller mutation: %#v", got)
	}
}

func TestActivationRequestCarriesFactorySessionCorrelation(t *testing.T) {
	t.Parallel()

	factory := &Factory{
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		factoryDefinitions:        activationDefinitionsStub{snapshot: activationSnapshot()},
	}
	activation, err := factory.activationRequest(context.Background(), &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: "/factory"},
		FactorySession: factorysessions.SessionRuntimeOpeningRequest{
			FactorySessionID: "session-1",
		},
	})
	if err != nil {
		t.Fatalf("activationRequest() error = %v", err)
	}
	if activation.FactorySessionID != "session-1" || activation.Snapshot.Invocation.FactorySessionID != "session-1" {
		t.Fatalf("activation session correlation = %#v / %#v, want session-1", activation.FactorySessionID, activation.Snapshot.Invocation)
	}
}

func TestActivationRequestDerivesDirectoryForSourceOnlySnapshot(t *testing.T) {
	t.Parallel()

	sourcePath := filepath.Join(t.TempDir(), factorydefinitions.FactoryConfigFile)
	factory := &Factory{
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		factoryDefinitions: activationDefinitionsStub{snapshot: factorydefinitions.RuntimeSnapshot{
			EffectiveFactory:  factorydefinitions.FactoryConfig{Name: "source-only"},
			DefinitionVersion: &factorydefinitions.FactoryVersion{Logical: 1},
		}},
	}
	activation, err := factory.activationRequest(context.Background(), &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{SourcePath: sourcePath},
	})
	if err != nil {
		t.Fatalf("activationRequest() error = %v", err)
	}
	if activation.Snapshot.FactoryDir != filepath.Dir(sourcePath) {
		t.Fatalf("snapshot FactoryDir = %q, want %q", activation.Snapshot.FactoryDir, filepath.Dir(sourcePath))
	}
}

func TestActivationRequestReturnsTypedDefinitionsFailureBeforeRuntimeActivation(t *testing.T) {
	t.Parallel()

	want := &factorydefinitions.RuntimeSnapshotResolutionError{
		Diagnostic: factorydefinitions.RuntimeSnapshotDiagnostic{
			Code:    factorydefinitions.RuntimeSnapshotDiagnosticInvalidDefinition,
			Field:   "source",
			Message: "invalid Factory source",
		},
		Cause: factorydefinitions.ErrInvalidRuntimeSnapshotDefinition,
	}
	factory := &Factory{
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		factoryDefinitions:        activationDefinitionsStub{err: want},
	}
	_, err := factory.activationRequest(context.Background(), &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: "/factory"},
	})
	if !errors.Is(err, factorydefinitions.ErrInvalidRuntimeSnapshotDefinition) {
		t.Fatalf("activationRequest() error = %v, want typed Definitions failure", err)
	}
}

type activationDefinitionsStub struct {
	factorydefinitions.Service
	snapshot factorydefinitions.RuntimeSnapshot
	err      error
}

type resumeDefinitionStub struct {
	factorydefinitions.Service
	request  factorydefinitions.ResolveRuntimeSnapshotRequest
	snapshot factorydefinitions.RuntimeSnapshot
}

func (stub *resumeDefinitionStub) ResolveRuntimeSnapshot(
	_ context.Context,
	request factorydefinitions.ResolveRuntimeSnapshotRequest,
) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
	stub.request = request
	return factorydefinitions.ResolveRuntimeSnapshotResult{Snapshot: stub.snapshot}, nil
}

func (stub activationDefinitionsStub) ResolveRuntimeSnapshot(
	context.Context,
	factorydefinitions.ResolveRuntimeSnapshotRequest,
) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
	if stub.err != nil {
		return factorydefinitions.ResolveRuntimeSnapshotResult{}, stub.err
	}
	return factorydefinitions.ResolveRuntimeSnapshotResult{Snapshot: stub.snapshot}, nil
}

func activationSnapshot() factorydefinitions.RuntimeSnapshot {
	return factorydefinitions.RuntimeSnapshot{
		FactoryDir:        "/factory",
		RuntimeBaseDir:    "/runtime",
		DefinitionVersion: &factorydefinitions.FactoryVersion{Logical: 1},
		EffectiveFactory:  factorydefinitions.FactoryConfig{Name: "snapshot"},
	}
}

func TestOpenForRequestRoutesLegacyReplayThroughRuntimeRoot(t *testing.T) {
	t.Parallel()

	root := &replayRoutingRoot{}
	replayInputs := &legacyReplayInputsStub{}
	factory := &Factory{
		runtimeRoot:               root,
		replayInputs:              replayInputs,
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		factoryDefinitions:        activationDefinitionsStub{snapshot: activationSnapshot()},
		decodeReplayConfig: func(*factorydefinitions.FactorySnapshot) (factorydefinitions.ReplayRuntimeConfig, error) {
			return replayRuntimeConfigStub{}, nil
		},
	}
	_, err := factory.openForRequest(context.Background(), &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: "/factory"},
		Recordings:        recordings.RuntimeOpeningRequest{ReplayPath: "legacy.json"},
	})
	if err != nil {
		t.Fatalf("openForRequest(legacy replay) error = %v", err)
	}
	if root.activations != 1 {
		t.Fatalf("Runtime root activations = %d, want one", root.activations)
	}
	if replayInputs.calls != 1 {
		t.Fatalf("replay input classifications = %d, want one", replayInputs.calls)
	}
}

func TestOpenForRequestConsumesResumeSourceBeforeLiveSuccessorActivation(t *testing.T) {
	t.Parallel()

	root := &resumeRoutingRoot{}
	factorySnapshot := factorydefinitions.FactorySnapshot(`{"factoryDirectory":"/factory","name":"legacy"}`)
	resumeInput := recordings.LoadResumeInputResult{
		Input: recordings.LoadReplayInputResult{
			Legacy: &factorydefinitions.ReplayArtifact{
				Factory: &factorySnapshot,
				Events: []factorydefinitions.FactoryEvent{{
					Id:      "resume-event",
					Context: factorydefinitions.FactoryEventContext{Tick: 7},
				}},
			},
		},
	}
	resumeRuntime := &resumeInputRuntime{result: resumeInput}
	factory := &Factory{
		runtimeRoot:               root,
		recordingsRuntime:         resumeRuntime,
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		factoryDefinitions:        activationDefinitionsStub{snapshot: activationSnapshot()},
		decodeReplayConfig: func(*factorydefinitions.FactorySnapshot) (factorydefinitions.ReplayRuntimeConfig, error) {
			return replayRuntimeConfigStub{}, nil
		},
	}
	_, err := factory.openForRequest(context.Background(), &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: "/factory"},
		Recordings: recordings.RuntimeOpeningRequest{
			RecordPath: "successor.recording.json",
			ResumePath: "source.recording.json",
		},
	})
	if err != nil {
		t.Fatalf("openForRequest(resume) error = %v", err)
	}
	if resumeRuntime.path != "source.recording.json" {
		t.Fatalf("resume source path = %q, want source.recording.json", resumeRuntime.path)
	}
	if root.activation.Inputs.ResumeInput != resumeInput {
		t.Fatalf("activation resume input = %#v, want %#v", root.activation.Inputs.ResumeInput, resumeInput)
	}
	if len(root.activation.Inputs.ResumeInput.Input.Legacy.Events) != 1 ||
		root.activation.Inputs.ResumeInput.Input.Legacy.Events[0].Id != "resume-event" {
		t.Fatalf("activation resume events = %#v, want selected recording event", root.activation.Inputs.ResumeInput.Input.Legacy.Events)
	}
	if root.activation.Inputs.Recordings.ResumePath != "source.recording.json" {
		t.Fatalf("activation resume path = %q, want source.recording.json", root.activation.Inputs.Recordings.ResumePath)
	}
	if root.activation.Inputs.Recordings.RecordPath != "successor.recording.json" {
		t.Fatalf("activation successor path = %q, want successor.recording.json", root.activation.Inputs.Recordings.RecordPath)
	}
	if root.activation.Inputs.Recordings.ReplayPath != "" {
		t.Fatalf("activation replay path = %q, want empty for resume", root.activation.Inputs.Recordings.ReplayPath)
	}
}

func TestOpenForRequestResumeUsesCapturedFactoryDefinition(t *testing.T) {
	t.Parallel()

	factorySnapshot := factorydefinitions.FactorySnapshot(`{"factoryDirectory":"/recorded","name":"recorded"}`)
	resumeInput := recordings.LoadResumeInputResult{
		Input: recordings.LoadReplayInputResult{
			Legacy: &factorydefinitions.ReplayArtifact{
				Factory: &factorySnapshot,
				Events: []factorydefinitions.FactoryEvent{{
					Id:      "resume-event",
					Context: factorydefinitions.FactoryEventContext{Tick: 4},
				}},
			},
		},
	}
	definitions := &resumeDefinitionStub{
		snapshot: factorydefinitions.RuntimeSnapshot{
			FactoryDir:     "/recorded",
			RuntimeBaseDir: "/recorded",
			EffectiveFactory: factorydefinitions.FactoryConfig{
				Name: "recorded",
			},
		},
	}
	root := &resumeRoutingRoot{}
	factory := &Factory{
		runtimeRoot:               root,
		recordingsRuntime:         &resumeInputRuntime{result: resumeInput},
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		factoryDefinitions:        definitions,
		decodeReplayConfig: func(*factorydefinitions.FactorySnapshot) (factorydefinitions.ReplayRuntimeConfig, error) {
			return replayRuntimeConfigStub{factoryDir: "/recorded"}, nil
		},
	}

	_, err := factory.openForRequest(context.Background(), &factorysessions.RuntimeOpeningRequest{
		Recordings: recordings.RuntimeOpeningRequest{
			RecordPath: "successor.recording.json",
			ResumePath: "source.recording.json",
		},
	})
	if err != nil {
		t.Fatalf("openForRequest(bare resume) error = %v", err)
	}
	if string(definitions.request.Canonical) != string(factorySnapshot) {
		t.Fatalf("resume Factory Definition canonical = %q, want captured recording definition", definitions.request.Canonical)
	}
	if root.activation.Snapshot.EffectiveFactory.Name != "recorded" {
		t.Fatalf("activation Factory name = %q, want captured recording definition", root.activation.Snapshot.EffectiveFactory.Name)
	}
	if root.activation.Snapshot.FactoryDir != "/recorded" {
		t.Fatalf("activation Factory directory = %q, want captured recording directory", root.activation.Snapshot.FactoryDir)
	}
	if root.activation.Inputs.Recordings.RecordPath != "successor.recording.json" {
		t.Fatalf("activation successor path = %q, want successor.recording.json", root.activation.Inputs.Recordings.RecordPath)
	}
	if root.activation.Inputs.Recordings.ReplayPath != "" {
		t.Fatalf("activation replay path = %q, want empty for resume", root.activation.Inputs.Recordings.ReplayPath)
	}
}

type resumeRoutingRoot struct {
	factoryruntime.Service
	activation factoryruntime.RuntimeActivationRequest
}

func (root *resumeRoutingRoot) Activate(
	_ context.Context,
	request factoryruntime.RuntimeActivationRequest,
) (factoryruntime.RuntimeActivationResult, error) {
	root.activation = request
	return factoryruntime.RuntimeActivationResult{
		RuntimeID: "runtime-1",
		Runtime: factoryruntime.RuntimeActivationView{
			RuntimeID: "runtime-1",
			Service:   &activatedRuntimeService{products: runtimeProducts{}},
		},
	}, nil
}

func (root *resumeRoutingRoot) Deactivate(
	context.Context,
	factoryruntime.RuntimeDeactivationRequest,
) (factoryruntime.RuntimeDeactivationResult, error) {
	return factoryruntime.RuntimeDeactivationResult{}, nil
}

type resumeInputRuntime struct {
	recordings.RuntimeOpening
	path   string
	result recordings.LoadResumeInputResult
}

func (runtime *resumeInputRuntime) LoadResumeInput(
	request recordings.LoadResumeInputRequest,
) (recordings.LoadResumeInputResult, error) {
	runtime.path = request.Path
	return runtime.result, nil
}

type replayRoutingRoot struct {
	factoryruntime.Service
	activations int
}

func (root *replayRoutingRoot) Activate(
	context.Context,
	factoryruntime.RuntimeActivationRequest,
) (factoryruntime.RuntimeActivationResult, error) {
	root.activations++
	return factoryruntime.RuntimeActivationResult{
		RuntimeID: "runtime-1",
		Runtime: factoryruntime.RuntimeActivationView{
			RuntimeID: "runtime-1",
			Service:   &activatedRuntimeService{products: runtimeProducts{}},
		},
	}, nil
}

func (root *replayRoutingRoot) Deactivate(
	context.Context,
	factoryruntime.RuntimeDeactivationRequest,
) (factoryruntime.RuntimeDeactivationResult, error) {
	return factoryruntime.RuntimeDeactivationResult{}, nil
}

type legacyReplayInputsStub struct {
	calls int
}

type replayRuntimeConfigStub struct {
	factoryDir     string
	runtimeBaseDir string
	name           string
}

func (stub replayRuntimeConfigStub) FactoryConfig() *factorydefinitions.FactoryConfig {
	name := stub.name
	if name == "" {
		name = "legacy"
	}
	return &factorydefinitions.FactoryConfig{Name: name}
}
func (stub replayRuntimeConfigStub) FactoryDir() string {
	if stub.factoryDir != "" {
		return stub.factoryDir
	}
	return "/factory"
}
func (stub replayRuntimeConfigStub) RuntimeBaseDir() string {
	if stub.runtimeBaseDir != "" {
		return stub.runtimeBaseDir
	}
	return stub.FactoryDir()
}
func (replayRuntimeConfigStub) Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	return nil, false
}
func (replayRuntimeConfigStub) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}
func (replayRuntimeConfigStub) WorkstationByID(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}

func (stub *legacyReplayInputsStub) LoadReplayInput(
	recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	stub.calls++
	snapshot := factorydefinitions.FactorySnapshot(`{"name":"legacy"}`)
	return recordings.LoadReplayInputResult{
		Legacy: &factorydefinitions.ReplayArtifact{Factory: &snapshot},
	}, nil
}

// TestOpenActivatedRuntimeRoutesRoleCleanupThroughRuntimeDeactivation pins the
// P6-B successor behavior for Sessions runtime opening: opening resolves
// Definitions values, calls Runtime.Activate, and routes every opened role's
// cleanup edge through the Runtime deactivation operation rather than through a
// retained hosted-instance, replacement-builder, lifecycle, or sidecar handle.
// All three role cleanup edges must resolve to the single Runtime-owned closer,
// so draining them cannot deactivate the Runtime more than once.
func TestOpenActivatedRuntimeRoutesRoleCleanupThroughRuntimeDeactivation(t *testing.T) {
	t.Parallel()

	root := &cleanupRoutingRoot{}
	factory := &Factory{
		runtimeRoot:               root,
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		factoryDefinitions:        activationDefinitionsStub{snapshot: activationSnapshot()},
	}

	products, err := factory.openActivatedRuntime(context.Background(), &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: "/factory"},
	})
	if err != nil {
		t.Fatalf("openActivatedRuntime() error = %v", err)
	}
	if root.activations != 1 {
		t.Fatalf("Runtime root activations = %d, want exactly one", root.activations)
	}

	roleCleanups := []struct {
		name  string
		close func() error
	}{
		{name: "application", close: products.application.Resources.Close},
		{name: "invocation", close: products.invocation.CloseArtifacts},
		{name: "execution", close: products.execution.Resources.Close},
	}
	for _, role := range roleCleanups {
		if role.close == nil {
			t.Fatalf("%s cleanup edge = nil, want the Runtime deactivation operation", role.name)
		}
	}
	if root.deactivations != 0 {
		t.Fatalf("Runtime deactivations before cleanup = %d, want zero", root.deactivations)
	}

	for _, role := range roleCleanups {
		if err := role.close(); err != nil {
			t.Fatalf("%s cleanup error = %v", role.name, err)
		}
	}
	if root.deactivations != 1 {
		t.Fatalf(
			"Runtime deactivations after draining every role cleanup = %d, want exactly one Runtime-routed deactivation",
			root.deactivations,
		)
	}

	// Opening publishes the Runtime root itself; it does not hand callers a
	// Sessions-retained runtime handle recovered from the opening products.
	if products.application.FactoryRuntime != factoryruntime.Service(root) {
		t.Fatalf(
			"opened application FactoryRuntime = %T, want the Runtime root %T",
			products.application.FactoryRuntime,
			root,
		)
	}
}

type cleanupRoutingRoot struct {
	factoryruntime.Service
	activations   int
	deactivations int
}

func (root *cleanupRoutingRoot) Activate(
	context.Context,
	factoryruntime.RuntimeActivationRequest,
) (factoryruntime.RuntimeActivationResult, error) {
	root.activations++
	return factoryruntime.RuntimeActivationResult{
		RuntimeID: "runtime-1",
		Runtime: factoryruntime.RuntimeActivationView{
			RuntimeID: "runtime-1",
			Service:   &activatedRuntimeService{products: runtimeProducts{}},
		},
	}, nil
}

func (root *cleanupRoutingRoot) Deactivate(
	context.Context,
	factoryruntime.RuntimeDeactivationRequest,
) (factoryruntime.RuntimeDeactivationResult, error) {
	root.deactivations++
	return factoryruntime.RuntimeDeactivationResult{}, nil
}
