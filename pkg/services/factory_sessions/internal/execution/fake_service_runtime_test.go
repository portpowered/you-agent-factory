// backendsizecheck:ignore-file consolidated JavaScript runtime execution tests remain together until dedicated execution test seams split.
// pkgmaintcheck:ignore-file-lines consolidated JavaScript runtime execution tests remain together until dedicated execution test seams split.
package factorysessionexecution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/checkpointfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/factoryruntimefixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimejavascript "github.com/portpowered/infinite-you/pkg/services/factory_runtime/javascript"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type javaScriptRuntimeServiceConfig struct {
	ProjectRoot        string
	ChildExecutorMode  string
	InvocationExecutor workerexecution.InvocationExecutor
	Persistence        runtimepersist.Store
	Clock              factory.Clock
	Workflows          factory.JavaScriptWorkflows
}

func testRuntimePersistenceStoreFactory(projectRoot string) (runtimepersist.Store, error) {
	return runtimepersist.NewProjectStore(projectRoot, platformfilesystem.Local{})
}

func mustTestRuntimePersistenceStore(t *testing.T, dir string) runtimepersist.Store {
	t.Helper()
	store, err := runtimepersist.NewDirectoryStore(dir, platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewDirectoryStore: %v", err)
	}
	return store
}

func newConfiguredJavaScriptRuntimeService(config javaScriptRuntimeServiceConfig) *JavaScriptRuntimeService {
	workflows := config.Workflows
	if workflows == nil {
		workflows = factoryruntimefixtures.ScriptedJavaScriptWorkflows{}
	}
	clock := config.Clock
	if clock == nil {
		clock = durableFixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	}
	return NewJavaScriptRuntimeService(
		config.ProjectRoot, config.ChildExecutorMode, config.InvocationExecutor,
		config.Persistence, clock, testSyncWaitScheduler{}, checkpointfixtures.CheckpointSummariesFixture{
			BuildResult:  checkpointfixtures.ResumableCheckpointSummaryResult(),
			LatestResult: checkpointfixtures.ResumableCheckpointSummaryResult(),
		},
		workflows, workflows, workflows,
		nil, factory.JavaScriptWorkerSettings{}, mustTestRecordingWriter(),
		testSessionIDGenerator,
		nil, nil, nil,
	)
}

func mustTestRecordingWriter() recordings.PortableRecordingWriter {
	return portableRecordingTestWriter{}
}

type portableRecordingTestWriter struct{}

func (portableRecordingTestWriter) Write(path string, value recordings.PortableRecording) error {
	if err := recordings.ValidatePortableRecording(value); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func int64Ptr(value int64) *int64 {
	return &value
}

func seedRuntimeSessionWithRunningDispatch(
	service *JavaScriptRuntimeService,
	sessionID, dispatchID, label string,
) error {
	if service == nil {
		return NewValidationError("service", "service is required")
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return err
	}
	dispatchID = strings.TrimSpace(dispatchID)
	if dispatchID == "" {
		return NewValidationError("dispatchId", "dispatchId is required")
	}

	now := service.now()
	session := SessionReadResult{
		SessionID:        id,
		Status:           LifecycleStatusRunning,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Phase:            "execute",
		Lifecycle:        &LifecycleTimestamps{StartedAt: &now},
		Links:            InspectionLinksForSession(id, true),
		Progress: &ProgressCounts{
			TotalDispatches:    1,
			InFlightDispatches: 1,
		},
	}
	result := ResultReadResult{
		SessionID:     id,
		SessionStatus: LifecycleStatusRunning,
		ResultStatus:  ResultStatusNotReady,
		Availability: &ResultAvailabilityDetail{
			Reason:    "RESULT_NOT_READY",
			Message:   "Session is still running.",
			Retryable: true,
		},
	}
	dispatches := []DispatchSummary{{
		ID: dispatchID, Status: DispatchStatusRunning, Phase: "execute", Label: label,
	}}
	state := &runtimeSessionState{
		session:    session,
		result:     result,
		dispatches: dispatches,
		dispatchStatusTransitions: map[string][]DispatchStatus{
			dispatchID: {DispatchStatusQueued, DispatchStatusRunning},
		},
	}
	state.events = rebuildRuntimeSessionCanonicalEvents(state)
	service.mu.Lock()
	defer service.mu.Unlock()
	service.sessions[id] = state
	return nil
}

func applyRuntimeTerminalOutcome(
	service *JavaScriptRuntimeService,
	sessionID string,
	outcome factory.JavaScriptRuntimeOutcome,
) error {
	if service == nil {
		return NewValidationError("service", "service is required")
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	state, ok := service.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}
	finishedAt := service.now()
	terminal := runtimeSessionState{
		session: cloneSessionRead(state.session),
		result:  cloneResultRead(state.result),
	}
	if outcome.OK {
		applyRuntimeSuccessProjection(&terminal, id, outcome, finishedAt)
	} else if len(outcome.Records) > 0 {
		applyRuntimeExecutionRecordProjection(&terminal, id, outcome.Records, finishedAt)
		projectRuntimeFailure(&terminal.session, &terminal.result, outcome)
	}
	applyTerminalRuntimeProjection(state, terminal, outcome)
	return nil
}

func TestTaggedDurableHistoryIsAuthoritativeDuringHydrationAndResave(t *testing.T) {
	t.Parallel()
	canonical := json.RawMessage(`{"type":"SESSION_STARTED","context":{"sessionId":"dur-sess-tagged"}}`)
	checkpoint := factory.JavaScriptRuntimeRecord{
		Sequence: 2,
		Kind:     factory.JavaScriptRecordKindCheckpoint,
		Checkpoint: &factory.JavaScriptCheckpointRecord{
			ID: "checkpoint-tagged", State: map[string]any{"position": "review"},
		},
	}
	mutation := interfaces.TokenMutationRecord{
		Type: interfaces.MutationMove, TransitionID: "approve", ToPlace: "review:approved",
	}
	snapshot := PersistedRuntimeSessionState{
		Events:         []json.RawMessage{json.RawMessage(`{"type":"LEGACY_EVENT"}`)},
		RuntimeRecords: []factory.JavaScriptRuntimeRecord{{Sequence: 99, Kind: factory.JavaScriptRecordKindPhase}},
		Records: []DurableSessionRecord{
			{Kind: DurableRecordKindCanonicalFactoryEvent, CanonicalEvent: canonical},
			{Kind: DurableRecordKindJavaScriptRuntime, JavaScriptRecord: &checkpoint},
			{Kind: DurableRecordKindPetriTokenMutation, PetriMutation: &mutation},
		},
	}

	hydrated := runtimeStateFromPersistedSnapshot(snapshot)
	if len(hydrated.events) != 1 || !bytes.Equal(hydrated.events[0], canonical) {
		t.Fatalf("hydrated events = %s, want tagged canonical event", hydrated.events)
	}
	if len(hydrated.runtimeRecords) != 1 || hydrated.runtimeRecords[0].Checkpoint == nil || hydrated.runtimeRecords[0].Checkpoint.ID != "checkpoint-tagged" {
		t.Fatalf("hydrated JavaScript records = %#v, want tagged checkpoint", hydrated.runtimeRecords)
	}
	if len(hydrated.petriMutations) != 1 || hydrated.petriMutations[0].TransitionID != "approve" || hydrated.petriMutations[0].ToPlace != "review:approved" {
		t.Fatalf("hydrated Petri mutations = %#v, want tagged transition", hydrated.petriMutations)
	}

	resaved := persistedSnapshotFromRuntimeState(hydrated)
	if len(resaved.Records) != 3 || resaved.Records[2].PetriMutation == nil || resaved.Records[2].PetriMutation.TransitionID != "approve" {
		t.Fatalf("resaved tagged history = %#v, want lossless mixed records", resaved.Records)
	}
}

func TestProjectResultRead_ModePartialAndFinal(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-js-run-n-001")

	partial, err := service.GetResult(context.Background(), "dur-sess-js-run-n-001", ResultRequest{Mode: ResultModePartial})
	if err != nil {
		t.Fatalf("GetResult partial: %v", err)
	}
	if partial.ResultStatus != ResultStatusPartial {
		t.Fatalf("partial status = %q, want PARTIAL", partial.ResultStatus)
	}
	if len(partial.PrimaryResult) == 0 {
		t.Fatal("partial primaryResult missing")
	}
	if partial.Mode != ResultModePartial {
		t.Fatalf("mode = %q, want partial", partial.Mode)
	}

	final, err := service.GetResult(context.Background(), "dur-sess-js-run-n-001", ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult final: %v", err)
	}
	if final.ResultStatus != ResultStatusNotReady {
		t.Fatalf("final status = %q, want NOT_READY", final.ResultStatus)
	}
	if len(final.PrimaryResult) != 0 {
		t.Fatal("final primaryResult should be omitted for running session")
	}
	if final.Availability == nil || final.Availability.Reason != "RESULT_NOT_READY" {
		t.Fatalf("availability = %#v, want RESULT_NOT_READY", final.Availability)
	}
}

func TestProjectResultRead_TerminalFinalAndUnavailable(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-success-001")

	final, err := service.GetResult(context.Background(), "dur-sess-petri-success-001", ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult terminal final: %v", err)
	}
	if final.ResultStatus != ResultStatusFinal {
		t.Fatalf("status = %q, want FINAL", final.ResultStatus)
	}
	if len(final.PrimaryResult) == 0 {
		t.Fatal("final primaryResult missing")
	}

	startAsyncByRequestID(t, service, "req-petri-cancel-001")
	unavailable, err := service.GetResult(context.Background(), "dur-sess-petri-cancel-001", ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult unavailable: %v", err)
	}
	if unavailable.ResultStatus != ResultStatusUnavailable {
		t.Fatalf("status = %q, want UNAVAILABLE", unavailable.ResultStatus)
	}
	if unavailable.Availability == nil || unavailable.Availability.Reason != "SESSION_CANCELED" {
		t.Fatalf("availability = %#v", unavailable.Availability)
	}
}

func TestProjectResultRead_FailedWithPartialHonorsPartialMode(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-js-failed-partial-001")

	result, err := service.GetResult(context.Background(), "dur-sess-js-failed-partial-001", ResultRequest{Mode: ResultModePartial})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != ResultStatusFailedWithPartial {
		t.Fatalf("status = %q, want FAILED_WITH_PARTIAL", result.ResultStatus)
	}
	if len(result.PrimaryResult) == 0 {
		t.Fatal("partial primaryResult missing")
	}
	if result.Failure == nil || !result.Failure.PartialResultAvailable {
		t.Fatal("failure detail missing")
	}
}

func TestProjectResultRead_IncludeArtifactsShaping(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-success-001")

	excluded, err := service.GetResult(context.Background(), "dur-sess-petri-success-001", ResultRequest{
		Mode:             ResultModeFinal,
		IncludeArtifacts: false,
	})
	if err != nil {
		t.Fatalf("GetResult excluded: %v", err)
	}
	if excluded.IncludeArtifacts {
		t.Fatal("includeArtifacts = true, want false")
	}
	if len(excluded.ArtifactRefs) != 0 {
		t.Fatalf("artifactRefs = %#v, want omitted", excluded.ArtifactRefs)
	}
	if len(excluded.ArtifactIDs) != 1 || excluded.ArtifactIDs[0] != "art-petri-final-001" {
		t.Fatalf("artifactIds = %#v", excluded.ArtifactIDs)
	}

	included, err := service.GetResult(context.Background(), "dur-sess-petri-success-001", ResultRequest{
		Mode:             ResultModeFinal,
		IncludeArtifacts: true,
	})
	if err != nil {
		t.Fatalf("GetResult included: %v", err)
	}
	if !included.IncludeArtifacts {
		t.Fatal("includeArtifacts = false, want true")
	}
	if len(included.ArtifactRefs) != 1 || included.ArtifactRefs[0].ID != "art-petri-final-001" {
		t.Fatalf("artifactRefs = %#v", included.ArtifactRefs)
	}
	if len(included.ArtifactIDs) != 0 {
		t.Fatalf("artifactIds = %#v, want omitted when refs included", included.ArtifactIDs)
	}
}

func TestProjectResultRead_NotReadyRunningSession(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-run-001")

	result, err := service.GetResult(context.Background(), "dur-sess-petri-run-001", ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != ResultStatusNotReady {
		t.Fatalf("status = %q, want NOT_READY", result.ResultStatus)
	}
	if result.Availability == nil || result.Availability.Message == "" {
		t.Fatal("availability missing")
	}
}

func TestProjectResultRead_DefaultsToFinalMode(t *testing.T) {
	t.Parallel()
	canonical := ResultReadResult{
		SessionID:     "dur-sess-001",
		ResultStatus:  ResultStatusFinal,
		SessionStatus: LifecycleStatusSucceeded,
		PrimaryResult: json.RawMessage(`[{"type":"text","text":"done"}]`),
	}
	session := SessionReadResult{
		SessionID: "dur-sess-001",
		Status:    LifecycleStatusSucceeded,
		ResultSummary: &ResultSummary{
			ResultStatus: string(ResultStatusFinal),
		},
	}

	projected, err := ProjectResultRead(canonical, session, nil, ResultRequest{})
	if err != nil {
		t.Fatalf("ProjectResultRead: %v", err)
	}
	if projected.Mode != ResultModeFinal {
		t.Fatalf("mode = %q, want final", projected.Mode)
	}
	if projected.ResultStatus != ResultStatusFinal {
		t.Fatalf("status = %q, want FINAL", projected.ResultStatus)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this helper regression keeps lifecycle mutation assertions together on one scenario.
func TestFakeService_InternalLifecycleHelpers(t *testing.T) {
	t.Parallel()
	state := &fakeSessionState{
		session: SessionReadResult{
			SessionID: "dur-sess-1",
			Status:    LifecycleStatusAwaitingApproval,
		},
		result: ResultReadResult{
			SessionID:     "dur-sess-1",
			SessionStatus: LifecycleStatusAwaitingApproval,
			ResultStatus:  ResultStatusNotReady,
		},
		dispatches: []DispatchSummary{{ID: "disp-1", Status: DispatchStatusFailed, Attempt: 1}},
		dispatchDetails: map[string]DispatchDetail{
			"disp-1": {DispatchSummary: DispatchSummary{ID: "disp-1", Status: DispatchStatusFailed, Attempt: 1}},
		},
	}

	if err := validateLifecycleControlRequest(LifecycleControlApprove, ControlRequest{}, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{}); err != nil {
		t.Fatalf("approve validation: %v", err)
	}
	if err := validateLifecycleControlRequest(LifecycleControlRetryDispatch, ControlRequest{}, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{}); err == nil {
		t.Fatal("retry without dispatch id should fail validation")
	}
	if err := validateLifecycleControlRequest(LifecycleControlInterruptDispatch, ControlRequest{}, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{}); err == nil {
		t.Fatal("interrupt without dispatch id should fail validation")
	}
	if err := validateLifecycleControlRequest(LifecycleControlPause, ControlRequest{}, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{}); err != nil {
		t.Fatalf("pause validation: %v", err)
	}

	accepted := lifecycleControlResultFromState(state, "dur-sess-1", LifecycleControlApprove, LifecycleControlOutcomeAccepted, RetryDispatchRequest{}, InterruptDispatchRequest{})
	if accepted.Session == nil || accepted.Session.Status != LifecycleStatusAwaitingApproval {
		t.Fatalf("accepted lifecycle control result = %#v", accepted)
	}
	noop := lifecycleControlResultFromState(state, "dur-sess-1", LifecycleControlPause, LifecycleControlOutcomeNoOp, RetryDispatchRequest{}, InterruptDispatchRequest{})
	if noop.Session == nil {
		t.Fatalf("noop lifecycle control result = %#v", noop)
	}
	retry := lifecycleControlResultFromState(state, "dur-sess-1", LifecycleControlRetryDispatch, LifecycleControlOutcomeAccepted, RetryDispatchRequest{DispatchID: "disp-1"}, InterruptDispatchRequest{})
	if retry.DispatchID != "disp-1" || retry.RetryDispatchID != "disp-1" {
		t.Fatalf("retry lifecycle control result = %#v", retry)
	}
	interrupt := lifecycleControlResultFromState(state, "dur-sess-1", LifecycleControlInterruptDispatch, LifecycleControlOutcomeAccepted, RetryDispatchRequest{}, InterruptDispatchRequest{DispatchID: "disp-1"})
	if interrupt.DispatchID != "disp-1" {
		t.Fatalf("interrupt lifecycle control result = %#v", interrupt)
	}

	service := &FakeService{}
	service.mutateSessionForControl(state, LifecycleControlApprove, RetryDispatchRequest{}, InterruptDispatchRequest{})
	if state.session.Status != LifecycleStatusRunning {
		t.Fatalf("approve mutate status = %q, want RUNNING", state.session.Status)
	}
	state.session.Status = LifecycleStatusFailed
	state.result.SessionStatus = LifecycleStatusFailed
	service.mutateSessionForControl(state, LifecycleControlRetryDispatch, RetryDispatchRequest{DispatchID: "disp-1"}, InterruptDispatchRequest{})
	if state.session.Status != LifecycleStatusRunning || state.dispatches[0].Status != DispatchStatusQueued || state.dispatches[0].Attempt != 2 {
		t.Fatalf("retry mutate state = %#v / %#v", state.session, state.dispatches[0])
	}
	state.dispatches[0].Status = DispatchStatusRunning
	state.dispatches[0].Attempt = 1
	service.mutateSessionForControl(state, LifecycleControlInterruptDispatch, RetryDispatchRequest{}, InterruptDispatchRequest{DispatchID: "disp-1"})
	if state.dispatches[0].Status != DispatchStatusInterrupted {
		t.Fatalf("interrupt mutate status = %q, want INTERRUPTED", state.dispatches[0].Status)
	}
}

func TestFakeService_InternalStartAndProjectionHelpers(t *testing.T) {
	t.Parallel()
	state := newFakeServiceInternalStartState()
	service := &FakeService{}

	t.Run("start projections", func(t *testing.T) {
		async := service.asyncStartFromState(state)
		if async.SessionID != "dur-sess-1" || async.Policy.EffectiveHash != "policy" {
			t.Fatalf("asyncStartFromState = %#v", async)
		}

		sync := service.syncStartFromState(state)
		if sync.SyncOutcome != SyncOutcomeCompleted || len(sync.Result) == 0 {
			t.Fatalf("syncStartFromState = %#v", sync)
		}
		nonTerminal := *state
		nonTerminal.session.Status = LifecycleStatusRunning
		sync = service.syncStartFromState(&nonTerminal)
		if sync.SyncOutcome != "" || len(sync.Result) != 0 {
			t.Fatalf("non-terminal syncStartFromState = %#v", sync)
		}

		scenarioAsync := service.asyncStartFromScenario(FakeScenario{AsyncStart: &AsyncStartResult{SessionID: "override"}}, state)
		if scenarioAsync.SessionID != "override" {
			t.Fatalf("asyncStartFromScenario = %#v", scenarioAsync)
		}
		scenarioSync := service.syncStartFromScenario(FakeScenario{SyncStart: &SyncStartResult{AsyncStartResult: AsyncStartResult{SessionID: "override-sync"}}}, state)
		if scenarioSync.SessionID != "override-sync" {
			t.Fatalf("syncStartFromScenario = %#v", scenarioSync)
		}
	})

	t.Run("result projections and clones", func(t *testing.T) {
		testFakeServiceInternalResultProjectionHelpers(t, state, service)
	})

	t.Run("sync wait outcome", func(t *testing.T) {
		testFakeServiceInternalSyncWaitHelpers(t, state)
	})
}

func newFakeServiceInternalStartState() *fakeSessionState {
	return &fakeSessionState{
		session: SessionReadResult{
			SessionID:        "dur-sess-1",
			Status:           LifecycleStatusSucceeded,
			OrchestratorKind: "JAVASCRIPT",
			Dialect:          "v1",
			ResolvedSource:   ResolvedSource{Kind: factory.WorkflowSourceKindWorkflowName, SourceRef: "audit", SourceHash: "hash", Dialect: "v1"},
			SourceHash:       "hash",
			Policy:           PolicyProjection{EffectiveHash: "policy"},
			Links:            InspectionLinks{Session: "/factory-sessions/dur-sess-1"},
		},
		result: ResultReadResult{
			SessionID:        "dur-sess-1",
			ResultStatus:     ResultStatusFinal,
			SessionStatus:    LifecycleStatusSucceeded,
			PrimaryResult:    json.RawMessage(`[{"type":"text","text":"done"}]`),
			Availability:     &ResultAvailabilityDetail{Reason: "IGNORED"},
			IncludeArtifacts: true,
		},
		artifacts: []ArtifactSummary{
			{ID: "art-1", Kind: "LOG", Visibility: "PUBLIC", ContentHash: "hash-1", SizeBytes: 7},
			{ID: " ", Kind: "LOG", Visibility: "PUBLIC"},
		},
		events: []json.RawMessage{json.RawMessage(`{"id":"event-1"}`)},
	}
}

func testFakeServiceInternalResultProjectionHelpers(t *testing.T, state *fakeSessionState, service *FakeService) {
	t.Helper()

	async := service.asyncStartFromState(state)
	sync := service.syncStartFromState(state)

	t.Run("projection modes", func(t *testing.T) {
		testFakeServiceInternalResultProjectionModes(t, state)
	})
	t.Run("helper branches and clones", func(t *testing.T) {
		testFakeServiceInternalResultHelperBranches(t, state, async, sync)
	})
}

func testFakeServiceInternalResultProjectionModes(t *testing.T, state *fakeSessionState) {
	t.Helper()

	canonical := ResultReadResult{
		SessionID:     "dur-sess-1",
		ResultStatus:  ResultStatusPartial,
		PrimaryResult: json.RawMessage(`[{"type":"text","text":"partial"}]`),
		Failure:       &FailureSummary{Reason: "warn", PartialResultAvailable: true},
	}
	session := SessionReadResult{
		SessionID: "dur-sess-1",
		Status:    LifecycleStatusRunning,
		ResultSummary: &ResultSummary{
			ResultStatus: string(ResultStatusPartial),
		},
	}
	projected, err := ProjectResultRead(canonical, session, state.artifacts, ResultRequest{Mode: ResultModeFinal, IncludeArtifacts: true})
	if err != nil {
		t.Fatalf("ProjectResultRead final: %v", err)
	}
	if projected.ResultStatus != ResultStatusNotReady || projected.Availability == nil || len(projected.ArtifactRefs) != 2 {
		t.Fatalf("projected final = %#v", projected)
	}

	projected, err = ProjectResultRead(canonical, session, state.artifacts, ResultRequest{Mode: ResultModePartial})
	if err != nil {
		t.Fatalf("ProjectResultRead partial: %v", err)
	}
	if projected.ResultStatus != ResultStatusPartial || len(projected.PrimaryResult) == 0 || projected.Availability != nil {
		t.Fatalf("projected partial = %#v", projected)
	}
	if _, err := ProjectResultRead(canonical, session, nil, ResultRequest{Mode: ResultMode("bad")}); err == nil {
		t.Fatal("invalid mode should fail normalization")
	}
}

func testFakeServiceInternalResultHelperBranches(t *testing.T, state *fakeSessionState, async AsyncStartResult, sync SyncStartResult) {
	t.Helper()

	t.Run("status and artifact helpers", func(t *testing.T) {
		testFakeServiceInternalResultStatusAndArtifactHelpers(t, state)
	})
	t.Run("clone helpers", func(t *testing.T) {
		testFakeServiceInternalResultCloneHelpers(t, async, sync)
	})
}

func testFakeServiceInternalResultStatusAndArtifactHelpers(t *testing.T, state *fakeSessionState) {
	t.Helper()

	if got := canonicalResultStatus(ResultReadResult{ResultStatus: ResultStatusUnavailable}, SessionReadResult{
		ResultSummary: &ResultSummary{ResultStatus: " FINAL "},
	}); got != ResultStatusFinal {
		t.Fatalf("canonicalResultStatus = %q", got)
	}
	if got := defaultNotReadyAvailability(SessionReadResult{Status: LifecycleStatusRunning}); got == nil || !got.Retryable {
		t.Fatalf("running default availability = %#v", got)
	}
	if got := defaultNotReadyAvailability(SessionReadResult{Status: LifecycleStatusSucceeded}); got == nil || got.Retryable {
		t.Fatalf("terminal default availability = %#v", got)
	}

	if refs := artifactRefsFromSummaries(nil); refs != nil {
		t.Fatalf("artifactRefsFromSummaries(nil) = %#v", refs)
	}
	refs := artifactRefsFromSummaries(state.artifacts)
	if len(refs) != 2 || refs[0].ID != "art-1" {
		t.Fatalf("artifactRefsFromSummaries = %#v", refs)
	}
	ids := artifactIDsFromSummaries(state.artifacts)
	if len(ids) != 1 || ids[0] != "art-1" {
		t.Fatalf("artifactIDsFromSummaries = %#v", ids)
	}
}

func testFakeServiceInternalResultCloneHelpers(t *testing.T, async AsyncStartResult, sync SyncStartResult) {
	t.Helper()

	canonicalFailure := &FailureSummary{Reason: "warn", PartialResultAvailable: true}

	if cloneFailureSummary(nil) != nil || cloneResultAvailability(nil) != nil || cloneRawJSON(nil) != nil {
		t.Fatal("nil clones should stay nil")
	}
	if clone := cloneFailureSummary(canonicalFailure); clone == canonicalFailure || clone.Reason != "warn" {
		t.Fatalf("cloneFailureSummary = %#v", clone)
	}
	if clone := cloneResultAvailability(&ResultAvailabilityDetail{Reason: "NOT_READY"}); clone == nil || clone.Reason != "NOT_READY" {
		t.Fatalf("cloneResultAvailability = %#v", clone)
	}
	if clone := cloneAsyncStartResult(async); clone.SessionID != async.SessionID {
		t.Fatalf("cloneAsyncStartResult = %#v", clone)
	}
	if clone := cloneSyncStartResult(sync); clone.SessionID != sync.SessionID {
		t.Fatalf("cloneSyncStartResult = %#v", clone)
	}
	if clone := cloneRawJSON(json.RawMessage(`{"ok":true}`)); string(clone) != `{"ok":true}` {
		t.Fatalf("cloneRawJSON = %s", clone)
	}
}

func testFakeServiceInternalSyncWaitHelpers(t *testing.T, state *fakeSessionState) {
	t.Helper()

	applySyncWaitOutcome(nil, state, StartRequest{})
	applySyncWaitOutcome(&SyncStartResult{}, nil, StartRequest{})
	timeout := SyncStartResult{AsyncStartResult: AsyncStartResult{Status: string(LifecycleStatusRunning)}, SyncOutcome: SyncOutcomeTimedOut, TimedOut: true}
	runningState := &fakeSessionState{
		session: SessionReadResult{SessionID: "dur-sess-2", Status: LifecycleStatusRunning},
		result:  ResultReadResult{SessionID: "dur-sess-2", SessionStatus: LifecycleStatusRunning, ResultStatus: ResultStatusNotReady},
	}
	applySyncWaitOutcome(&timeout, runningState, StartRequest{
		Wait: &WaitOptions{CancelOnTimeout: true},
	})
	if !timeout.SessionCanceledByTimeout || timeout.Status != string(LifecycleStatusCanceling) || runningState.result.Availability == nil {
		t.Fatalf("applySyncWaitOutcome timeout = %#v / %#v", timeout, runningState.result)
	}
}

func TestIsTerminalLifecycleStatus(t *testing.T) {
	t.Parallel()
	terminal := []LifecycleStatus{
		LifecycleStatusSucceeded,
		LifecycleStatusFailed,
		LifecycleStatusCanceled,
		LifecycleStatusTimedOut,
		LifecycleStatusInterrupted,
		LifecycleStatusTerminated,
	}
	for _, status := range terminal {
		if !IsTerminalLifecycleStatus(status) {
			t.Fatalf("status %q should be terminal", status)
		}
		if status != LifecycleStatusFailed && AllowsRetryDispatchOnTerminal(status) {
			t.Fatalf("retry-dispatch should be rejected on terminal status %q", status)
		}
	}
	if !AllowsRetryDispatchOnTerminal(LifecycleStatusFailed) {
		t.Fatal("retry-dispatch should remain allowed on FAILED terminal sessions")
	}
	active := []LifecycleStatus{
		LifecycleStatusRunning,
		LifecycleStatusPaused,
		LifecycleStatusCanceling,
	}
	for _, status := range active {
		if IsTerminalLifecycleStatus(status) {
			t.Fatalf("status %q should be active", status)
		}
	}
}

func TestEvaluateLifecycleControl_ValidTransitions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		operation LifecycleControlKind
		status    LifecycleStatus
		want      LifecycleControlOutcome
	}{
		{LifecycleControlPause, LifecycleStatusRunning, LifecycleControlOutcomeAccepted},
		{LifecycleControlPause, LifecycleStatusPaused, LifecycleControlOutcomeNoOp},
		{LifecycleControlResume, LifecycleStatusPaused, LifecycleControlOutcomeAccepted},
		{LifecycleControlResume, LifecycleStatusInterrupted, LifecycleControlOutcomeAccepted},
		{LifecycleControlResume, LifecycleStatusRunning, LifecycleControlOutcomeNoOp},
		{LifecycleControlCancel, LifecycleStatusRunning, LifecycleControlOutcomeAccepted},
		{LifecycleControlCancel, LifecycleStatusCanceling, LifecycleControlOutcomeNoOp},
		{LifecycleControlTerminate, LifecycleStatusRunning, LifecycleControlOutcomeAccepted},
		{LifecycleControlApprove, LifecycleStatusAwaitingApproval, LifecycleControlOutcomeAccepted},
		{LifecycleControlRetryDispatch, LifecycleStatusRunning, LifecycleControlOutcomeAccepted},
		{LifecycleControlRetryDispatch, LifecycleStatusFailed, LifecycleControlOutcomeAccepted},
	}
	for _, tc := range cases {
		got := EvaluateLifecycleControl(tc.operation, tc.status)
		if got != tc.want {
			t.Fatalf("%s on %s = %q, want %q", tc.operation, tc.status, got, tc.want)
		}
	}
}

func TestEvaluateLifecycleControl_InvalidAndTerminal(t *testing.T) {
	t.Parallel()
	if got := EvaluateLifecycleControl(LifecycleControlPause, LifecycleStatusAwaitingApproval); got != LifecycleControlOutcomeInvalidState {
		t.Fatalf("pause on awaiting approval = %q, want INVALID_STATE", got)
	}
	if got := EvaluateLifecycleControl(LifecycleControlRetryDispatch, LifecycleStatusSucceeded); got != LifecycleControlOutcomeTerminalSession {
		t.Fatalf("retry on succeeded = %q, want TERMINAL_SESSION", got)
	}
	if got := EvaluateLifecycleControl(LifecycleControlCancel, LifecycleStatusCanceled); got != LifecycleControlOutcomeNoOp {
		t.Fatalf("cancel on canceled = %q, want NO_OP", got)
	}
}

func TestNormalizeRetryDispatchRequest_RequiresDispatchID(t *testing.T) {
	t.Parallel()
	_, err := NormalizeRetryDispatchRequest(RetryDispatchRequest{})
	if err == nil {
		t.Fatal("error = nil, want validation error")
	}
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T, want ValidationError", err)
	}
}

func TestControlIdempotencyTupleHash_IsStable(t *testing.T) {
	t.Parallel()
	retry := RetryDispatchRequest{
		ControlRequest: ControlRequest{RequestID: "req-retry-001"},
		DispatchID:     "disp-js-success-002",
	}
	first, err := ControlIdempotencyTupleHash(LifecycleControlRetryDispatch, "dur-sess-js-success-002", ApproveRequest{}, retry, InterruptDispatchRequest{})
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	second, err := ControlIdempotencyTupleHash(LifecycleControlRetryDispatch, "dur-sess-js-success-002", ApproveRequest{}, retry, InterruptDispatchRequest{})
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if first != second {
		t.Fatalf("hash mismatch: %q vs %q", first, second)
	}
}

func TestCheckControlRequestIDReplay_Conflict(t *testing.T) {
	t.Parallel()
	err := CheckControlRequestIDReplay("req-1", "sha256:abc", "sha256:def")
	if !errors.Is(err, ErrControlRequestIDConflict) {
		t.Fatalf("error = %v, want ErrControlRequestIDConflict", err)
	}
}

func TestServiceMethods_PropagateContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var service interface {
		GetSession(context.Context, string) (SessionReadResult, error)
	}
	service = stubCancelAwareService{}
	if _, err := service.GetSession(ctx, "dur-sess-001"); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetSession error = %v, want context.Canceled", err)
	}
}

type stubCancelAwareService struct{}

func (stubCancelAwareService) GetSession(ctx context.Context, _ string) (SessionReadResult, error) {
	if err := ctx.Err(); err != nil {
		return SessionReadResult{}, err
	}
	return SessionReadResult{}, nil
}

func (stubCancelAwareService) StartAsync(context.Context, StartRequest) (AsyncStartResult, error) {
	return AsyncStartResult{}, nil
}
func (stubCancelAwareService) StartSync(context.Context, StartRequest) (SyncStartResult, error) {
	return SyncStartResult{}, nil
}
func (stubCancelAwareService) Pause(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}
func (stubCancelAwareService) Resume(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}
func (stubCancelAwareService) Cancel(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}
func (stubCancelAwareService) Terminate(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}
func (stubCancelAwareService) Approve(context.Context, string, ApproveRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}
func (stubCancelAwareService) RetryDispatch(context.Context, string, RetryDispatchRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}
func (stubCancelAwareService) InterruptDispatch(context.Context, string, InterruptDispatchRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}
func (stubCancelAwareService) GetResult(context.Context, string, ResultRequest) (ResultReadResult, error) {
	return ResultReadResult{}, nil
}
func (stubCancelAwareService) ListDispatches(context.Context, string) (ListDispatchesResult, error) {
	return ListDispatchesResult{}, nil
}
func (stubCancelAwareService) GetDispatch(context.Context, string, string) (DispatchDetail, error) {
	return DispatchDetail{}, nil
}
func (stubCancelAwareService) ListArtifacts(context.Context, string) (ListArtifactsResult, error) {
	return ListArtifactsResult{}, nil
}
func (stubCancelAwareService) GetArtifact(context.Context, string, string) (ArtifactDetail, error) {
	return ArtifactDetail{}, nil
}
func (stubCancelAwareService) ReadEvents(context.Context, string, EventReconnectRequest) (EventReadResult, error) {
	return EventReadResult{}, nil
}

func (stubCancelAwareService) ListSessions(context.Context, ListSessionsRequest) (ListSessionsResult, error) {
	return ListSessionsResult{}, nil
}

const simpleFinalWorkflowSource = `return {
  label: meta.name,
  description: meta.description,
  subject: args.subject,
  repeat: args.count,
  echo: args.prefix + ":" + args.subject,
};
`

const busyLoopWorkflowSource = `while (true) {}`

const throwErrorWorkflowSource = `throw new Error("workflow execution failed: " + args.subject);`

const progressThenFinalWorkflowSource = `
phase("execute");
const artifactRef = workflow.artifact({
  kind: "log",
  label: "unpersisted-output",
  content: { message: "must roll back" },
});
workflow.checkpoint({
  label: "before-final",
  state: { artifactRef: artifactRef },
});
return { artifactRef: artifactRef };
`

type durableFixedClock struct{ now time.Time }

func (c durableFixedClock) Now() time.Time { return c.now }

func TestJavaScriptRuntimeService_StartSync_UsesInjectedClock(t *testing.T) {
	t.Parallel()
	want := time.Date(2031, time.April, 5, 6, 7, 8, 0, time.FixedZone("offset", -7*60*60))
	projectRoot := t.TempDir()
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Clock:       durableFixedClock{now: want},
	})

	started, err := service.StartSync(context.Background(), inlineWorkflowStartRequest(
		"req-runtime-clock-001",
		simpleFinalWorkflowSource,
		map[string]any{"subject": "clock", "count": 1, "prefix": "fixed"},
		nil,
	))
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	read, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	wantUTC := want.UTC()
	if read.Lifecycle == nil || read.Lifecycle.StartedAt == nil || !read.Lifecycle.StartedAt.Equal(wantUTC) {
		t.Fatalf("startedAt = %#v, want %s", read.Lifecycle, wantUTC)
	}
	if read.Lifecycle.FinishedAt == nil || !read.Lifecycle.FinishedAt.Equal(wantUTC) {
		t.Fatalf("finishedAt = %#v, want %s", read.Lifecycle, wantUTC)
	}
}

func TestJavaScriptRuntimeService_StartSync_SimpleWorkflowCompletesWithPrimaryResult(t *testing.T) {
	t.Parallel()
	service := newDefaultJavaScriptRuntimeService(t, scriptedSuccessfulRuntimeWorkflows(map[string]any{
		"label":       "runtime-sync-fixture",
		"description": "runtime sync fixture",
		"subject":     "workflows",
		"repeat":      float64(3),
		"echo":        "you:workflows",
	}))

	started, err := service.StartSync(context.Background(), inlineWorkflowStartRequest(
		"req-runtime-sync-simple-final-001",
		simpleFinalWorkflowSource,
		map[string]any{
			"subject": "workflows",
			"count":   3,
			"prefix":  "you",
		},
		nil,
	))
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if started.SyncOutcome != SyncOutcomeCompleted {
		t.Fatalf("syncOutcome = %q, want COMPLETED", started.SyncOutcome)
	}
	if started.Status != string(LifecycleStatusSucceeded) {
		t.Fatalf("status = %q, want SUCCEEDED", started.Status)
	}
	if started.OrchestratorKind != interfaces.OrchestratorKindJavaScript {
		t.Fatalf("orchestratorKind = %q, want JAVASCRIPT", started.OrchestratorKind)
	}
	if started.SessionID == "" || started.ResolvedSource.SourceRef == "" || started.SourceHash == "" {
		t.Fatalf("start result missing resolved source metadata: %#v", started)
	}

	testJavaScriptRuntimeSyncCompletedSession(t, service, started.SessionID)
	testJavaScriptRuntimeSyncCompletedResult(t, service, started.SessionID)
	testJavaScriptRuntimeSyncCompletedEvents(t, service, started.SessionID)
}

func TestJavaScriptRuntimeService_StartSync_PersistenceFailureDoesNotPublishSuccess(t *testing.T) {
	t.Parallel()
	store := &runtimeRecordingStore{saveErr: errors.New("append unavailable")}
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Persistence: store,
	})

	started, err := service.StartSync(context.Background(), inlineWorkflowStartRequest(
		"req-runtime-sync-persist-failure-001",
		simpleFinalWorkflowSource,
		nil,
		nil,
	))
	if err == nil || !strings.Contains(err.Error(), "append unavailable") {
		t.Fatalf("StartSync result = %#v, error = %v, want persistence failure", started, err)
	}

	store.mu.Lock()
	saveCalls := store.saveCalls
	store.mu.Unlock()
	if saveCalls != 1 {
		t.Fatalf("persistence save calls = %d, want exactly one", saveCalls)
	}
	service.mu.RLock()
	defer service.mu.RUnlock()
	for _, state := range service.sessions {
		if state.session.Status == LifecycleStatusSucceeded || state.result.ResultStatus == ResultStatusFinal {
			t.Fatalf("unpersisted success became live: session=%#v result=%#v", state.session, state.result)
		}
	}
}

func TestJavaScriptRuntimeService_StartAsync_PersistenceFailurePublishesFailureNotSuccess(t *testing.T) {
	t.Parallel()
	store := &runtimeRecordingStore{saveErr: errors.New("append unavailable")}
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Persistence: store,
	})

	started, err := service.StartAsync(context.Background(), inlineWorkflowStartRequest(
		"req-runtime-async-persist-failure-001",
		progressThenFinalWorkflowSource,
		nil,
		nil,
	))
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	failed := waitUntilSessionStatus(t, service, started.SessionID, LifecycleStatusFailed, 2*time.Second)
	if failed.Failure == nil || !strings.Contains(failed.Failure.Message, "append unavailable") {
		t.Fatalf("failure = %#v, want explicit persistence failure", failed.Failure)
	}
	assertPersistenceFailureRolledBackLiveProjections(t, service, started.SessionID)

	store.mu.Lock()
	saveCalls := store.saveCalls
	store.mu.Unlock()
	if saveCalls != 1 {
		t.Fatalf("persistence save calls = %d, want exactly one", saveCalls)
	}
}

func assertPersistenceFailureRolledBackLiveProjections(
	t *testing.T,
	service *JavaScriptRuntimeService,
	sessionID string,
) {
	t.Helper()
	result, err := service.GetResult(context.Background(), sessionID, ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus == ResultStatusFinal {
		t.Fatalf("unpersisted terminal result status = %q, want unavailable", result.ResultStatus)
	}
	dispatches, err := service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 0 {
		t.Fatalf("unpersisted dispatches = %#v, want none", dispatches.Dispatches)
	}
	artifacts, err := service.ListArtifacts(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(artifacts.Artifacts) != 0 {
		t.Fatalf("unpersisted artifacts = %#v, want none", artifacts.Artifacts)
	}
	events, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	encodedEvents, err := json.Marshal(events.Events)
	if err != nil {
		t.Fatalf("marshal events: %v", err)
	}
	if bytes.Contains(encodedEvents, []byte("unpersisted-output")) || bytes.Contains(encodedEvents, []byte("checkpoint")) {
		t.Fatalf("event history retained unpersisted terminal records: %s", encodedEvents)
	}
	assertPersistenceFailureClearedInternalRuntimeState(t, service, sessionID)
}

func assertPersistenceFailureClearedInternalRuntimeState(t *testing.T, service *JavaScriptRuntimeService, sessionID string) {
	t.Helper()
	service.mu.RLock()
	live := cloneRuntimeSessionState(service.sessions[sessionID])
	service.mu.RUnlock()
	if live.session.Phase != "" || live.session.Progress != nil || len(live.session.ArtifactRefs) != 0 {
		t.Fatalf("session retained unpersisted projection: %#v", live.session)
	}
	if len(live.runtimeRecords) != 0 || live.checkpointSummary != nil {
		t.Fatalf("runtime state retained unpersisted records: records=%#v checkpoint=%#v", live.runtimeRecords, live.checkpointSummary)
	}
}

type runtimeRecordingStore struct {
	mu        sync.Mutex
	saveCalls int
	saveErr   error
	payload   []byte
}

func (s *runtimeRecordingStore) Save(_ string, encoded []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCalls++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.payload = append([]byte(nil), encoded...)
	return nil
}

func (s *runtimeRecordingStore) Load(string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.payload) == 0 {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), s.payload...), nil
}

func TestJavaScriptRuntimeService_StartAsync_RunningCancelAndReads(t *testing.T) {
	t.Parallel()
	service := newDefaultJavaScriptRuntimeService(t, scriptedBlockingRuntimeWorkflows())

	started, err := service.StartAsync(context.Background(), inlineWorkflowStartRequest(
		"req-runtime-async-running-001",
		busyLoopWorkflowSource,
		map[string]any{"subject": "workflows"},
		nil,
	))
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.Status != string(LifecycleStatusRunning) {
		t.Fatalf("start status = %q, want RUNNING", started.Status)
	}

	session, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Status != LifecycleStatusRunning {
		t.Fatalf("session status = %q, want RUNNING", session.Status)
	}

	result, err := service.GetResult(context.Background(), started.SessionID, ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != ResultStatusNotReady {
		t.Fatalf("resultStatus = %q, want NOT_READY", result.ResultStatus)
	}
	if result.Availability == nil || result.Availability.Reason != "RESULT_NOT_READY" {
		t.Fatalf("availability = %#v, want RESULT_NOT_READY", result.Availability)
	}

	dispatches, err := service.ListDispatches(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 0 {
		t.Fatalf("dispatches = %#v, want none for busy loop workflow", dispatches.Dispatches)
	}

	canceled, err := service.Cancel(context.Background(), started.SessionID, ControlRequest{})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if canceled.Outcome != LifecycleControlOutcomeAccepted {
		t.Fatalf("cancel outcome = %q, want ACCEPTED", canceled.Outcome)
	}

	finalSession := waitUntilSessionStatus(t, service, started.SessionID, LifecycleStatusCanceled, 5*time.Second)
	if finalSession.Failure == nil || finalSession.Failure.Reason != "WORKFLOW_RUNTIME_CANCELED" {
		t.Fatalf("failure = %#v, want WORKFLOW_RUNTIME_CANCELED", finalSession.Failure)
	}
}

func TestJavaScriptRuntimeService_StartAsync_FailedAndTimedOut(t *testing.T) {
	t.Parallel()
	t.Run("failed", func(t *testing.T) {
		service := newDefaultJavaScriptRuntimeService(t, scriptedFailedRuntimeWorkflows())
		started, err := service.StartAsync(context.Background(), inlineWorkflowStartRequest(
			"req-runtime-async-failed-001",
			throwErrorWorkflowSource,
			map[string]any{"subject": "workflows"},
			nil,
		))
		if err != nil {
			t.Fatalf("StartAsync: %v", err)
		}

		session := waitUntilSessionStatus(t, service, started.SessionID, LifecycleStatusFailed, 5*time.Second)
		if session.Failure == nil || session.Failure.Reason == "" {
			t.Fatalf("failure = %#v, want runtime failure summary", session.Failure)
		}

		result, err := service.GetResult(context.Background(), started.SessionID, ResultRequest{Mode: ResultModeFinal})
		if err != nil {
			t.Fatalf("GetResult: %v", err)
		}
		if result.ResultStatus != ResultStatusUnavailable || result.SessionStatus != LifecycleStatusFailed {
			t.Fatalf("result = %#v, want unavailable failed result", result)
		}
	})

	t.Run("timed out", func(t *testing.T) {
		service := newDefaultJavaScriptRuntimeService(t, scriptedBlockingRuntimeWorkflows())
		maxRunDurationMs := int64(50)
		started, err := service.StartAsync(context.Background(), inlineWorkflowStartRequest(
			"req-runtime-async-timeout-001",
			busyLoopWorkflowSource,
			map[string]any{"subject": "workflows"},
			map[string]any{"maxRunDurationMs": maxRunDurationMs},
		))
		if err != nil {
			t.Fatalf("StartAsync: %v", err)
		}

		session := waitUntilSessionStatus(t, service, started.SessionID, LifecycleStatusTimedOut, 5*time.Second)
		if session.Failure == nil || session.Failure.Reason != "WORKFLOW_RUNTIME_TIMEOUT" {
			t.Fatalf("failure = %#v, want WORKFLOW_RUNTIME_TIMEOUT", session.Failure)
		}

		result, err := service.GetResult(context.Background(), started.SessionID, ResultRequest{Mode: ResultModeFinal})
		if err != nil {
			t.Fatalf("GetResult: %v", err)
		}
		if result.ResultStatus != ResultStatusUnavailable || result.SessionStatus != LifecycleStatusTimedOut {
			t.Fatalf("result = %#v, want unavailable timed out result", result)
		}
	})
}

func TestJavaScriptRuntimeService_StartSync_WaitTimeoutWithoutCancelKeepsSessionRunning(t *testing.T) {
	t.Parallel()
	service := newDefaultJavaScriptRuntimeService(t, scriptedBlockingRuntimeWorkflows())
	waitMillis := int64(50)

	started, err := service.StartSync(context.Background(), StartRequest{
		RequestID: "req-runtime-sync-wait-timeout-001",
		Source: Source{
			Kind: factory.WorkflowSourceKindInlineWorkflow,
			InlineWorkflow: &InlineWorkflowSource{
				InlineSource: busyLoopWorkflowSource,
				Dialect:      "you-workflow-v1",
				Metadata:     map[string]string{"name": "runtime-sync-wait-fixture"},
			},
		},
		Args: map[string]any{"subject": "workflows"},
		Wait: &WaitOptions{TimeoutMillis: &waitMillis},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if started.SyncOutcome != SyncOutcomeTimedOut || !started.TimedOut {
		t.Fatalf("sync response = %#v, want TIMED_OUT", started)
	}
	if started.SessionCanceledByTimeout {
		t.Fatal("sessionCanceledByTimeout = true, want false")
	}
	if started.Status != string(LifecycleStatusRunning) {
		t.Fatalf("status = %q, want RUNNING", started.Status)
	}

	session, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Status != LifecycleStatusRunning {
		t.Fatalf("session status = %q, want RUNNING after sync wait timeout", session.Status)
	}

	result, err := service.GetResult(context.Background(), started.SessionID, ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.Availability == nil || result.Availability.Reason != "SYNC_WAIT_TIMED_OUT" {
		t.Fatalf("availability = %#v, want SYNC_WAIT_TIMED_OUT", result.Availability)
	}
}

func TestExecutionServiceAndHelperNormalization(t *testing.T) {
	t.Parallel()
	t.Run("execution service providers", testExecutionServiceProviders)
	t.Run("explicit persistence choices", testExecutionServicePersistenceChoices)
	t.Run("child executor and smoke provider", testExecutionServiceChildExecutorHelpers)
	t.Run("source request helpers", testExecutionServiceSourceRequestHelpers)
}

func testExecutionServiceProviders(t *testing.T) {
	t.Helper()

	fakeService, err := newExecutionService(ExecutionProviderFake, serviceConfig{})
	if err != nil {
		t.Fatalf("NewExecutionService(fake): %v", err)
	}
	if _, ok := fakeService.(*FakeService); !ok {
		t.Fatalf("fake provider type = %T, want *FakeService", fakeService)
	}

	projectRoot := t.TempDir()
	persistence, err := ProjectPersistence(projectRoot, testRuntimePersistenceStoreFactory)
	if err != nil {
		t.Fatalf("ProjectPersistence: %v", err)
	}
	runtimeService, err := newExecutionService(ExecutionProviderJavaScriptRuntime, serviceConfig{ProjectRoot: projectRoot, Persistence: persistence, Clock: durableFixedClock{now: time.Now()}})
	if err != nil {
		t.Fatalf("NewExecutionService(runtime): %v", err)
	}
	jsService, ok := runtimeService.(*JavaScriptRuntimeService)
	if !ok {
		t.Fatalf("runtime provider type = %T, want *JavaScriptRuntimeService", runtimeService)
	}
	if jsService.persistence == nil {
		t.Fatal("expected runtime service to use the injected persisted session store")
	}

	if _, err := newExecutionService(ExecutionProviderJavaScriptRuntime, serviceConfig{}); err == nil {
		t.Fatal("NewExecutionService(runtime without projectRoot) error = nil, want validation error")
	}
	if _, err := newExecutionService(ExecutionProvider("unknown"), serviceConfig{}); err == nil {
		t.Fatal("NewExecutionService(unknown) error = nil, want validation error")
	}
}

func testExecutionServicePersistenceChoices(t *testing.T) {
	t.Helper()
	projectRoot := t.TempDir()
	testApplicationPersistencePolicies(t, projectRoot)
	testExecutionServiceRequiredPersistenceDependencies(t, projectRoot)
	testExecutionServiceDisabledPersistence(t, projectRoot)
	testExecutionServiceInvalidPersistenceChoices(t, projectRoot)
}

func testExecutionServiceRequiredPersistenceDependencies(t *testing.T, projectRoot string) {
	t.Helper()
	if _, err := newExecutionService(ExecutionProviderJavaScriptRuntime, serviceConfig{
		ProjectRoot: projectRoot,
		Persistence: DisabledPersistence(),
	}); err == nil {
		t.Fatal("NewExecutionService(runtime without clock) error = nil, want validation error")
	} else if validation, ok := err.(*ValidationError); !ok || validation.Field != "clock" {
		t.Fatalf("runtime without clock error = %#v, want clock ValidationError", err)
	}
	if _, err := newExecutionService(ExecutionProviderJavaScriptRuntime, serviceConfig{ProjectRoot: projectRoot}); err == nil {
		t.Fatal("NewExecutionService(runtime without persistence choice) error = nil, want validation error")
	}
}

func testExecutionServiceDisabledPersistence(t *testing.T, projectRoot string) {
	t.Helper()
	disabled, err := newExecutionService(ExecutionProviderJavaScriptRuntime, serviceConfig{
		ProjectRoot: projectRoot,
		Persistence: DisabledPersistence(),
		Clock:       durableFixedClock{now: time.Now()},
	})
	if err != nil {
		t.Fatalf("NewExecutionService(runtime with disabled persistence): %v", err)
	}
	if disabled.(*JavaScriptRuntimeService).persistence != nil {
		t.Fatal("disabled persistence unexpectedly configured a store")
	}
	if _, err := newExecutionService(ExecutionProviderJavaScriptRuntime, serviceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: ChildExecutorModeLive,
		Persistence:       DisabledPersistence(),
		Clock:             durableFixedClock{now: time.Now()},
	}); err == nil {
		t.Fatal("NewExecutionService(live runtime without provider) error = nil, want validation error")
	} else if validation, ok := err.(*ValidationError); !ok || validation.Field != "runtime.childExecutorMode" {
		t.Fatalf("live runtime without provider error = %#v, want runtime.childExecutorMode ValidationError", err)
	}
}

func testExecutionServiceInvalidPersistenceChoices(t *testing.T, projectRoot string) {
	t.Helper()
	contradictory := PersistenceChoice{store: mustTestRuntimePersistenceStore(t, t.TempDir()), disabled: true}
	if _, err := newExecutionService(ExecutionProviderJavaScriptRuntime, serviceConfig{
		ProjectRoot: projectRoot,
		Persistence: contradictory,
		Clock:       durableFixedClock{now: time.Now()},
	}); err == nil {
		t.Fatal("NewExecutionService(runtime with contradictory persistence) error = nil, want validation error")
	} else if validation, ok := err.(*ValidationError); !ok || validation.Field != "persistence" {
		t.Fatalf("contradictory persistence error = %#v, want persistence ValidationError", err)
	}

	blockedRoot := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockedRoot, []byte("blocked"), 0o600); err != nil {
		t.Fatalf("write blocked persistence root: %v", err)
	}
	if _, err := ProjectPersistence(blockedRoot, testRuntimePersistenceStoreFactory); err == nil {
		t.Fatal("ProjectPersistence(unavailable root) error = nil, want validation error")
	} else if validation, ok := err.(*ValidationError); !ok || validation.Field != "persistence" {
		t.Fatalf("unavailable persistence error = %#v, want persistence ValidationError", err)
	}
}

func testApplicationPersistencePolicies(t *testing.T, projectRoot string) {
	t.Helper()
	for _, tc := range []struct {
		name     string
		policy   PersistencePolicy
		disabled bool
	}{
		{name: "default enabled"},
		{name: "enabled", policy: PersistencePolicyEnabled},
		{name: "disabled", policy: PersistencePolicyDisabled, disabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			choice, err := PersistenceChoiceForPolicy(tc.policy, projectRoot, testRuntimePersistenceStoreFactory)
			if err != nil {
				t.Fatalf("PersistenceChoiceForPolicy: %v", err)
			}
			store, err := choice.resolve()
			if err != nil {
				t.Fatalf("resolve policy choice: %v", err)
			}
			if (store == nil) != tc.disabled {
				t.Fatalf("store nil = %t, want disabled = %t", store == nil, tc.disabled)
			}
		})
	}
	if _, err := PersistenceChoiceForPolicy(PersistencePolicy("invalid"), projectRoot, testRuntimePersistenceStoreFactory); err == nil {
		t.Fatal("PersistenceChoiceForPolicy(invalid) error = nil, want validation error")
	} else if validation, ok := err.(*ValidationError); !ok || validation.Field != "persistence.policy" {
		t.Fatalf("invalid policy error = %#v, want persistence.policy ValidationError", err)
	}
}

func testExecutionServiceChildExecutorHelpers(t *testing.T) {
	t.Helper()

	if err := validateLiveChildProviderExecutor(ChildExecutorModeLive, nil); err == nil {
		t.Fatal("validateLiveChildProviderExecutor(live,nil) error = nil, want validation error")
	}
	if err := validateLiveChildProviderExecutor(ChildExecutorModeFake, nil); err != nil {
		t.Fatalf("validateLiveChildProviderExecutor(fake,nil) error = %v", err)
	}

	smoke := SmokeLiveChildProvider()
	response, err := smoke.Infer(context.Background(), workerexecution.ProviderInferenceRequest{})
	if err != nil {
		t.Fatalf("SmokeLiveChildProvider().Infer: %v", err)
	}
	if response.ProviderSession == nil || response.ProviderSession.ID == "" {
		t.Fatalf("provider session = %#v, want stable session metadata", response.ProviderSession)
	}
}

func testExecutionServiceSourceRequestHelpers(t *testing.T) {
	t.Helper()

	inlineReq := startSourceRequest(Source{
		Kind: factory.WorkflowSourceKindInlineWorkflow,
		InlineWorkflow: &InlineWorkflowSource{
			InlineSource: "return 1;",
		},
	})
	if inlineReq.Value != "return 1;" || inlineReq.InlineSource != "return 1;" {
		t.Fatalf("startSourceRequest(inline) = %#v", inlineReq)
	}
	if resolutionOrderForLookupStage(factory.WorkflowSourceLookupStageProjectClaude) != "PROJECT_CLAUDE_WORKFLOWS" {
		t.Fatal("unexpected lookup stage mapping for project claude")
	}
	if resolutionOrderForLookupStage(factory.WorkflowSourceLookupStageNamedJavaScript) != "BUILTIN_GLOBAL_JAVASCRIPT_FACTORIES" {
		t.Fatal("unexpected lookup stage mapping for named javascript")
	}
	if resolutionOrderForLookupStage("unknown") != "" {
		t.Fatal("unexpected lookup stage mapping for unknown stage")
	}
}

func TestNormalizationAndIdempotencyHelpers(t *testing.T) {
	t.Parallel()
	t.Run("approve and source normalization", testNormalizationApproveAndSourceBranches)
	t.Run("canonical json and replay helpers", testNormalizationCanonicalAndReplayBranches)
	t.Run("idempotency hash stability", testNormalizationIdempotencyHashBranches)
}

func testNormalizationApproveAndSourceBranches(t *testing.T) {
	t.Helper()

	approved, err := NormalizeApproveRequest(ApproveRequest{
		ControlRequest:    ControlRequest{RequestID: "  ctrl-1  ", Reason: "  ok  "},
		ApprovalPreviewID: "  preview-1  ",
		ApprovedPolicy:    map[string]any{"policyHash": " hash-1 "},
	})
	if err != nil {
		t.Fatalf("NormalizeApproveRequest: %v", err)
	}
	if approved.RequestID != "ctrl-1" || approved.Reason != "ok" || approved.ApprovalPreviewID != "preview-1" {
		t.Fatalf("normalized approve request = %#v", approved)
	}

	inlineSource, err := normalizeSourceForIdempotency(Source{
		Kind: factory.WorkflowSourceKindInlineWorkflow,
		InlineWorkflow: &InlineWorkflowSource{
			InlineSource: " return 1; ",
			Dialect:      " you-workflow-v1 ",
			Entrypoint:   " default ",
			Metadata: map[string]string{
				"b": "2",
				"a": "1",
			},
		},
	})
	if err != nil {
		t.Fatalf("normalizeSourceForIdempotency(inline): %v", err)
	}
	inlineWorkflow, ok := inlineSource["inlineWorkflow"].(map[string]any)
	if !ok || inlineWorkflow["inlineSource"] != "return 1;" {
		t.Fatalf("inline workflow projection = %#v", inlineSource["inlineWorkflow"])
	}
	metadata, ok := inlineWorkflow["metadata"].(map[string]string)
	if !ok || len(metadata) != 2 || metadata["a"] != "1" || metadata["b"] != "2" {
		t.Fatalf("inline workflow metadata = %#v", inlineWorkflow["metadata"])
	}

	if _, err := normalizeSourceForIdempotency(Source{Kind: factory.WorkflowSourceKindInlineWorkflow}); err == nil {
		t.Fatal("normalizeSourceForIdempotency(missing inline) error = nil, want validation error")
	}
}

func testNormalizationCanonicalAndReplayBranches(t *testing.T) {
	t.Helper()

	if _, err := canonicalizeRawJSON(json.RawMessage("{")); err == nil {
		t.Fatal("canonicalizeRawJSON(invalid) error = nil, want parse error")
	}
	canonical, err := canonicalizeRawJSON(json.RawMessage(`{"b":2,"a":[{"d":4,"c":3}]}`))
	if err != nil {
		t.Fatalf("canonicalizeRawJSON(valid): %v", err)
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		t.Fatalf("json.Marshal(canonical): %v", err)
	}
	if string(encoded) != `{"a":[{"c":3,"d":4}],"b":2}` {
		t.Fatalf("canonical json = %s, want sorted object keys", encoded)
	}

	if err := CheckSyncStartReplayMode(&AsyncStartResult{}, nil, false); !errors.Is(err, ErrExecutionRequestIDConflict) {
		t.Fatalf("CheckSyncStartReplayMode(async replay mismatch) = %v, want ErrExecutionRequestIDConflict", err)
	}
	if err := CheckSyncStartReplayMode(nil, nil, false); err != nil {
		t.Fatalf("CheckSyncStartReplayMode(empty replay) = %v, want nil", err)
	}
	if err := CheckAsyncStartReplayMode(nil); !errors.Is(err, ErrExecutionRequestIDConflict) {
		t.Fatalf("CheckAsyncStartReplayMode(nil) = %v, want ErrExecutionRequestIDConflict", err)
	}
}

func testNormalizationIdempotencyHashBranches(t *testing.T) {
	t.Helper()

	hashA, err := IdempotencyTupleHash(inlineWorkflowStartRequest("req-hash-1", simpleFinalWorkflowSource, map[string]any{"x": 1}, map[string]any{"policyHash": "same"}))
	if err != nil {
		t.Fatalf("IdempotencyTupleHash(first): %v", err)
	}
	hashB, err := IdempotencyTupleHash(inlineWorkflowStartRequest("req-hash-1", simpleFinalWorkflowSource, map[string]any{"x": 1}, map[string]any{"policyHash": "same"}))
	if err != nil {
		t.Fatalf("IdempotencyTupleHash(second): %v", err)
	}
	if hashA != hashB {
		t.Fatalf("tuple hashes differ: %q vs %q", hashA, hashB)
	}
}

func TestPrepareStartAndPersistenceHelpers(t *testing.T) {
	t.Parallel()
	projectRoot := writeSimpleFinalWorkflowProject(t)
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: mustTestRuntimePersistenceStore(t, runtimepersist.DirForProjectRoot(projectRoot)),
	})

	prepared, err := service.prepareStart(StartRequest{
		RequestID: "req-prepare-start-001",
		Source: Source{
			Kind:         factory.WorkflowSourceKindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   1,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("prepareStart: %v", err)
	}
	if prepared.SourceRef == "" || prepared.SourceContent == "" || prepared.TupleHash == "" {
		t.Fatalf("prepared start = %#v, want resolved source fields", prepared)
	}

	terminal, err := service.executeImmediateSyncSession(context.Background(), prepared.Request, prepared.ResolvedSource, prepared.SourceContent, policyResolutionFromPrepared(prepared), "dur-sess-prepare-001")
	if err != nil {
		t.Fatalf("executeImmediateSyncSession: %v", err)
	}
	sessionID, err := NewDurableSessionID(testSessionIDGenerator)
	if err != nil {
		t.Fatalf("NewDurableSessionID: %v", err)
	}
	terminal.session.SessionID = sessionID
	terminal.result.SessionID = sessionID
	if terminal.session.Status != LifecycleStatusSucceeded {
		t.Fatalf("terminal session status = %q, want SUCCEEDED", terminal.session.Status)
	}

	if err := service.persistTerminalSessionState(terminal); err != nil {
		t.Fatalf("persistTerminalSessionState: %v", err)
	}
	loaded, err := service.snapshotSessionState(sessionID)
	if err != nil {
		t.Fatalf("snapshotSessionState(load persisted): %v", err)
	}
	if loaded.session.SessionID != sessionID {
		t.Fatalf("loaded sessionID = %q, want %q", loaded.session.SessionID, sessionID)
	}
	if loaded.result.ResultStatus != ResultStatusFinal {
		t.Fatalf("loaded result status = %q, want FINAL", loaded.result.ResultStatus)
	}
}

func TestValidateNamedAgentPresetsRejectsUnknownPresetBeforeStart(t *testing.T) {
	t.Parallel()
	err := validateNamedAgentPresets(
		map[string]interfaces.FactoryOrchestratorJavaScriptAgent{
			"reviewer": {Preset: "missing-preset"},
		},
		map[string]struct{}{"known-preset": {}},
	)
	if err == nil || !strings.Contains(err.Error(), `factory agent "reviewer" references unknown operator worker preset "missing-preset"`) {
		t.Fatalf("validateNamedAgentPresets() error = %v", err)
	}
}

func TestJavaScriptRuntimeService_ProjectRootAloneDoesNotEnablePersistence(t *testing.T) {
	t.Parallel()
	projectRoot := writeSimpleFinalWorkflowProject(t)
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: projectRoot})

	if _, err := service.StartSync(context.Background(), StartRequest{
		RequestID: "req-runtime-no-implicit-persistence-001",
		Source: Source{
			Kind:         factory.WorkflowSourceKindWorkflowName,
			WorkflowName: "simple-final",
		},
	}); err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	if _, err := os.Stat(runtimepersist.DirForProjectRoot(projectRoot)); !os.IsNotExist(err) {
		t.Fatalf("durable persistence path stat error = %v, want not exist", err)
	}
}

func newDefaultJavaScriptRuntimeService(t *testing.T, workflows ...factory.JavaScriptWorkflows) *JavaScriptRuntimeService {
	t.Helper()

	config := javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Clock:       durableFixedClock{now: time.Now()},
	}
	if len(workflows) > 0 {
		config.Workflows = workflows[0]
	}
	return newConfiguredJavaScriptRuntimeService(config)
}

func scriptedRuntimeWorkflows(
	run func(context.Context, factory.JavaScriptRuntimeRequest, factory.JavaScriptRuntimeHooks) (factory.JavaScriptRuntimeOutcome, error),
) factory.JavaScriptWorkflows {
	return factoryruntimefixtures.ScriptedJavaScriptWorkflows{RunFunc: run}
}

func scriptedSuccessfulRuntimeWorkflows(value map[string]any) factory.JavaScriptWorkflows {
	return scriptedRuntimeWorkflows(func(
		context.Context,
		factory.JavaScriptRuntimeRequest,
		factory.JavaScriptRuntimeHooks,
	) (factory.JavaScriptRuntimeOutcome, error) {
		encoded, err := json.Marshal(value)
		if err != nil {
			return factory.JavaScriptRuntimeOutcome{}, err
		}
		return factory.JavaScriptRuntimeOutcome{
			OK:    true,
			Value: factory.TypedValue{JSON: encoded},
		}, nil
	})
}

func scriptedBlockingRuntimeWorkflows() factory.JavaScriptWorkflows {
	return scriptedRuntimeWorkflows(func(
		ctx context.Context,
		_ factory.JavaScriptRuntimeRequest,
		_ factory.JavaScriptRuntimeHooks,
	) (factory.JavaScriptRuntimeOutcome, error) {
		<-ctx.Done()
		code := factory.JavaScriptRuntimeCodeCanceled
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code = factory.JavaScriptRuntimeCodeTimeout
		}
		return factory.JavaScriptRuntimeOutcome{
			Failure: factory.JavaScriptRuntimeFailure{Code: code, Message: ctx.Err().Error()},
		}, nil
	})
}

func scriptedFailedRuntimeWorkflows() factory.JavaScriptWorkflows {
	return scriptedRuntimeWorkflows(func(
		context.Context,
		factory.JavaScriptRuntimeRequest,
		factory.JavaScriptRuntimeHooks,
	) (factory.JavaScriptRuntimeOutcome, error) {
		return factory.JavaScriptRuntimeOutcome{
			Failure: factory.JavaScriptRuntimeFailure{
				Code:    factory.JavaScriptRuntimeCodeScriptError,
				Message: "scripted workflow execution failure",
			},
		}, nil
	})
}

func inlineWorkflowStartRequest(
	requestID string,
	source string,
	args map[string]any,
	requestedPolicy map[string]any,
) StartRequest {
	return StartRequest{
		RequestID: requestID,
		Source: Source{
			Kind: factory.WorkflowSourceKindInlineWorkflow,
			InlineWorkflow: &InlineWorkflowSource{
				InlineSource: source,
				Dialect:      "you-workflow-v1",
				Metadata: map[string]string{
					"name":        "runtime-async-fixture",
					"description": "returns a structured final value",
				},
			},
		},
		Args:            args,
		RequestedPolicy: requestedPolicy,
	}
}

func waitUntilSessionStatus(
	t *testing.T,
	service Service,
	sessionID string,
	want LifecycleStatus,
	timeout time.Duration,
) SessionReadResult {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session, err := service.GetSession(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if session.Status == want {
			return session
		}
		if IsTerminalLifecycleStatus(session.Status) && session.Status != want {
			t.Fatalf("session %s reached terminal %q before %q", sessionID, session.Status, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session %s did not reach status %q within %s", sessionID, want, timeout)
	return SessionReadResult{}
}

func decodePrimaryResultMap(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()

	var content []struct {
		Type string          `json:"type"`
		JSON json.RawMessage `json:"json,omitempty"`
	}
	if err := json.Unmarshal(raw, &content); err != nil {
		t.Fatalf("unmarshal primary result content: %v", err)
	}
	for _, part := range content {
		if part.Type == "JSON" && len(part.JSON) > 0 {
			var projected map[string]any
			if err := json.Unmarshal(part.JSON, &projected); err != nil {
				t.Fatalf("unmarshal primary result json part: %v", err)
			}
			return projected
		}
	}
	t.Fatalf("primary result content = %#v, want JSON part", content)
	return nil
}

func writeSimpleFinalWorkflowProject(t *testing.T) string {
	t.Helper()

	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, factory.WorkflowSourceProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	workflowPath := filepath.Join(workflowDir, "simple-final.workflow.js")
	if err := os.WriteFile(workflowPath, []byte(simpleFinalWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
}

func TestFakeService_DetailReadersAndRemainingControlWrappers(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)

	startAsyncByRequestID(t, service, "req-petri-success-001")
	dispatch, err := service.GetDispatch(context.Background(), "dur-sess-petri-success-001", "disp-petri-success-001")
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatch.DispatchSummary.ID != "disp-petri-success-001" {
		t.Fatalf("dispatch detail = %#v", dispatch)
	}

	artifact, err := service.GetArtifact(context.Background(), "dur-sess-petri-success-001", "art-petri-final-001")
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if artifact.ArtifactSummary.ID != "art-petri-final-001" {
		t.Fatalf("artifact detail = %#v", artifact)
	}

	startAsyncByRequestID(t, service, "req-js-run-n-001")
	cancelled, err := service.Cancel(context.Background(), "dur-sess-js-run-n-001", ControlRequest{})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Status != LifecycleStatusCanceling {
		t.Fatalf("cancel status = %q, want CANCELING", cancelled.Status)
	}

	terminated, err := service.Terminate(context.Background(), "dur-sess-js-run-n-001", ControlRequest{})
	if err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if terminated.Status != LifecycleStatusTerminated {
		t.Fatalf("terminate status = %q, want TERMINATED", terminated.Status)
	}

	startAsyncByRequestID(t, service, "req-js-awaiting-001")
	approved, err := service.Approve(context.Background(), "dur-sess-js-awaiting-001", ApproveRequest{})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if approved.Status != LifecycleStatusRunning {
		t.Fatalf("approve status = %q, want RUNNING", approved.Status)
	}
}

func TestJavaScriptRuntimeService_ControlWrappersAndDetailReaders(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	service.sessions["dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"] = newJavaScriptRuntimeRunningControlState(now)

	t.Run("detail readers and running controls", func(t *testing.T) {
		testJavaScriptRuntimeServiceRunningControlWrappers(t, service)
	})
	t.Run("approve awaiting session", func(t *testing.T) {
		testJavaScriptRuntimeServiceApproveAwaitingSession(t, service)
	})
	t.Run("retry failed dispatch", func(t *testing.T) {
		testJavaScriptRuntimeServiceRetryFailedDispatch(t, service)
	})
}

func newJavaScriptRuntimeRunningControlState(now time.Time) *runtimeSessionState {
	return &runtimeSessionState{
		session: SessionReadResult{
			SessionID:        "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Status:           LifecycleStatusRunning,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Lifecycle:        &LifecycleTimestamps{StartedAt: &now},
			ResolvedSource: ResolvedSource{
				SourceRef: "inline",
			},
			Links: InspectionLinksForSession("dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true),
		},
		result: ResultReadResult{
			SessionID:     "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SessionStatus: LifecycleStatusRunning,
			ResultStatus:  ResultStatusNotReady,
		},
		dispatches: []DispatchSummary{
			{ID: "disp-1", Status: DispatchStatusFailed, Attempt: 1},
		},
		dispatchStatusTransitions: map[string][]DispatchStatus{
			"disp-1": {DispatchStatusQueued, DispatchStatusFailed},
		},
		dispatchJavaScript: map[string]DispatchJavaScriptProjection{
			"disp-1": {TaskLabel: "child"},
		},
		artifacts: []ArtifactSummary{
			{ID: "art-1"},
		},
		events: BuildCanonicalRuntimeSessionEvents(
			SessionReadResult{
				SessionID:        "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Status:           LifecycleStatusRunning,
				OrchestratorKind: interfaces.OrchestratorKindJavaScript,
				Lifecycle:        &LifecycleTimestamps{StartedAt: &now},
			},
			ResultReadResult{
				SessionID:     "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				SessionStatus: LifecycleStatusRunning,
				ResultStatus:  ResultStatusNotReady,
				Availability: &ResultAvailabilityDetail{
					Reason:    "RESULT_NOT_READY",
					Message:   "Session is still running.",
					Retryable: true,
				},
			},
		),
	}
}

func testJavaScriptRuntimeServiceRunningControlWrappers(t *testing.T, service *JavaScriptRuntimeService) {
	t.Helper()

	if _, err := service.GetDispatch(context.Background(), "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "disp-1"); err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if _, err := service.ListArtifacts(context.Background(), "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if _, err := service.GetArtifact(context.Background(), "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "art-1"); err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	listed, err := service.ListSessions(context.Background(), ListSessionsRequest{Scope: SessionListScopeAll})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(listed.LiveSessions) != 1 {
		t.Fatalf("live sessions = %#v, want one session", listed.LiveSessions)
	}

	if _, err := service.Pause(context.Background(), "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ControlRequest{}); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if _, err := service.Resume(context.Background(), "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ControlRequest{}); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if _, err := service.Terminate(context.Background(), "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ControlRequest{}); err != nil {
		t.Fatalf("Terminate: %v", err)
	}
}

func testJavaScriptRuntimeServiceApproveAwaitingSession(t *testing.T, service *JavaScriptRuntimeService) {
	t.Helper()

	service.sessions["dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"] = &runtimeSessionState{
		session: SessionReadResult{
			SessionID:        "dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Status:           LifecycleStatusAwaitingApproval,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Lifecycle:        &LifecycleTimestamps{},
			Links:            InspectionLinksForSession("dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", true),
		},
		result: ResultReadResult{
			SessionID:     "dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			SessionStatus: LifecycleStatusAwaitingApproval,
			ResultStatus:  ResultStatusNotReady,
		},
	}
	if _, err := service.Approve(context.Background(), "dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ApproveRequest{}); err != nil {
		t.Fatalf("Approve: %v", err)
	}
}

func testJavaScriptRuntimeServiceRetryFailedDispatch(t *testing.T, service *JavaScriptRuntimeService) {
	t.Helper()

	service.sessions["dur-sess-cccccccccccccccccccccccccccccccc"] = &runtimeSessionState{
		session: SessionReadResult{
			SessionID:        "dur-sess-cccccccccccccccccccccccccccccccc",
			Status:           LifecycleStatusFailed,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Lifecycle:        &LifecycleTimestamps{},
			Links:            InspectionLinksForSession("dur-sess-cccccccccccccccccccccccccccccccc", true),
		},
		result: ResultReadResult{
			SessionID:     "dur-sess-cccccccccccccccccccccccccccccccc",
			SessionStatus: LifecycleStatusFailed,
			ResultStatus:  ResultStatusUnavailable,
		},
		dispatches: []DispatchSummary{
			{ID: "disp-retry", Status: DispatchStatusFailed, Attempt: 2},
		},
	}
	if _, err := service.RetryDispatch(context.Background(), "dur-sess-cccccccccccccccccccccccccccccccc", RetryDispatchRequest{DispatchID: "disp-retry"}); err != nil {
		t.Fatalf("RetryDispatch: %v", err)
	}
}

func TestNormalizeStartRequestAndErrorHelpers(t *testing.T) {
	t.Parallel()
	t.Run("normalize valid and invalid start requests", testNormalizeStartRequestBranches)
	t.Run("normalize source and child executor mode", testNormalizeSourceAndExecutorModeBranches)
	t.Run("error helper strings", testControlAndValidationErrorHelpers)
}

func testNormalizeStartRequestBranches(t *testing.T) {
	t.Helper()

	normalized, err := NormalizeStartRequest(StartRequest{
		RequestID: " req-1 ",
		Source: Source{
			Kind:          factory.WorkflowSourceKindFactoryInline,
			FactoryInline: json.RawMessage(`{"b":2,"a":1}`),
		},
		Orchestrator: &OrchestratorOverride{
			Kind: " custom ",
			Raw:  json.RawMessage(`{"z":2,"a":1}`),
		},
		Runtime: &RuntimeOptions{ChildExecutorMode: " live-provider "},
	})
	if err != nil {
		t.Fatalf("NormalizeStartRequest(factory inline): %v", err)
	}
	if normalized.RequestID != "req-1" {
		t.Fatalf("requestID = %q, want req-1", normalized.RequestID)
	}
	if string(normalized.Source.FactoryInline) == "" {
		t.Fatalf("factory inline unexpectedly empty: %#v", normalized.Source)
	}
	if normalized.Runtime == nil || normalized.Runtime.ChildExecutorMode != ChildExecutorModeLive {
		t.Fatalf("runtime = %#v, want live mode", normalized.Runtime)
	}
	if normalized.Orchestrator == nil || normalized.Orchestrator.Kind != "custom" {
		t.Fatalf("orchestrator = %#v, want trimmed kind", normalized.Orchestrator)
	}

	if _, err := NormalizeStartRequest(StartRequest{}); err == nil {
		t.Fatal("NormalizeStartRequest(missing requestID) error = nil, want validation error")
	}
	if _, err := NormalizeStartRequest(StartRequest{
		RequestID: "req-2",
		Source: Source{
			Kind:         factory.WorkflowSourceKindWorkflowFile,
			WorkflowFile: " path/to/workflow.js ",
		},
		Orchestrator: &OrchestratorOverride{
			Raw: json.RawMessage("{"),
		},
	}); err == nil {
		t.Fatal("NormalizeStartRequest(invalid orchestrator) error = nil, want validation error")
	}
}

func testNormalizeSourceAndExecutorModeBranches(t *testing.T) {
	t.Helper()

	if _, err := normalizeSource(Source{Kind: factory.WorkflowSourceKindWorkflowName, WorkflowName: "  "}); err == nil {
		t.Fatal("normalizeSource(empty workflow name) error = nil, want validation error")
	}
	if got := normalizeChildExecutorMode(" live-provider "); got != ChildExecutorModeLive {
		t.Fatalf("normalizeChildExecutorMode = %q, want live", got)
	}
	if got := resolveChildExecutorMode("fake", StartRequest{Runtime: &RuntimeOptions{ChildExecutorMode: "live-provider"}}); got != ChildExecutorModeLive {
		t.Fatalf("resolveChildExecutorMode = %q, want live override", got)
	}
}

func testControlAndValidationErrorHelpers(t *testing.T) {
	t.Helper()

	var controlErr *ControlError
	if controlErr.Error() != "" {
		t.Fatalf("nil control error message = %q, want empty", controlErr.Error())
	}
	controlErr = &ControlError{Outcome: LifecycleControlOutcomeConflict}
	if controlErr.Error() != string(LifecycleControlOutcomeConflict) {
		t.Fatalf("control error message = %q, want outcome text", controlErr.Error())
	}
	var validationErr *ValidationError
	if validationErr.Error() != "" {
		t.Fatalf("nil validation error message = %q, want empty", validationErr.Error())
	}
}

func TestRuntimeAndValidationHelperBranches(t *testing.T) {
	t.Parallel()
	t.Run("child executor hooks and marshal args", testRuntimeHookAndMarshalBranches)
	t.Run("workflow metadata and source validation errors", testRuntimeMetadataAndSourceValidationBranches)
	t.Run("policy validation errors", testRuntimePolicyValidationBranches)
}

func testRuntimeHookAndMarshalBranches(t *testing.T) {
	t.Helper()

	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	if hooks := service.childExecutorHooks(ChildExecutorModeFake, "session-fake"); hooks.NewChildExecutor != nil {
		t.Fatalf("fake hooks = %#v, want no child executor override", hooks)
	}
	liveService := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot:        t.TempDir(),
		InvocationExecutor: constructorInvocationExecutor{},
	})

	if hooks := liveService.childExecutorHooks(ChildExecutorModeLive, "session-live"); hooks.NewChildExecutor == nil {
		t.Fatal("expected live child executor hook")
	}

	if raw, err := marshalStartArgs(nil); err != nil || raw != nil {
		t.Fatalf("marshalStartArgs(nil) = %q, %v, want nil,nil", raw, err)
	}
	if _, err := marshalStartArgs(map[string]any{"bad": func() {}}); err == nil {
		t.Fatal("marshalStartArgs(non-json) error = nil, want validation error")
	}
}

func testRuntimeMetadataAndSourceValidationBranches(t *testing.T) {
	t.Helper()

	metadata := workflowMetadataFromResolved(ResolvedSource{
		SourceRef: "resolved-ref",
		Metadata: map[string]string{
			"project": "root",
		},
	}, StartRequest{
		Source: Source{
			WorkflowName: "named-workflow",
			InlineWorkflow: &InlineWorkflowSource{
				Metadata: map[string]string{"team": "ops"},
			},
		},
	})
	if metadata["name"] != "named-workflow" || metadata["team"] != "ops" || metadata["project"] != "root" {
		t.Fatalf("workflow metadata = %#v", metadata)
	}

	if err := validationErrorFromSourceIssues(nil); err == nil || err.Error() == "" {
		t.Fatalf("validationErrorFromSourceIssues(nil) = %v, want default validation error", err)
	}
	if err := validationErrorFromSourceIssues([]factory.WorkflowValidationIssue{{Message: "bad source", Line: 3, Column: 5}}); err == nil || err.Error() != "bad source (line 3, column 5)" {
		t.Fatalf("validationErrorFromSourceIssues(location) = %v", err)
	}
	if err := validationErrorFromSourceIssues([]factory.WorkflowValidationIssue{{}}); err == nil || err.Error() != "workflow source validation failed" {
		t.Fatalf("validationErrorFromSourceIssues(default message) = %v", err)
	}
}

func testRuntimePolicyValidationBranches(t *testing.T) {
	t.Helper()

	if err := validationErrorFromPolicyIssues(nil); err != nil {
		t.Fatalf("validationErrorFromPolicyIssues(nil) = %v, want nil", err)
	}
	if err := validationErrorFromPolicyIssues([]factory.JavaScriptPolicyIssue{{Message: "blocked"}}); err == nil || err.Error() != "blocked" {
		t.Fatalf("validationErrorFromPolicyIssues = %v, want blocked", err)
	}
	if err := validationErrorFromPolicyIssues([]factory.JavaScriptPolicyIssue{{}}); err == nil || err.Error() != "requested policy is invalid" {
		t.Fatalf("validationErrorFromPolicyIssues(default message) = %v", err)
	}
}

func TestStartSourceRequestAndResolutionOrderBranches(t *testing.T) {
	t.Parallel()
	cases := []struct {
		source Source
		want   string
	}{
		{Source{Kind: factory.WorkflowSourceKindFactoryID, FactoryID: "factory-1"}, "factory-1"},
		{Source{Kind: factory.WorkflowSourceKindFactoryInline, FactoryInline: json.RawMessage(`{"name":"factory"}`)}, `{"name":"factory"}`},
		{Source{Kind: factory.WorkflowSourceKindWorkflowFile, WorkflowFile: "wf.js"}, "wf.js"},
		{Source{Kind: factory.WorkflowSourceKindWorkflowName, WorkflowName: "name"}, "name"},
	}
	for _, tc := range cases {
		if got := startSourceRequest(tc.source); got.Value != tc.want {
			t.Fatalf("startSourceRequest(%s) value = %q, want %q", tc.source.Kind, got.Value, tc.want)
		}
	}
	if got := startSourceRequest(Source{Kind: factory.WorkflowSourceKindInlineWorkflow}); got.Value != "" || got.InlineSource != "" {
		t.Fatalf("startSourceRequest(missing inline) = %#v, want empty inline request", got)
	}

	stages := []factory.WorkflowSourceLookupStage{
		factory.WorkflowSourceLookupStageProjectClaude,
		factory.WorkflowSourceLookupStageExplicitSourceKind,
		factory.WorkflowSourceLookupStageGlobalUser,
		factory.WorkflowSourceLookupStagePackageRelative,
		factory.WorkflowSourceLookupStageNamedJavaScript,
		factory.WorkflowSourceLookupStageExplicitFactory,
	}
	for _, stage := range stages {
		if resolutionOrderForLookupStage(stage) == "" {
			t.Fatalf("resolutionOrderForLookupStage(%q) returned empty mapping", stage)
		}
	}
}

func TestJavaScriptRuntimeService_ReplayAndReadErrorBranches(t *testing.T) {
	t.Parallel()
	service := newDefaultJavaScriptRuntimeService(t)
	req := inlineWorkflowStartRequest(
		"req-runtime-replay-001",
		simpleFinalWorkflowSource,
		map[string]any{"subject": "workflows", "count": 1, "prefix": "you"},
		nil,
	)

	first, err := service.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("StartAsync(first): %v", err)
	}
	second, err := service.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("StartAsync(replay): %v", err)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("replay sessionID = %q, want %q", second.SessionID, first.SessionID)
	}
	waitUntilSessionStatus(t, service, first.SessionID, LifecycleStatusSucceeded, 5*time.Second)

	syncReq := inlineWorkflowStartRequest(
		"req-runtime-replay-sync-001",
		simpleFinalWorkflowSource,
		map[string]any{"subject": "workflows", "count": 1, "prefix": "you"},
		nil,
	)
	syncFirst, err := service.StartSync(context.Background(), syncReq)
	if err != nil {
		t.Fatalf("StartSync(first): %v", err)
	}
	syncSecond, err := service.StartSync(context.Background(), syncReq)
	if err != nil {
		t.Fatalf("StartSync(replay): %v", err)
	}
	if syncSecond.SessionID != syncFirst.SessionID {
		t.Fatalf("sync replay sessionID = %q, want %q", syncSecond.SessionID, syncFirst.SessionID)
	}

	if _, err := service.GetSession(context.Background(), ""); err == nil {
		t.Fatal("GetSession(empty) error = nil, want validation error")
	}
	if _, err := service.GetSession(context.Background(), "dur-sess-dddddddddddddddddddddddddddddddd"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("GetSession(missing) = %v, want ErrSessionNotFound", err)
	}
	if _, err := service.GetDispatch(context.Background(), syncFirst.SessionID, "missing-dispatch"); !errors.Is(err, ErrDispatchNotFound) {
		t.Fatalf("GetDispatch(missing) = %v, want ErrDispatchNotFound", err)
	}
	if _, err := service.GetArtifact(context.Background(), syncFirst.SessionID, "missing-artifact"); !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("GetArtifact(missing) = %v, want ErrArtifactNotFound", err)
	}
	if _, err := service.ReadEvents(context.Background(), syncFirst.SessionID, EventReconnectRequest{AfterEventID: "missing"}); !errors.Is(err, ErrReconnectCursorNotFound) {
		t.Fatalf("ReadEvents(missing cursor) = %v, want ErrReconnectCursorNotFound", err)
	}
}

func TestPersistAndMetadataNoOpBranches(t *testing.T) {
	t.Parallel()
	if err := (&JavaScriptRuntimeService{}).persistTerminalSessionState(runtimeSessionState{}); err != nil {
		t.Fatalf("persistTerminalSessionState(no dir) = %v, want nil", err)
	}

	projectRoot := t.TempDir()
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: mustTestRuntimePersistenceStore(t, runtimepersist.DirForProjectRoot(projectRoot)),
	})

	if err := service.persistTerminalSessionState(runtimeSessionState{
		session: SessionReadResult{SessionID: "dur-sess-eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Status: LifecycleStatusRunning},
	}); err != nil {
		t.Fatalf("persistTerminalSessionState(non-terminal) = %v, want nil", err)
	}
	if err := service.persistTerminalSessionState(runtimeSessionState{
		session: SessionReadResult{Status: LifecycleStatusSucceeded},
	}); err != nil {
		t.Fatalf("persistTerminalSessionState(empty session id) = %v, want nil", err)
	}

	metadata := workflowMetadataFromResolved(ResolvedSource{SourceRef: "fallback-ref"}, StartRequest{})
	if metadata["name"] != "fallback-ref" {
		t.Fatalf("fallback metadata name = %#v, want fallback-ref", metadata["name"])
	}
}

func TestNormalizeStartRequestAdditionalSourceBranches(t *testing.T) {
	t.Parallel()
	cases := []StartRequest{
		{
			RequestID: "req-file",
			Source: Source{
				Kind:         factory.WorkflowSourceKindWorkflowFile,
				WorkflowFile: " workflow.js ",
			},
		},
		{
			RequestID: "req-name",
			Source: Source{
				Kind:         factory.WorkflowSourceKindWorkflowName,
				WorkflowName: " named-workflow ",
			},
		},
		{
			RequestID: "req-inline",
			Source: Source{
				Kind: factory.WorkflowSourceKindInlineWorkflow,
				InlineWorkflow: &InlineWorkflowSource{
					InlineSource: " return 1; ",
					Dialect:      " you-workflow-v1 ",
					Entrypoint:   " default ",
					Metadata:     map[string]string{"k": "v"},
				},
			},
		},
	}
	for _, req := range cases {
		normalized, err := NormalizeStartRequest(req)
		if err != nil {
			t.Fatalf("NormalizeStartRequest(%s): %v", req.RequestID, err)
		}
		if normalized.Source.Kind != req.Source.Kind {
			t.Fatalf("normalized source kind = %q, want %q", normalized.Source.Kind, req.Source.Kind)
		}
	}
	if _, err := normalizeSource(Source{}); err == nil {
		t.Fatal("normalizeSource(unknown kind) error = nil, want validation error")
	}
}

func TestListingFiltersAndNormalizationBranches(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	later := now.Add(2 * time.Hour)
	summary := DurableSessionListSummary{
		SessionID:        "dur-sess-filter-1",
		Status:           LifecycleStatusRunning,
		OrchestratorKind: "JAVASCRIPT",
		ResolvedSource: ResolvedSource{
			Kind:      factory.WorkflowSourceKindWorkflowName,
			SourceRef: "customer/support",
			Metadata:  map[string]string{"project": "/workspace/customer"},
		},
		Recoverable: true,
		StaleLease:  true,
		Lifecycle: &LifecycleTimestamps{
			QueuedAt:   &now,
			StartedAt:  &later,
			UpdatedAt:  &later,
			FinishedAt: &later,
		},
	}
	yes := true
	after := now.Add(-time.Minute)
	before := later.Add(time.Minute)
	if !MatchesDurableSessionListFilters(summary, SessionListFilters{
		Statuses:          []LifecycleStatus{LifecycleStatusRunning},
		OrchestratorKinds: []string{" javascript "},
		SourceKind:        factory.WorkflowSourceKindWorkflowName,
		SourceRef:         "support",
		ProjectBoundary:   "workspace",
		Recoverable:       &yes,
		StaleLease:        &yes,
		CreatedAfter:      &after,
		CreatedBefore:     &before,
		UpdatedAfter:      &after,
		UpdatedBefore:     &before,
	}) {
		t.Fatal("expected summary to match all listing filters")
	}
	no := false
	if MatchesDurableSessionListFilters(summary, SessionListFilters{Recoverable: &no}) {
		t.Fatal("recoverable mismatch unexpectedly matched")
	}
	if containsLifecycleStatus([]LifecycleStatus{LifecycleStatusPaused}, LifecycleStatusRunning) {
		t.Fatal("containsLifecycleStatus mismatch unexpectedly matched")
	}
	if containsString([]string{"Alpha"}, "beta") {
		t.Fatal("containsString mismatch unexpectedly matched")
	}
	if firstLifecycleTimestamp(nil, &later) != &later {
		t.Fatal("firstLifecycleTimestamp did not return first non-nil value")
	}
	if latestLifecycleTimestamp(summary.Lifecycle) != &later {
		t.Fatal("latestLifecycleTimestamp did not return latest time")
	}

	normalized, err := NormalizeListSessionsRequest(ListSessionsRequest{
		Scope: SessionListScopeAll,
		Filters: SessionListFilters{
			Statuses:          []LifecycleStatus{LifecycleStatusRunning},
			OrchestratorKinds: []string{" JAVASCRIPT ", ""},
			SourceKind:        factory.WorkflowSourceKindWorkflowName,
			CreatedAfter:      &after,
			CreatedBefore:     &before,
		},
	})
	if err != nil {
		t.Fatalf("NormalizeListSessionsRequest: %v", err)
	}
	if normalized.Scope != SessionListScopeAll || len(normalized.Filters.OrchestratorKinds) != 1 {
		t.Fatalf("normalized list request = %#v", normalized)
	}
	if _, err := NormalizeListSessionsRequest(ListSessionsRequest{Scope: SessionListScope("bad")}); err == nil {
		t.Fatal("NormalizeListSessionsRequest(bad scope) error = nil, want validation error")
	}
	if _, err := NormalizeListSessionsRequest(ListSessionsRequest{
		Filters: SessionListFilters{
			SourceKind:    factory.WorkflowSourceKind("unknown"),
			CreatedAfter:  &before,
			CreatedBefore: &after,
		},
	}); err == nil {
		t.Fatal("NormalizeListSessionsRequest(invalid filters) error = nil, want validation error")
	}
}

func TestSmallHelperBranches(t *testing.T) {
	t.Parallel()
	if got := resolvedDialect(ResolvedSource{Dialect: "custom"}); got != "custom" {
		t.Fatalf("resolvedDialect(custom) = %q, want custom", got)
	}
	if got := resolvedDialect(ResolvedSource{}); got != "you-workflow-v1" {
		t.Fatalf("resolvedDialect(default) = %q, want you-workflow-v1", got)
	}
	if id, err := NormalizeSessionID(" session-1 "); err != nil || id != "session-1" {
		t.Fatalf("NormalizeSessionID = %q, %v, want session-1,nil", id, err)
	}
	if _, err := NormalizeSessionID("   "); err == nil {
		t.Fatal("NormalizeSessionID(empty) error = nil, want validation error")
	}
}

func TestProjectionCloneHelpers(t *testing.T) {
	t.Parallel()
	observedAt := time.Now().UTC()
	artifact := artifactSummaryFromRuntimeRecord("dur-sess-helper-1", factory.JavaScriptArtifactRecord{
		ID:         "art-helper-1",
		Kind:       "RESULT",
		Visibility: "PUBLIC",
		Label:      "helper",
	}, observedAt)
	if artifact.ID != "art-helper-1" || artifact.RetrievalRef == nil || artifact.RetrievalRef.Href == "" {
		t.Fatalf("artifact summary = %#v", artifact)
	}

	js := cloneDispatchJavaScriptProjections(map[string]DispatchJavaScriptProjection{
		"disp-1": {TaskLabel: "child"},
	})
	if js["disp-1"].TaskLabel != "child" {
		t.Fatalf("cloned javascript projections = %#v", js)
	}
	transitions := cloneDispatchStatusTransitions(map[string][]DispatchStatus{
		"disp-1": {DispatchStatusQueued, DispatchStatusRunning},
	})
	if len(transitions["disp-1"]) != 2 {
		t.Fatalf("cloned transitions = %#v", transitions)
	}
}

func testJavaScriptRuntimeSyncCompletedSession(t *testing.T, service *JavaScriptRuntimeService, sessionID string) {
	t.Helper()

	session, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Status != LifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", session.Status)
	}
	if session.ResultSummary == nil || session.ResultSummary.ResultStatus != string(ResultStatusFinal) {
		t.Fatalf("resultSummary = %#v, want FINAL", session.ResultSummary)
	}
}

func testJavaScriptRuntimeSyncCompletedResult(t *testing.T, service *JavaScriptRuntimeService, sessionID string) {
	t.Helper()

	result, err := service.GetResult(context.Background(), sessionID, ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != ResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", result.ResultStatus)
	}
	projected := decodePrimaryResultMap(t, result.PrimaryResult)
	if projected["echo"] != "you:workflows" {
		t.Fatalf("primaryResult echo = %#v, want you:workflows", projected["echo"])
	}
}

func testJavaScriptRuntimeSyncCompletedEvents(t *testing.T, service *JavaScriptRuntimeService, sessionID string) {
	t.Helper()

	events, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events.Events) != 3 {
		t.Fatalf("events = %d, want 3 canonical lifecycle events", len(events.Events))
	}
}
func TestProjectedLifecycleControlStatus_PrefersCanonicalBracketStatus(t *testing.T) {
	t.Parallel()
	status := ProjectedLifecycleControlStatus("PAUSED", "RUNNING")
	if status != LifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", status)
	}
}

func TestProjectedLifecycleControlStatus_FallsBackToFactoryRuntimeState(t *testing.T) {
	t.Parallel()
	if got := ProjectedLifecycleControlStatus("", "PAUSED"); got != LifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", got)
	}
	if got := ProjectedLifecycleControlStatus("", "RUNNING"); got != LifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", got)
	}
}

func TestFactoryStateToLifecycleStatus_MapsLiveFactoryStates(t *testing.T) {
	t.Parallel()

	for state, want := range map[string]LifecycleStatus{
		"IDLE": LifecycleStatusRunning, "RUNNING": LifecycleStatusRunning,
		"PAUSED": LifecycleStatusPaused, "COMPLETED": LifecycleStatusSucceeded,
		"FAILED": LifecycleStatusFailed,
	} {
		if got := LifecycleStatusFromFactoryRuntimeState(state); got != want {
			t.Fatalf("state %q = %q, want %q", state, got, want)
		}
	}
}

func TestLiveLifecycleControlResponse_BuildsTypedPauseOutcome(t *testing.T) {
	t.Parallel()

	result := LifecycleControlResult{
		SessionID: "~default", Operation: LifecycleControlPause,
		Outcome: LifecycleControlOutcomeAccepted, Status: LifecycleStatusPaused,
		Links: LiveLifecycleControlLinksForSession("~default"),
	}
	if result.SessionID != "~default" || result.Operation != LifecycleControlPause ||
		result.Outcome != LifecycleControlOutcomeAccepted || result.Status != LifecycleStatusPaused {
		t.Fatalf("result = %#v, want accepted live pause outcome", result)
	}
	if result.Links.Session != "/factory-sessions/~default" {
		t.Fatalf("links = %#v, want /factory-sessions/~default", result.Links)
	}
}
func TestJavaScriptRuntimeService_InterruptAcceptedBeforeChildCompletion_RecordsObservedCancellation(t *testing.T) {
	t.Parallel()
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	sessionID := "dur-sess-interrupt-race-runtime-001"
	dispatchID := "dispatch-1"
	if err := seedRuntimeSessionWithRunningDispatch(service, sessionID, dispatchID, "summarize-findings"); err != nil {
		t.Fatalf("SeedRuntimeSessionWithRunningDispatch: %v", err)
	}

	interruptResult, err := service.InterruptDispatch(context.Background(), sessionID, InterruptDispatchRequest{
		ControlRequest: ControlRequest{Reason: "operator stop before provider completion"},
		DispatchID:     dispatchID,
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}
	if interruptResult.Outcome != LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", interruptResult.Outcome)
	}
	if interruptResult.DispatchID != dispatchID {
		t.Fatalf("dispatchId = %q, want %q", interruptResult.DispatchID, dispatchID)
	}

	dispatch, err := service.GetDispatch(context.Background(), sessionID, dispatchID)
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatch.Status != DispatchStatusInterrupted {
		t.Fatalf("dispatch status = %q, want INTERRUPTED", dispatch.Status)
	}
	if dispatch.FailureDetail == nil || dispatch.FailureDetail.Message != "operator stop before provider completion" {
		t.Fatalf("failureDetail = %#v, want operator stop before provider completion", dispatch.FailureDetail)
	}

	events, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	payload := findDispatchInterruptedEventPayload(t, events.Events, dispatchID)
	if payload.Reason != "operator stop before provider completion" {
		t.Fatalf("event reason = %q, want operator stop before provider completion", payload.Reason)
	}
	if payload.ObservedStatus != string(factoryapi.FactoryDispatchStatusRUNNING) {
		t.Fatalf("observedStatus = %q, want RUNNING", payload.ObservedStatus)
	}

	replayed, err := ReplayDispatchProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplayDispatchProjection: %v", err)
	}
	if len(replayed) != 1 || replayed[0].Status != DispatchStatusInterrupted {
		t.Fatalf("replayed dispatches = %#v, want one interrupted dispatch", replayed)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this runtime regression keeps late-result suppression assertions together on one scenario.
func TestJavaScriptRuntimeService_LateChildResultAfterInterrupt_SuppressesNormalRouting(t *testing.T) {
	t.Parallel()
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	sessionID := "dur-sess-interrupt-late-runtime-001"
	dispatchID := "dispatch-1"
	if err := seedRuntimeSessionWithRunningDispatch(service, sessionID, dispatchID, "summarize-findings"); err != nil {
		t.Fatalf("SeedRuntimeSessionWithRunningDispatch: %v", err)
	}

	_, err := service.InterruptDispatch(context.Background(), sessionID, InterruptDispatchRequest{
		ControlRequest: ControlRequest{Reason: "operator stop"},
		DispatchID:     dispatchID,
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}

	lateRecords := []factory.JavaScriptRuntimeRecord{{
		Kind: factory.JavaScriptRecordKindChildDispatch,
		ChildDispatch: &factory.JavaScriptChildDispatchRecord{
			DispatchID:         dispatchID,
			Status:             factory.JavaScriptChildDispatchStatusCompleted,
			Label:              "summarize-findings",
			ArtifactRef:        factory.FormatArtifactURI(sessionID, "child-artifact-late"),
			ProviderSessionRef: "provider-session-late",
			Provider:           "mock",
		},
	}}
	if err := applyRuntimeTerminalOutcome(service, sessionID, factory.JavaScriptRuntimeOutcome{
		OK:      true,
		Records: lateRecords,
		Value:   factory.TypedValue{JSON: json.RawMessage(`{"label":"agent-run-fake-child"}`)},
	}); err != nil {
		t.Fatalf("ApplyRuntimeTerminalOutcomeForTests: %v", err)
	}

	dispatch, err := service.GetDispatch(context.Background(), sessionID, dispatchID)
	if err != nil {
		t.Fatalf("GetDispatch after late completion: %v", err)
	}
	if dispatch.Status != DispatchStatusInterrupted {
		t.Fatalf("dispatch status = %q, want INTERRUPTED after late completion", dispatch.Status)
	}
	if len(dispatch.OutputArtifactIDs) != 0 {
		t.Fatalf("outputArtifactIds = %#v, want suppressed late child output", dispatch.OutputArtifactIDs)
	}
	if len(dispatch.ProviderSessionRefs) != 1 || dispatch.ProviderSessionRefs[0].ID != "provider-session-late" {
		t.Fatalf("providerSessionRefs = %#v, want late diagnostic preserved", dispatch.ProviderSessionRefs)
	}

	session, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession after late completion: %v", err)
	}
	if session.Status != LifecycleStatusInterrupted {
		t.Fatalf("session status = %q, want INTERRUPTED after late completion", session.Status)
	}
	if session.Progress != nil && session.Progress.CompletedDispatches != 0 {
		t.Fatalf("completedDispatches = %d, want 0 after suppression", session.Progress.CompletedDispatches)
	}
	if session.ResultSummary == nil || session.ResultSummary.ResultStatus != string(ResultStatusUnavailable) {
		t.Fatalf("resultSummary = %#v, want UNAVAILABLE after late completion suppression", session.ResultSummary)
	}

	result, err := service.GetResult(context.Background(), sessionID, ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult after late completion: %v", err)
	}
	if result.SessionStatus != LifecycleStatusInterrupted || result.ResultStatus != ResultStatusUnavailable {
		t.Fatalf("result = status %q session %q, want UNAVAILABLE/INTERRUPTED", result.ResultStatus, result.SessionStatus)
	}
	if result.Availability == nil || result.Availability.Reason != "SESSION_INTERRUPTED" {
		t.Fatalf("availability = %#v, want SESSION_INTERRUPTED", result.Availability)
	}

	artifacts, err := service.ListArtifacts(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	for _, artifact := range artifacts.Artifacts {
		if artifact.DispatchID == dispatchID && artifact.Kind == "CHILD_RESULT" {
			t.Fatalf("artifact = %#v, want late child output suppressed", artifact)
		}
	}

	events, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if !containsEventType(events.Events, "DISPATCH_INTERRUPTED") {
		t.Fatal("DISPATCH_INTERRUPTED event missing after late completion merge")
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this fake-service race regression keeps interrupt and replay assertions together on one scenario.
func TestFakeService_InterruptAcceptedBeforeCompletion_ObservableDispatchAndEventOutcomes(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	started := startAsyncByRequestID(t, service, "req-js-run-n-001")

	dispatches, err := service.ListDispatches(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches before interrupt: %v", err)
	}
	if len(dispatches.Dispatches) < 2 {
		t.Fatalf("dispatches = %#v, want at least two fixture dispatches", dispatches.Dispatches)
	}
	runningBefore := findDispatchByID(dispatches.Dispatches, "disp-js-002")
	if runningBefore == nil || runningBefore.Status != DispatchStatusRunning {
		t.Fatalf("dispatch disp-js-002 = %#v, want RUNNING before interrupt", runningBefore)
	}

	interruptResult, err := service.InterruptDispatch(context.Background(), started.SessionID, InterruptDispatchRequest{
		ControlRequest: ControlRequest{Reason: "stop before provider completion"},
		DispatchID:     "disp-js-002",
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}
	if interruptResult.Outcome != LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", interruptResult.Outcome)
	}

	dispatch, err := service.GetDispatch(context.Background(), started.SessionID, "disp-js-002")
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatch.Status != DispatchStatusInterrupted {
		t.Fatalf("dispatch status = %q, want INTERRUPTED", dispatch.Status)
	}

	events, err := service.ReadEvents(context.Background(), started.SessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	payload := findDispatchInterruptedEventPayload(t, events.Events, "disp-js-002")
	if payload.ObservedStatus != string(factoryapi.FactoryDispatchStatusRUNNING) {
		t.Fatalf("observedStatus = %q, want RUNNING", payload.ObservedStatus)
	}
	if payload.Reason != "stop before provider completion" {
		t.Fatalf("reason = %q, want stop before provider completion", payload.Reason)
	}

	session, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	updated, err := service.ListDispatches(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches after interrupt: %v", err)
	}
	interrupted := findDispatchByID(updated.Dispatches, "disp-js-002")
	if interrupted == nil || interrupted.Status != DispatchStatusInterrupted {
		t.Fatalf("dispatch disp-js-002 after interrupt = %#v, want INTERRUPTED", interrupted)
	}
	if err := ValidateDispatchListMatchesSessionProgress(session, updated.Dispatches); err != nil {
		t.Fatalf("ValidateDispatchListMatchesSessionProgress: %v", err)
	}
}

func findDispatchInterruptedEventPayload(t *testing.T, events []json.RawMessage, dispatchID string) dispatchInterruptedEventPayload {
	t.Helper()
	for _, raw := range events {
		var envelope canonicalFactoryEvent
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if envelope.Type != "DISPATCH_INTERRUPTED" {
			continue
		}
		if envelope.Context.DispatchID == nil || *envelope.Context.DispatchID != dispatchID {
			continue
		}
		var payload dispatchInterruptedEventPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			t.Fatalf("unmarshal DISPATCH_INTERRUPTED payload: %v", err)
		}
		return payload
	}
	t.Fatalf("DISPATCH_INTERRUPTED event for %s not found in %#v", dispatchID, events)
	return dispatchInterruptedEventPayload{}
}

func containsEventType(events []json.RawMessage, eventType string) bool {
	for _, raw := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if envelope.Type == eventType {
			return true
		}
	}
	return false
}

func findDispatchByID(dispatches []DispatchSummary, dispatchID string) *DispatchSummary {
	for index := range dispatches {
		if dispatches[index].ID == dispatchID {
			return &dispatches[index]
		}
	}
	return nil
}

func TestJavaScriptRuntimeService_InterruptRunningDispatch_PreservesObservedCancellationAtRecordTime(t *testing.T) {
	t.Parallel()
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	sessionID := "dur-sess-interrupt-observed-status-001"
	now := time.Date(2026, 6, 20, 18, 0, 0, 0, time.UTC)
	service.sessions[sessionID] = &runtimeSessionState{
		session: SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusRunning,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Phase:            "execute",
			Lifecycle:        &LifecycleTimestamps{StartedAt: &now},
			Links:            InspectionLinksForSession(sessionID, true),
		},
		result: ResultReadResult{
			SessionID:     sessionID,
			SessionStatus: LifecycleStatusRunning,
			ResultStatus:  ResultStatusNotReady,
		},
		dispatches: []DispatchSummary{{
			ID:     "dispatch-1",
			Status: DispatchStatusRunning,
			Phase:  "execute",
			Label:  "summarize-findings",
		}},
		dispatchStatusTransitions: map[string][]DispatchStatus{
			"dispatch-1": {DispatchStatusQueued, DispatchStatusRunning},
		},
		events: BuildCanonicalRuntimeSessionEvents(
			SessionReadResult{SessionID: sessionID, Status: LifecycleStatusRunning, OrchestratorKind: interfaces.OrchestratorKindJavaScript},
			ResultReadResult{SessionID: sessionID, SessionStatus: LifecycleStatusRunning, ResultStatus: ResultStatusNotReady},
		),
	}

	_, err := service.InterruptDispatch(context.Background(), sessionID, InterruptDispatchRequest{
		ControlRequest: ControlRequest{Reason: "cancellation observed while running"},
		DispatchID:     "dispatch-1",
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}

	events, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	payload := findDispatchInterruptedEventPayload(t, events.Events, "dispatch-1")
	if payload.ObservedStatus != string(factoryapi.FactoryDispatchStatusRUNNING) {
		t.Fatalf("observedStatus = %q, want RUNNING", payload.ObservedStatus)
	}
	if payload.Reason != "cancellation observed while running" {
		t.Fatalf("reason = %q, want cancellation observed while running", payload.Reason)
	}
}

func TestValidateCheckpointSummaryForResume_RejectsInvalidMetadata(t *testing.T) {
	t.Parallel()
	sessionID := "dur-sess-checkpoint-validation-001"
	valid := &factory.JavaScriptCheckpointSummary{
		Kind:           factory.JavaScriptCheckpointSummaryKind,
		SchemaVersion:  factory.JavaScriptCheckpointSummarySchemaVersion,
		CheckpointID:   "checkpoint-1",
		SessionID:      sessionID,
		ResumeStrategy: factory.JavaScriptResumeStrategy,
	}
	if err := validateCheckpointSummaryForResume(valid, sessionID); err != nil {
		t.Fatalf("validateCheckpointSummaryForResume(valid): %v", err)
	}

	cases := []struct {
		name    string
		summary *factory.JavaScriptCheckpointSummary
		field   string
	}{
		{
			name:    "missing checkpoint",
			summary: nil,
			field:   "checkpointSummary",
		},
		{
			name: "invalid kind",
			summary: &factory.JavaScriptCheckpointSummary{
				Kind:         "invalid-kind",
				CheckpointID: "checkpoint-1",
			},
			field: "checkpointSummary.kind",
		},
		{
			name: "unsupported schema version",
			summary: &factory.JavaScriptCheckpointSummary{
				SchemaVersion: 99,
				CheckpointID:  "checkpoint-1",
			},
			field: "checkpointSummary.schemaVersion",
		},
		{
			name: "missing checkpoint id",
			summary: &factory.JavaScriptCheckpointSummary{
				Kind: factory.JavaScriptCheckpointSummaryKind,
			},
			field: "checkpointSummary.checkpointId",
		},
		{
			name: "session id mismatch",
			summary: &factory.JavaScriptCheckpointSummary{
				CheckpointID:   "checkpoint-1",
				SessionID:      "dur-sess-other",
				ResumeStrategy: factory.JavaScriptResumeStrategy,
			},
			field: "checkpointSummary.sessionId",
		},
		{
			name: "checkpoint not approved for resume",
			summary: &factory.JavaScriptCheckpointSummary{
				CheckpointID: "checkpoint-1",
			},
			field: "checkpointSummary.resumeStrategy",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCheckpointSummaryForResume(tc.summary, sessionID)
			resumeErr, ok := err.(*ResumeError)
			if !ok {
				t.Fatalf("error = %T (%v), want *ResumeError", err, err)
			}
			if resumeErr.Outcome != ResumeOutcomeInvalidState && resumeErr.Outcome != ResumeOutcomeMissingCheckpoint {
				t.Fatalf("outcome = %q, want typed resume failure", resumeErr.Outcome)
			}
			if tc.field != "" && resumeErr.Field != tc.field {
				t.Fatalf("field = %q, want %q", resumeErr.Field, tc.field)
			}
		})
	}
}

func TestFinalizeInterruptedTerminalSession_PreservesPartialAndUnavailableResults(t *testing.T) {
	t.Parallel()
	sessionID := "dur-sess-interrupted-finalize-001"
	interruptedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	t.Run("partial result", func(t *testing.T) {
		state := &runtimeSessionState{
			session: SessionReadResult{
				SessionID: sessionID,
				Status:    LifecycleStatusRunning,
				Lifecycle: &LifecycleTimestamps{InterruptedAt: &interruptedAt},
			},
		}
		priorSession := SessionReadResult{
			SessionID:     sessionID,
			ResultSummary: &ResultSummary{ResultStatus: string(ResultStatusPartial), Summary: "partial output"},
		}
		priorResult := ResultReadResult{
			SessionID:     sessionID,
			ResultStatus:  ResultStatusPartial,
			PrimaryResult: json.RawMessage(`{"text":"partial output"}`),
		}
		finalizeInterruptedTerminalSession(state, priorSession, priorResult)
		if state.session.Status != LifecycleStatusInterrupted {
			t.Fatalf("status = %q, want INTERRUPTED", state.session.Status)
		}
		if state.result.ResultStatus != ResultStatusPartial {
			t.Fatalf("result status = %q, want PARTIAL", state.result.ResultStatus)
		}
		if state.session.ResultSummary == nil || state.session.ResultSummary.ResultStatus != string(ResultStatusPartial) {
			t.Fatalf("result summary = %#v, want PARTIAL", state.session.ResultSummary)
		}
	})

	t.Run("unavailable result", func(t *testing.T) {
		state := &runtimeSessionState{
			session: SessionReadResult{
				SessionID: sessionID,
				Status:    LifecycleStatusRunning,
				Lifecycle: &LifecycleTimestamps{FinishedAt: &interruptedAt},
			},
		}
		finalizeInterruptedTerminalSession(state, SessionReadResult{SessionID: sessionID}, ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusNotReady,
		})
		if state.result.ResultStatus != ResultStatusUnavailable {
			t.Fatalf("result status = %q, want UNAVAILABLE", state.result.ResultStatus)
		}
		if state.result.Availability == nil || state.result.Availability.Reason != "SESSION_INTERRUPTED" {
			t.Fatalf("availability = %#v, want SESSION_INTERRUPTED", state.result.Availability)
		}
	})
}

// pkgmaintcheck:ignore-function-lines this restart integration test keeps the pre-restart and post-restart observable read assertions together.
// pkgmaintcheck:ignore-cyclomatic-complexity each assertion validates one durable partial-result field across the restart boundary.
func TestJavaScriptRuntimeService_PausePersistsStablePartialTerminalReadState(t *testing.T) {
	t.Parallel()
	store := &runtimeRecordingStore{}
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Persistence: store,
	})

	sessionID := "dur-sess-paused-restart-001"
	dispatchID := "dispatch-completed-1"
	if err := seedRuntimeSessionWithRunningDispatch(service, sessionID, dispatchID, "completed child"); err != nil {
		t.Fatalf("SeedRuntimeSessionWithRunningDispatch: %v", err)
	}

	service.mu.Lock()
	state := service.sessions[sessionID]
	state.dispatches[0].Status = DispatchStatusCompleted
	state.dispatches[0].OutputArtifactIDs = []string{"artifact-1"}
	state.dispatchStatusTransitions[dispatchID] = []DispatchStatus{
		DispatchStatusQueued,
		DispatchStatusRunning,
		DispatchStatusCompleted,
	}
	state.artifacts = []ArtifactSummary{{
		ID:         "artifact-1",
		Kind:       "CHILD_RESULT",
		Visibility: "session",
		DispatchID: dispatchID,
	}}
	state.session.ArtifactCount = 1
	state.session.ArtifactRefs = artifactRefsFromSummaries(state.artifacts)
	state.events = rebuildRuntimeSessionCanonicalEvents(state)
	service.mu.Unlock()

	paused, err := service.Pause(context.Background(), sessionID, ControlRequest{RequestID: "pause-restart-001"})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.Status != LifecycleStatusPaused {
		t.Fatalf("pause status = %q, want PAUSED", paused.Status)
	}

	wantSession, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession before restart: %v", err)
	}
	wantResult, err := service.GetResult(context.Background(), sessionID, ResultRequest{Mode: ResultModePartial, IncludeArtifacts: true})
	if err != nil {
		t.Fatalf("GetResult before restart: %v", err)
	}
	wantDispatches, err := service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches before restart: %v", err)
	}
	wantEvents, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents before restart: %v", err)
	}

	restarted := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Persistence: store,
	})

	gotSession, err := restarted.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession after restart: %v", err)
	}
	gotResult, err := restarted.GetResult(context.Background(), sessionID, ResultRequest{Mode: ResultModePartial, IncludeArtifacts: true})
	if err != nil {
		t.Fatalf("GetResult after restart: %v", err)
	}
	gotDispatches, err := restarted.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches after restart: %v", err)
	}
	gotEvents, err := restarted.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents after restart: %v", err)
	}

	if !reflect.DeepEqual(gotSession, wantSession) {
		t.Fatalf("session changed across restart:\ngot  %#v\nwant %#v", gotSession, wantSession)
	}
	if !reflect.DeepEqual(gotResult, wantResult) || gotResult.ResultStatus != ResultStatusNotReady || gotResult.Availability == nil {
		t.Fatalf("result changed across restart: got %#v want %#v", gotResult, wantResult)
	}
	if !reflect.DeepEqual(gotDispatches, wantDispatches) {
		t.Fatalf("dispatches changed across restart: got %#v want %#v", gotDispatches, wantDispatches)
	}
	if len(gotEvents.Events) != len(wantEvents.Events) {
		t.Fatalf("event count changed across restart: got %d want %d", len(gotEvents.Events), len(wantEvents.Events))
	}
	for index := range wantEvents.Events {
		var gotEvent, wantEvent any
		if err := json.Unmarshal(gotEvents.Events[index], &gotEvent); err != nil {
			t.Fatalf("decode restarted event %d: %v", index, err)
		}
		if err := json.Unmarshal(wantEvents.Events[index], &wantEvent); err != nil {
			t.Fatalf("decode live event %d: %v", index, err)
		}
		if !reflect.DeepEqual(gotEvent, wantEvent) {
			t.Fatalf("event %d changed across restart: got %#v want %#v", index, gotEvent, wantEvent)
		}
	}
}

func TestJavaScriptRuntimeService_PausePersistenceFailureKeepsRunningProjection(t *testing.T) {
	t.Parallel()
	store := &runtimeRecordingStore{saveErr: errors.New("pause persistence unavailable")}
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Persistence: store,
	})

	sessionID := "dur-sess-pause-persist-failure-001"
	if err := seedRuntimeSessionWithRunningDispatch(service, sessionID, "dispatch-1", "running child"); err != nil {
		t.Fatalf("SeedRuntimeSessionWithRunningDispatch: %v", err)
	}
	cancelCalls := 0
	service.mu.Lock()
	service.sessions[sessionID].runCancel = func() { cancelCalls++ }
	service.mu.Unlock()

	_, err := service.Pause(context.Background(), sessionID, ControlRequest{})
	if err == nil || !strings.Contains(err.Error(), "persist durable session snapshot") {
		t.Fatalf("Pause error = %v, want persistence failure", err)
	}
	read, readErr := service.GetSession(context.Background(), sessionID)
	if readErr != nil {
		t.Fatalf("GetSession: %v", readErr)
	}
	if read.Status != LifecycleStatusRunning || read.Lifecycle == nil || read.Lifecycle.PausedAt != nil {
		t.Fatalf("session after rejected pause = %#v, want unchanged RUNNING projection", read)
	}
	if _, err := service.Cancel(context.Background(), sessionID, ControlRequest{}); err != nil {
		t.Fatalf("Cancel after rejected pause: %v", err)
	}
	if cancelCalls != 1 {
		t.Fatalf("cancel calls after rejected pause = %d, want 1", cancelCalls)
	}
}

func TestInterruptedTerminalTimestamp_PrefersSessionLifecycle(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	interruptedAt := time.Date(2026, 6, 29, 11, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 6, 29, 12, 30, 0, 0, time.UTC)

	got := interruptedTerminalTimestamp(
		SessionReadResult{Lifecycle: &LifecycleTimestamps{InterruptedAt: &interruptedAt}},
		SessionReadResult{Lifecycle: &LifecycleTimestamps{StartedAt: &startedAt}},
	)
	if got == nil || !got.Equal(interruptedAt) {
		t.Fatalf("timestamp = %v, want interruptedAt", got)
	}

	got = interruptedTerminalTimestamp(
		SessionReadResult{Lifecycle: &LifecycleTimestamps{FinishedAt: &finishedAt}},
		SessionReadResult{},
	)
	if got == nil || !got.Equal(finishedAt) {
		t.Fatalf("timestamp = %v, want finishedAt", got)
	}

	got = interruptedTerminalTimestamp(
		SessionReadResult{Lifecycle: &LifecycleTimestamps{UpdatedAt: &updatedAt}},
		SessionReadResult{},
	)
	if got == nil || !got.Equal(updatedAt) {
		t.Fatalf("timestamp = %v, want updatedAt", got)
	}

	got = interruptedTerminalTimestamp(SessionReadResult{}, SessionReadResult{
		Lifecycle: &LifecycleTimestamps{StartedAt: &startedAt},
	})
	if got == nil || !got.Equal(startedAt) {
		t.Fatalf("timestamp = %v, want prior startedAt", got)
	}
}

func TestResumeHelperFunctions_CoverMergeCloneAndPolicyPaths(t *testing.T) {
	t.Parallel()
	existing := []factory.JavaScriptRuntimeRecord{{Kind: factory.JavaScriptRecordKindCheckpoint, Checkpoint: &factory.JavaScriptCheckpointRecord{ID: "cp-1"}}}
	resumed := []factory.JavaScriptRuntimeRecord{{Kind: factory.JavaScriptRecordKindChildDispatch, ChildDispatch: &factory.JavaScriptChildDispatchRecord{DispatchID: "dispatch-1"}}}
	merged := mergeRuntimeRecords(existing, resumed)
	if len(merged) != 2 {
		t.Fatalf("merged records = %d, want 2", len(merged))
	}
	if len(mergeRuntimeRecords(nil, resumed)) != 1 {
		t.Fatal("mergeRuntimeRecords(nil, resumed) should clone resumed records")
	}
	if len(mergeRuntimeRecords(existing, nil)) != 1 {
		t.Fatal("mergeRuntimeRecords(existing, nil) should clone existing records")
	}

	policy := workflowPolicyFromSessionPolicy(PolicyProjection{})
	defaultPolicy := factory.DefaultJavaScriptPolicy()
	if policy.Mode != defaultPolicy.Mode {
		t.Fatalf("policy mode = %q, want default %q", policy.Mode, defaultPolicy.Mode)
	}
	customPolicy := workflowPolicyFromSessionPolicy(PolicyProjection{
		Effective: map[string]any{"mode": factory.JavaScriptPolicyModeReadOnly},
	})
	if customPolicy.Mode != factory.JavaScriptPolicyModeReadOnly {
		t.Fatalf("policy mode = %q, want %q", customPolicy.Mode, factory.JavaScriptPolicyModeReadOnly)
	}

	summary := &factory.JavaScriptCheckpointSummary{
		CheckpointID:         "checkpoint-1",
		CompletedDispatchIDs: []string{"dispatch-1"},
		PendingDispatchIDs:   []string{"dispatch-2"},
		ArtifactIDs:          []string{"artifact-1"},
		CheckpointState:      map[string]any{"phase": "execute"},
		CreatedAt:            time.Now().UTC(),
	}
	cloned := cloneCheckpointSummary(summary)
	if cloned == nil || cloned.CheckpointID != summary.CheckpointID {
		t.Fatalf("cloneCheckpointSummary = %#v", cloned)
	}
	cloned.CompletedDispatchIDs[0] = "mutated"
	if summary.CompletedDispatchIDs[0] != "dispatch-1" {
		t.Fatal("cloneCheckpointSummary should deep-copy completed dispatch ids")
	}

	if latestCheckpointSummaryFromRuntime(checkpointfixtures.CheckpointSummariesFixture{}, "dur-sess-1", nil, nil) != nil {
		t.Fatal("latestCheckpointSummaryFromRuntime(nil state) = summary, want nil")
	}
}

func TestApplyRuntimeSuccessProjection_InvalidResultMarksFailed(t *testing.T) {
	t.Parallel()
	sessionID := "dur-sess-invalid-result-001"
	foreignURI := factory.FormatArtifactURI("dur-sess-other-001", "artifact-1")
	raw, err := json.Marshal(foreignURI)
	if err != nil {
		t.Fatalf("marshal foreign uri: %v", err)
	}
	state := &runtimeSessionState{
		artifacts: []ArtifactSummary{{
			ID:         "artifact-1",
			Kind:       "IMAGE",
			Label:      "output",
			Visibility: "PUBLIC",
		}},
	}
	applyRuntimeSuccessProjection(state, sessionID, factory.JavaScriptRuntimeOutcome{
		OK:    true,
		Value: factory.TypedValue{JSON: raw},
	}, time.Now().UTC())
	if state.session.Status != LifecycleStatusFailed {
		t.Fatalf("status = %q, want FAILED", state.session.Status)
	}
	if state.session.Failure == nil || state.session.Failure.Reason != "WORKFLOW_RUNTIME_INVALID_RESULT" {
		t.Fatalf("failure = %#v, want WORKFLOW_RUNTIME_INVALID_RESULT", state.session.Failure)
	}
}

func TestCheckpointEventProjection_BuildsCanonicalCheckpointEvents(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	sessionID := "dur-sess-checkpoint-events-001"
	state := &runtimeSessionState{
		session: SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusInterrupted,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Dialect:          "you-workflow-v1",
			Phase:            "execute",
			SourceHash:       "sha256:fixture",
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		},
		result: ResultReadResult{
			SessionID:     sessionID,
			SessionStatus: LifecycleStatusInterrupted,
			ResultStatus:  ResultStatusPartial,
		},
		checkpointSummary: &factory.JavaScriptCheckpointSummary{
			CheckpointID: "checkpoint-1",
			CreatedAt:    startedAt.Add(time.Minute),
		},
		runtimeRecords: []factory.JavaScriptRuntimeRecord{{
			Kind: factory.JavaScriptRecordKindCheckpoint,
			Checkpoint: &factory.JavaScriptCheckpointRecord{
				ID:      "checkpoint-1",
				Label:   "after-first-child",
				Summary: "checkpoint after first child",
			},
		}},
	}
	checkpoints := checkpointEventsFromRuntimeState(state)
	if len(checkpoints) != 1 || checkpoints[0].CheckpointID != "checkpoint-1" {
		t.Fatalf("checkpoint events = %#v", checkpoints)
	}
	if checkpoints[0].ResumabilityStatus != "RESUMABLE" {
		t.Fatalf("resumability = %q, want RESUMABLE", checkpoints[0].ResumabilityStatus)
	}

	events := BuildCanonicalRuntimeSessionEvents(state.session, state.result, runtimeDispatchEventInputFromState(state))
	events = appendCanonicalOrchestratorCheckpointEvents(events, state.session, checkpoints, canonicalEventSourceRuntimeService)
	found := false
	for _, raw := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if envelope.Type == "ORCHESTRATOR_CHECKPOINT_WRITTEN" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected ORCHESTRATOR_CHECKPOINT_WRITTEN canonical event")
	}
}

func TestPhaseEventProjection_PreservesOrderedRunningAndTerminalPhases(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	session := SessionReadResult{
		SessionID:        "dur-sess-phase-events-001",
		Status:           LifecycleStatusRunning,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Dialect:          "you-workflow-v1",
		Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		PhaseSummaries: []PhaseSummary{
			{Phase: "setup"}, {Phase: " "}, {Phase: "execute"},
		},
	}
	events := appendCanonicalOrchestratorPhaseEvents(nil, session, canonicalEventSourceRuntimeService)
	if got, want := phaseEventStatuses(t, events), []string{"setup:COMPLETED", "execute:ACTIVE"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("running phases = %v, want %v", got, want)
	}

	session.Status = LifecycleStatusSucceeded
	events = appendCanonicalOrchestratorPhaseEvents(nil, session, canonicalEventSourceRuntimeService)
	if got, want := phaseEventStatuses(t, events), []string{"setup:COMPLETED", "execute:COMPLETED"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("terminal phases = %v, want %v", got, want)
	}
	if got := appendCanonicalOrchestratorPhaseEvents(events, SessionReadResult{}, canonicalEventSourceRuntimeService); len(got) != len(events) {
		t.Fatalf("empty phase projection changed event count from %d to %d", len(events), len(got))
	}
}

func phaseEventStatuses(t *testing.T, events []json.RawMessage) []string {
	t.Helper()
	statuses := make([]string, 0, len(events))
	for _, raw := range events {
		var event struct {
			Context struct {
				PhaseID *string `json:"phaseId"`
			} `json:"context"`
			Payload struct {
				PhaseStatus string `json:"phaseStatus"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("decode phase event: %v", err)
		}
		if event.Context.PhaseID != nil && event.Payload.PhaseStatus != "" {
			statuses = append(statuses, *event.Context.PhaseID+":"+event.Payload.PhaseStatus)
		}
	}
	return statuses
}

func TestJavaScriptRuntimeService_FactoryEventObserverDeliversOnlyUnseenEvents(t *testing.T) {
	t.Parallel()
	const sessionID = "dur-sess-observer-events-001"
	session := SessionReadResult{
		SessionID: sessionID, Status: LifecycleStatusRunning,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
	}
	state := &runtimeSessionState{
		session: session,
		result:  ResultReadResult{SessionID: sessionID, SessionStatus: LifecycleStatusRunning},
	}
	state.events = BuildCanonicalRuntimeSessionEvents(state.session, state.result)
	service := &JavaScriptRuntimeService{sessions: map[string]*runtimeSessionState{sessionID: state}}
	var delivered []interfaces.FactoryEvent
	stop := service.observeFactoryEvents(state, func(events []interfaces.FactoryEvent) {
		delivered = append(delivered, events...)
	})
	service.presentCurrentFactoryEvents(sessionID)
	service.presentCurrentFactoryEvents(sessionID)
	if len(delivered) != len(state.events) {
		t.Fatalf("delivered %d events after duplicate presentation, want %d", len(delivered), len(state.events))
	}
	stop()
	service.presentCurrentFactoryEvents(sessionID)
	if len(delivered) != len(state.events) {
		t.Fatalf("delivery continued after observer stopped: got %d, want %d", len(delivered), len(state.events))
	}
	if stopNil := service.observeFactoryEvents(state, nil); stopNil == nil {
		t.Fatal("nil observer cleanup is nil")
	} else {
		stopNil()
	}
	service.unregisterFactoryEventConsumer("missing-session")
	service.presentCurrentFactoryEvents("missing-session")
}

func TestRuntimeRecordEvents_ReconcileAppendOnlyPhaseCheckpointPhaseHistory(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 7, 22, 20, 0, 0, 0, time.UTC)
	const sessionID = "dur-sess-append-only-events-001"
	records := []factory.JavaScriptRuntimeRecord{
		{Sequence: 1, Kind: factory.JavaScriptRecordKindPhase, Phase: &factory.JavaScriptPhaseRecord{Name: "plan"}},
		{Sequence: 2, Kind: factory.JavaScriptRecordKindCheckpoint, Checkpoint: &factory.JavaScriptCheckpointRecord{ID: "checkpoint-plan", Label: "plan-ready"}},
		{Sequence: 3, Kind: factory.JavaScriptRecordKindPhase, Phase: &factory.JavaScriptPhaseRecord{Name: "execute"}},
	}
	state := &runtimeSessionState{
		session: SessionReadResult{
			SessionID: sessionID, Status: LifecycleStatusRunning,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Dialect:          "you-workflow-v1", SourceHash: "sha256:append-only",
			Lifecycle: &LifecycleTimestamps{StartedAt: &startedAt},
		},
		result: ResultReadResult{
			SessionID: sessionID, SessionStatus: LifecycleStatusRunning,
			ResultStatus: ResultStatusNotReady,
		},
		checkpointSummary: &factory.JavaScriptCheckpointSummary{
			CheckpointID: "checkpoint-plan", CreatedAt: startedAt.Add(time.Second),
		},
		runtimeRecords: append(append([]factory.JavaScriptRuntimeRecord(nil), records...), records...),
		eventConsumer:  func([]interfaces.FactoryEvent) {},
	}
	state.events = BuildCanonicalRuntimeSessionEvents(state.session, state.result)
	running := rebuildRuntimeSessionCanonicalEvents(state)
	assertStrictCanonicalSequences(t, running)
	if got, want := phaseEventStatuses(t, running), []string{"plan:ACTIVE", "plan:COMPLETED", "execute:ACTIVE"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("running phase transitions = %v, want %v", got, want)
	}

	state.events = running
	state.session.Status = LifecycleStatusSucceeded
	state.result.SessionStatus = LifecycleStatusSucceeded
	state.result.ResultStatus = ResultStatusFinal
	terminal := rebuildRuntimeSessionCanonicalEvents(state)
	assertStrictCanonicalSequences(t, terminal)
	if got, want := phaseEventStatuses(t, terminal), []string{"plan:ACTIVE", "plan:COMPLETED", "execute:ACTIVE", "execute:COMPLETED"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("terminal phase transitions = %v, want %v", got, want)
	}
	if len(terminal) <= len(running) {
		t.Fatalf("terminal events = %d, want append beyond %d running events", len(terminal), len(running))
	}
	for index := range running {
		if string(terminal[index]) != string(running[index]) {
			t.Fatalf("published event %d was mutated:\nrunning=%s\nterminal=%s", index, running[index], terminal[index])
		}
	}
}

func assertStrictCanonicalSequences(t *testing.T, events []json.RawMessage) {
	t.Helper()
	previousSequence := 0
	previousSessionSequence := -1
	for index, raw := range events {
		var event interfaces.FactoryEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("decode event %d: %v", index, err)
		}
		if event.Context.Sequence <= previousSequence || event.Context.SessionSequence == nil ||
			*event.Context.SessionSequence <= previousSessionSequence {
			t.Fatalf("event %d sequence context is not increasing: %#v", index, event.Context)
		}
		previousSequence = event.Context.Sequence
		previousSessionSequence = *event.Context.SessionSequence
	}
}

func TestFakeService_ResumeInterruptedSession_ReturnsUnsupported(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	_, err := service.ResumeInterruptedSession(context.Background(), "dur-sess-petri-run-001", ResumeSessionRequest{
		RequestID: "req-fake-resume-unsupported-001",
	})
	if !errors.Is(err, ErrUnsupportedControl) {
		t.Fatalf("ResumeInterruptedSession error = %v, want ErrUnsupportedControl", err)
	}
}

// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func TestJavaScriptRuntimeService_ResumeInterruptedSession_PackageLocalCoverage(t *testing.T) {
	t.Parallel()
	const sessionID = "dur-sess-0123456789abcdef0123456789abcdef"
	projectRoot := t.TempDir()
	store := mustTestRuntimePersistenceStore(t, runtimepersist.DirForProjectRoot(projectRoot))
	startedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	startRequest := StartRequest{
		RequestID: "req-package-resume-start-001",
		Source: Source{
			Kind:         factory.WorkflowSourceKindWorkflowName,
			WorkflowName: "resumable-two-step-fake-children",
		},
		Args: map[string]any{"subject": "workflows"},
	}
	checkpointSummary := checkpointfixtures.ResumableCheckpointSummaryResult()
	state := runtimeSessionState{
		session: SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusInterrupted,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Dialect:          "you-workflow-v1",
			SourceHash:       "sha256:scripted",
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt, InterruptedAt: &startedAt},
		},
		result: ResultReadResult{
			SessionID:     sessionID,
			SessionStatus: LifecycleStatusInterrupted,
			ResultStatus:  ResultStatusPartial,
		},
		dispatches: []DispatchSummary{{
			ID: "dispatch-1", Status: DispatchStatusCompleted, Attempt: 1,
		}},
		runtimeRecords: []factory.JavaScriptRuntimeRecord{
			{
				Sequence: 1,
				Kind:     factory.JavaScriptRecordKindChildDispatch,
				ChildDispatch: &factory.JavaScriptChildDispatchRecord{
					DispatchID: "dispatch-1", ChildIndex: 1,
					Status: factory.JavaScriptChildDispatchStatusCompleted,
					Output: map[string]any{"text": "step one"},
				},
			},
			{
				Sequence: 2,
				Kind:     factory.JavaScriptRecordKindCheckpoint,
				Checkpoint: &factory.JavaScriptCheckpointRecord{
					ID: "checkpoint-1", Label: "after-step-one",
				},
			},
		},
		checkpointSummary: checkpointSummary,
		startRequest:      &startRequest,
		resolvedSource: ResolvedSource{
			Kind:       factory.WorkflowSourceKindWorkflowName,
			SourceRef:  "resumable-two-step-fake-children.workflow.js",
			SourceHash: "sha256:scripted",
			Dialect:    "you-workflow-v1",
		},
		sourceContent: "scripted resumable workflow",
	}
	state.events = rebuildRuntimeSessionCanonicalEvents(&state)
	encoded, err := json.Marshal(persistedSnapshotFromRuntimeState(state))
	if err != nil {
		t.Fatalf("marshal interrupted snapshot: %v", err)
	}
	if err := store.Save(sessionID, encoded); err != nil {
		t.Fatalf("persist interrupted snapshot: %v", err)
	}

	var resumeContextCalls int
	workflows := factoryruntimefixtures.ScriptedJavaScriptWorkflows{
		ResumeContextFunc: func(
			summary factory.JavaScriptCompletedCheckpointSummary,
			records []factory.JavaScriptRuntimeRecord,
		) factory.JavaScriptResumeContext {
			resumeContextCalls++
			if len(summary.CompletedDispatchIDs) != 1 || summary.CompletedDispatchIDs[0] != "dispatch-1" || len(records) != 2 {
				t.Fatalf("resume inputs = %#v / %#v", summary, records)
			}
			return factory.JavaScriptResumeContext{
				CompletedDispatchIDs: []string{"dispatch-1"},
			}
		},
		RunFunc: func(
			_ context.Context,
			request factory.JavaScriptRuntimeRequest,
			_ factory.JavaScriptRuntimeHooks,
		) (factory.JavaScriptRuntimeOutcome, error) {
			if request.Resume == nil || len(request.Resume.CompletedDispatchIDs) != 1 {
				t.Fatalf("runtime resume context = %#v", request.Resume)
			}
			value, marshalErr := json.Marshal(map[string]any{"status": "resumed"})
			return factory.JavaScriptRuntimeOutcome{
				OK: true, Value: factory.TypedValue{JSON: value},
			}, marshalErr
		},
	}
	resumedService := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: store,
		Workflows:   workflows,
	})

	resumed, err := resumedService.ResumeInterruptedSession(context.Background(), sessionID, ResumeSessionRequest{
		RequestID: "req-package-resume-resume-001",
	})
	if err != nil {
		t.Fatalf("ResumeInterruptedSession: %v", err)
	}
	if resumed.Status != string(LifecycleStatusResuming) && resumed.Status != string(LifecycleStatusSucceeded) {
		t.Fatalf("resumed status = %q, want RESUMING or SUCCEEDED", resumed.Status)
	}

	if resumed.Status != string(LifecycleStatusSucceeded) {
		waitForResumeCoverageSessionStatus(t, resumedService, sessionID, LifecycleStatusSucceeded, 5*time.Second)
	}
	if resumeContextCalls != 1 {
		t.Fatalf("resume context calls = %d, want 1", resumeContextCalls)
	}
}

type resumeCoverageBlockingProvider struct {
	mu              sync.Mutex
	callCount       int
	blockedOnce     bool
	contextCanceled int
}

func newResumeCoverageBlockingProvider() *resumeCoverageBlockingProvider {
	return &resumeCoverageBlockingProvider{}
}

func (p *resumeCoverageBlockingProvider) Execute(
	ctx context.Context,
	input workerexecution.InvocationInput,
) (workerexecution.InvocationResult, error) {
	p.mu.Lock()
	p.callCount++
	call := p.callCount
	alreadyBlocked := p.blockedOnce
	p.mu.Unlock()

	if call == 1 {
		response := workerexecution.InferenceResponse{
			Content: `{"text":"live:resumable-two-step-fake-children:step-one:step-one:workflows","label":"step-one"}`,
			ProviderSession: &workerexecution.ProviderSessionMetadata{
				Provider: "mock",
				Kind:     "session_id",
				ID:       "live-provider-session-1",
			},
		}
		return workerexecution.InvocationResult{
			Response: response, Attempt: input.Attempt,
			ProviderSession: workerexecution.CloneProviderSessionMetadata(response.ProviderSession),
		}, nil
	}

	if !alreadyBlocked {
		p.mu.Lock()
		p.blockedOnce = true
		p.mu.Unlock()

		<-ctx.Done()
		p.mu.Lock()
		p.contextCanceled++
		p.mu.Unlock()
		return workerexecution.InvocationResult{Attempt: input.Attempt}, ctx.Err()
	}

	response := workerexecution.InferenceResponse{
		Content: `{"text":"live:resumable-two-step-fake-children:step-two:step-two:workflows","label":"step-two"}`,
		ProviderSession: &workerexecution.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "live-provider-session-2",
		},
	}
	return workerexecution.InvocationResult{
		Response: response, Attempt: input.Attempt,
		ProviderSession: workerexecution.CloneProviderSessionMetadata(response.ProviderSession),
	}, nil
}

func (p *resumeCoverageBlockingProvider) resumeCoverageCallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.callCount
}

var _ workerexecution.InvocationExecutor = (*resumeCoverageBlockingProvider)(nil)

func (p *resumeCoverageBlockingProvider) waitForCanceledResumeCoverageInfer(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		canceled := p.contextCanceled > 0
		p.mu.Unlock()
		if canceled {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for blocked provider infer cancellation")
}

func setupResumeCoverageWorkflowFixture(t *testing.T) string {
	t.Helper()
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, factory.WorkflowSourceProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	path := filepath.Join("..", "..", "..", "..", "..", "tests", "fixtures", "javascript_runtime", "resumable-two-step-fake-children.workflow.js")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read workflow fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "resumable-two-step-fake-children.js"), raw, 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
}

func waitForResumeCoverageSessionStatus(
	t *testing.T,
	service *JavaScriptRuntimeService,
	sessionID string,
	want LifecycleStatus,
	timeout time.Duration,
) SessionReadResult {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		read, err := service.GetSession(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if read.Status == want {
			return read
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("session %s did not reach status %s within %s", sessionID, want, timeout)
	return SessionReadResult{}
}

func waitForResumeCoverageDispatchStatus(
	t *testing.T,
	service Service,
	sessionID, dispatchID string,
	want DispatchStatus,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		dispatch, err := service.GetDispatch(context.Background(), sessionID, dispatchID)
		if err == nil && dispatch.Status == want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("dispatch %s did not reach status %s within %s", dispatchID, want, timeout)
}

func TestJavaScriptRuntimeServiceWriteRecordingUsesCanonicalSnapshotAndCorrelatesFailure(t *testing.T) {
	t.Parallel()
	const sessionID = "dur-sess-1234567890abcdef1234567890abcdef"
	observedAt := time.Date(2026, 7, 12, 16, 30, 0, 0, time.UTC)
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	service.sessions[sessionID] = &runtimeSessionState{
		session:        SessionReadResult{SessionID: sessionID, Status: LifecycleStatusSucceeded, OrchestratorKind: interfaces.OrchestratorKindJavaScript, ResolvedSource: ResolvedSource{SourceRef: "workflow/audit.js"}, SourceHash: "sha256:" + strings.Repeat("1", 64), Policy: PolicyProjection{EffectiveHash: "sha256:" + strings.Repeat("2", 64)}},
		startRequest:   &StartRequest{Args: map[string]any{"customer": "north"}},
		artifacts:      []ArtifactSummary{{ID: "artifact-1", Kind: "RESULT", Visibility: "PUBLIC", ContentHash: "sha256:" + strings.Repeat("3", 64), SizeBytes: 2, CreatedAt: &observedAt}},
		events:         []json.RawMessage{json.RawMessage(`{"id":"event-1","type":"SESSION_COMPLETED","context":{"sequence":0,"eventTime":"2026-07-12T16:30:00Z"},"payload":{"artifactIds":["artifact-1"]}}`)},
		runtimeRecords: []factory.JavaScriptRuntimeRecord{{Kind: factory.JavaScriptRecordKindCheckpoint, Checkpoint: &factory.JavaScriptCheckpointRecord{ID: "checkpoint-secret", State: map[string]any{"secret": "raw-state"}}}},
	}
	path := filepath.Join(t.TempDir(), "session.recording.json")
	if err := service.WriteRecording(context.Background(), sessionID, path); err != nil {
		t.Fatalf("WriteRecording: %v", err)
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(encoded), "checkpoint-secret") || strings.Contains(string(encoded), "raw-state") {
		t.Fatalf("recording leaked runtime state: %s", encoded)
	}
	badPath := filepath.Join(t.TempDir(), "missing", "\x00invalid")
	err = service.WriteRecording(context.Background(), sessionID, badPath)
	var recordingErr *RecordingError
	if !errors.As(err, &recordingErr) || recordingErr.SessionID != sessionID || recordingErr.Path != badPath {
		t.Fatalf("WriteRecording failure = %#v", err)
	}
	read, readErr := service.GetSession(context.Background(), sessionID)
	if readErr != nil || read.Status != LifecycleStatusSucceeded {
		t.Fatalf("live session changed after recording failure: read=%#v err=%v", read, readErr)
	}
}

func TestJavaScriptRuntimeService_StartSync_WorkflowFilePolicyDeniesDisallowedModel(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	workflowPath := filepath.Join(projectRoot, "workflow.js")
	workflowSource := `agent.run({
  prompt: "summarize workflows",
  label: "denied-model",
  model: "gpt-denied",
});
return { ok: true };`
	if err := os.WriteFile(workflowPath, []byte(workflowSource), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	defaultPolicy := json.RawMessage(`{
  "mode":"READ_ONLY",
  "maxAgents":4,
  "concurrency":2,
  "allowedModels":["gpt-allowed"],
  "allowedReasoningEfforts":["low"]
}`)
	var requestedPolicy map[string]any
	if err := json.Unmarshal(defaultPolicy, &requestedPolicy); err != nil {
		t.Fatalf("unmarshal default policy: %v", err)
	}

	workflows := realJavaScriptWorkflowsForExecutionTest(t)
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: ChildExecutorModeFake,
		Workflows:         workflows,
	})

	started, err := service.StartSync(context.Background(), StartRequest{
		RequestID: "req-policy-denied-model",
		Source: Source{
			Kind:         factory.WorkflowSourceKindWorkflowFile,
			WorkflowFile: workflowPath,
			InlineWorkflow: &InlineWorkflowSource{
				DefaultPolicy: defaultPolicy,
			},
		},
		Args:            map[string]any{"prompt": "hello"},
		RequestedPolicy: requestedPolicy,
		Runtime:         &RuntimeOptions{ChildExecutorMode: ChildExecutorModeFake},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if started.Status != string(LifecycleStatusFailed) {
		t.Fatalf("status = %q, want FAILED; outcome = %#v", started.Status, started)
	}

	session, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	result, err := service.GetResult(context.Background(), started.SessionID, ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	failureMessage := ""
	if result.Failure != nil {
		failureMessage = result.Failure.Message
	} else if session.Failure != nil {
		failureMessage = session.Failure.Message
	}
	if !strings.Contains(failureMessage, `policy denied: model "gpt-denied" is not listed in allowedModels`) {
		t.Fatalf("session failure = %#v result failure = %#v, want stable policy diagnostic", session.Failure, result.Failure)
	}
}

func realJavaScriptWorkflowsForExecutionTest(t *testing.T) factory.JavaScriptWorkflows {
	t.Helper()
	return factoryruntimejavascript.New(localWorkflowSourceFilesForExecutionTest{}, os.UserHomeDir, filepath.EvalSymlinks)
}

type localWorkflowSourceFilesForExecutionTest struct{}

func (localWorkflowSourceFilesForExecutionTest) ReadDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}
func (localWorkflowSourceFilesForExecutionTest) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
func (localWorkflowSourceFilesForExecutionTest) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
