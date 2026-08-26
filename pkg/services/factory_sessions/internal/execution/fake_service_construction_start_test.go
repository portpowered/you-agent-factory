package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/internal/testutil/checkpointfixtures"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewFakeServiceRequiresExplicitClockBeforeFixtureIO(t *testing.T) {
	t.Parallel()

	if service, err := NewFakeService(nil); err == nil || service != nil ||
		!strings.Contains(err.Error(), "clock is required") {
		t.Fatalf("NewFakeService(nil) = (%#v, %v), want required clock", service, err)
	}
	if service, err := NewFakeServiceFromContractFixtures("missing.json", nil, nil); err == nil || service != nil ||
		!strings.Contains(err.Error(), "clock is required") {
		t.Fatalf("NewFakeServiceFromContractFixtures(nil clock) = (%#v, %v), want required clock before IO", service, err)
	}
}

func TestNewFakeServiceFromContractFixturesRequiresInjectedReader(t *testing.T) {
	t.Parallel()

	service, err := NewFakeServiceFromContractFixtures("ignored.json", fakeServiceTestClock(), nil)
	if err == nil || service != nil || !strings.Contains(err.Error(), "fixture file reader is required") {
		t.Fatalf("NewFakeServiceFromContractFixtures missing reader = (%#v, %v)", service, err)
	}
}

func TestLoadFakeScenariosUsesInjectedReader(t *testing.T) {
	t.Parallel()

	want := errors.New("fixture read unavailable")
	_, err := LoadFakeScenariosFromContractFixtures("catalog.json", func(path string) ([]byte, error) {
		if path != "catalog.json" {
			t.Fatalf("path = %q, want catalog.json", path)
		}
		return nil, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("LoadFakeScenariosFromContractFixtures error = %v, want %v", err, want)
	}
}

func startAsyncByRequestID(t *testing.T, service *FakeService, requestID string) AsyncStartResult {
	t.Helper()
	result, err := service.StartAsync(context.Background(), StartRequest{
		RequestID: requestID,
		Source:    Source{Kind: factory.WorkflowSourceKindFactoryID, FactoryID: "customer-support-triage"},
	})
	if err != nil {
		t.Fatalf("StartAsync(%q): %v", requestID, err)
	}
	return result
}

func TestFakeService_StartAsync_ProjectsFixtureScenarios(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	cases := []struct {
		requestID     string
		sessionID     string
		status        LifecycleStatus
		result        ResultStatus
		resultRequest ResultRequest
	}{
		{"req-petri-run-001", "dur-sess-petri-run-001", LifecycleStatusRunning, ResultStatusNotReady, ResultRequest{Mode: ResultModeFinal}},
		{"req-js-run-n-001", "dur-sess-js-run-n-001", LifecycleStatusRunning, ResultStatusPartial, ResultRequest{Mode: ResultModePartial}},
		{"req-js-awaiting-001", "dur-sess-js-awaiting-001", LifecycleStatusAwaitingApproval, ResultStatusNotReady, ResultRequest{Mode: ResultModeFinal}},
		{"req-petri-success-001", "dur-sess-petri-success-001", LifecycleStatusSucceeded, ResultStatusFinal, ResultRequest{Mode: ResultModeFinal}},
		{"req-js-failed-partial-001", "dur-sess-js-failed-partial-001", LifecycleStatusFailed, ResultStatusFailedWithPartial, ResultRequest{Mode: ResultModePartial}},
		{"req-petri-cancel-001", "dur-sess-petri-cancel-001", LifecycleStatusCanceled, ResultStatusUnavailable, ResultRequest{Mode: ResultModeFinal}},
		{"req-js-timeout-001", "dur-sess-js-timeout-001", LifecycleStatusRunning, ResultStatusNotReady, ResultRequest{Mode: ResultModeFinal}},
		{"req-js-interrupted-001", "dur-sess-js-interrupted-001", LifecycleStatusInterrupted, ResultStatusPartial, ResultRequest{Mode: ResultModePartial}},
	}
	for _, tc := range cases {
		t.Run(tc.requestID, func(t *testing.T) {
			started, err := service.StartAsync(context.Background(), StartRequest{
				RequestID: tc.requestID,
				Source:    Source{Kind: factory.WorkflowSourceKindFactoryID, FactoryID: "customer-support-triage"},
			})
			if err != nil {
				t.Fatalf("StartAsync: %v", err)
			}
			if started.SessionID != tc.sessionID {
				t.Fatalf("sessionId = %q, want %q", started.SessionID, tc.sessionID)
			}
			read, err := service.GetSession(context.Background(), tc.sessionID)
			if err != nil {
				t.Fatalf("GetSession: %v", err)
			}
			if read.Status != tc.status {
				t.Fatalf("status = %q, want %q", read.Status, tc.status)
			}
			result, err := service.GetResult(context.Background(), tc.sessionID, tc.resultRequest)
			if err != nil {
				t.Fatalf("GetResult: %v", err)
			}
			if result.ResultStatus != tc.result {
				t.Fatalf("resultStatus = %q, want %q", result.ResultStatus, tc.result)
			}
		})
	}
}

func TestFakeService_StartAsync_IdempotentReplay(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	req := StartRequest{
		RequestID: "req-idempotent-replay-001",
		Source: Source{
			Kind:         factory.WorkflowSourceKindWorkflowFile,
			WorkflowFile: ".claude/workflows/idempotent.yaml",
		},
		Args: map[string]any{"task": "replay"},
		RequestedPolicy: map[string]any{
			"policyHash": "req-policy-idempotent",
		},
	}
	first, err := service.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("first StartAsync: %v", err)
	}
	second, err := service.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("second StartAsync: %v", err)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("replay sessionId = %q, want %q", second.SessionID, first.SessionID)
	}
	conflict := req
	conflict.Args["task"] = "different"
	_, err = service.StartAsync(context.Background(), conflict)
	if !errors.Is(err, ErrExecutionRequestIDConflict) {
		t.Fatalf("error = %v, want ErrExecutionRequestIDConflict", err)
	}
}

func TestFakeService_StartAsync_ErrorBranches(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.StartAsync(canceledCtx, StartRequest{
		RequestID: "req-petri-run-001",
		Source:    Source{Kind: factory.WorkflowSourceKindFactoryID, FactoryID: "customer-support-triage"},
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled StartAsync error = %v", err)
	}

	if _, err := service.StartAsync(context.Background(), StartRequest{}); err == nil {
		t.Fatal("invalid StartAsync request should fail")
	}

	if _, err := service.StartAsync(context.Background(), StartRequest{
		RequestID: "missing-scenario",
		Source:    Source{Kind: factory.WorkflowSourceKindFactoryID, FactoryID: "customer-support-triage"},
	}); err == nil {
		t.Fatal("unknown scenario should fail")
	}
}

func TestFakeService_StartSync_TerminalAndTimeoutFixtures(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)

	terminal, err := service.StartSync(context.Background(), StartRequest{
		RequestID: "req-petri-success-001",
		Source:    Source{Kind: factory.WorkflowSourceKindFactoryID, FactoryID: "customer-support-triage"},
	})
	if err != nil {
		t.Fatalf("StartSync terminal: %v", err)
	}
	if terminal.SyncOutcome != SyncOutcomeCompleted {
		t.Fatalf("syncOutcome = %q, want COMPLETED", terminal.SyncOutcome)
	}
	if terminal.Status != string(LifecycleStatusSucceeded) {
		t.Fatalf("status = %q, want SUCCEEDED", terminal.Status)
	}

	timedOut, err := service.StartSync(context.Background(), StartRequest{
		RequestID: "req-js-timeout-001",
		Source:    Source{Kind: factory.WorkflowSourceKindWorkflowName, WorkflowName: "long-running-audit"},
		Wait:      &WaitOptions{TimeoutMillis: int64Ptr(30000)},
	})
	if err != nil {
		t.Fatalf("StartSync timeout: %v", err)
	}
	if timedOut.SyncOutcome != SyncOutcomeTimedOut || !timedOut.TimedOut {
		t.Fatalf("timeout response = %#v", timedOut)
	}
	if timedOut.SessionCanceledByTimeout {
		t.Fatal("sessionCanceledByTimeout = true, want false without cancel-on-timeout")
	}
}

func TestFakeService_StartSync_AppliesCancelOnTimeoutOverlay(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	timedOut, err := service.StartSync(context.Background(), StartRequest{
		RequestID: "req-js-timeout-001",
		Source:    Source{Kind: factory.WorkflowSourceKindWorkflowName, WorkflowName: "long-running-audit"},
		Wait: &WaitOptions{
			TimeoutMillis:   int64Ptr(30000),
			CancelOnTimeout: true,
		},
	})
	if err != nil {
		t.Fatalf("StartSync timeout with cancel: %v", err)
	}
	if !timedOut.SessionCanceledByTimeout {
		t.Fatal("sessionCanceledByTimeout = false, want true")
	}
	if timedOut.Status != string(LifecycleStatusCanceling) {
		t.Fatalf("status = %q, want CANCELING", timedOut.Status)
	}

	session, err := service.GetSession(context.Background(), "dur-sess-js-timeout-001")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Status != LifecycleStatusCanceling {
		t.Fatalf("session status = %q, want CANCELING", session.Status)
	}

	result, err := service.GetResult(context.Background(), "dur-sess-js-timeout-001", ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != ResultStatusUnavailable {
		t.Fatalf("resultStatus = %q, want UNAVAILABLE", result.ResultStatus)
	}
	if result.Availability == nil || result.Availability.Reason != "SESSION_CANCELED" {
		t.Fatalf("availability = %#v, want SESSION_CANCELED", result.Availability)
	}
}

func TestFakeService_StartSync_ErrorAndReplayBranches(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.StartSync(canceledCtx, StartRequest{
		RequestID: "req-petri-success-001",
		Source:    Source{Kind: factory.WorkflowSourceKindFactoryID, FactoryID: "customer-support-triage"},
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled StartSync error = %v", err)
	}

	if _, err := service.StartSync(context.Background(), StartRequest{}); err == nil {
		t.Fatal("invalid StartSync request should fail")
	}

	asyncService := newContractFakeService(t)
	if _, err := asyncService.StartAsync(context.Background(), StartRequest{
		RequestID: "req-petri-run-001",
		Source:    Source{Kind: factory.WorkflowSourceKindFactoryID, FactoryID: "customer-support-triage"},
	}); err != nil {
		t.Fatalf("seed StartAsync: %v", err)
	}
	if _, err := asyncService.StartSync(context.Background(), StartRequest{
		RequestID: "req-petri-run-001",
		Source:    Source{Kind: factory.WorkflowSourceKindFactoryID, FactoryID: "customer-support-triage"},
	}); !errors.Is(err, ErrExecutionRequestIDConflict) {
		t.Fatalf("async replay StartSync error = %v, want ErrExecutionRequestIDConflict", err)
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

func TestFakeService_StartAsync_ConcurrentIdempotentStarts(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	const workers = 12
	var wg sync.WaitGroup
	results := make([]AsyncStartResult, workers)
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index], errs[index] = service.StartAsync(context.Background(), StartRequest{
				RequestID: "req-petri-run-001",
				Source:    Source{Kind: factory.WorkflowSourceKindFactoryID, FactoryID: "customer-support-triage"},
			})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("StartAsync worker %d: %v", i, err)
		}
	}
	for i := 1; i < workers; i++ {
		if results[i].SessionID != results[0].SessionID {
			t.Fatalf("sessionId[%d] = %q, want %q", i, results[i].SessionID, results[0].SessionID)
		}
	}
}

func TestFakeService_ConstructorsAndHelpers(t *testing.T) {
	t.Parallel()
	scenario := BuiltinInterruptedRecoverableScenario()
	service := mustNewFakeService(t,
		FakeScenario{},
		scenario,
		FakeScenario{
			RequestID: "seeded-terminal",
			ListSummary: &DurableSessionListSummary{
				SessionID: "dur-sess-seeded-terminal",
				Status:    LifecycleStatusSucceeded,
			},
		},
	)
	if service == nil {
		t.Fatal("NewFakeService returned nil")
	}
	if _, ok := service.scenariosByRequestID[scenario.RequestID]; !ok {
		t.Fatal("scenario was not registered")
	}
	if len(service.persistedSeeds) != 2 {
		t.Fatalf("persistedSeeds = %#v", service.persistedSeeds)
	}

	loaded, err := NewFakeServiceFromContractFixtures(contractFixturesPath(t), fakeServiceTestClock(), fileeffects.ContractFixtureReader(os.ReadFile))
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}
	if loaded == nil || len(loaded.scenariosByRequestID) == 0 {
		t.Fatal("loaded fake service should contain fixture scenarios")
	}

	if seeded := appendPersistedSeed(nil, DurableSessionListSummary{SessionID: "a"}); len(seeded) != 1 {
		t.Fatalf("appendPersistedSeed = %#v", seeded)
	}
	seeded := appendPersistedSeed([]DurableSessionListSummary{{SessionID: "a"}}, DurableSessionListSummary{SessionID: "a"})
	if len(seeded) != 1 {
		t.Fatalf("deduped persisted seeds = %#v", seeded)
	}

	dispatch, ok := findDispatchSummary([]DispatchSummary{{ID: "disp-1"}}, "disp-1")
	if !ok || dispatch.ID != "disp-1" {
		t.Fatalf("findDispatchSummary found = %#v, %v", dispatch, ok)
	}
	if _, ok := findDispatchSummary([]DispatchSummary{{ID: "disp-1"}}, "missing"); ok {
		t.Fatal("findDispatchSummary should report missing dispatch")
	}
}

var executionTestSessionIdentity atomic.Uint64

func testSessionIDGenerator() string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", executionTestSessionIdentity.Add(1))
}

// constructorWorkflowContracts proves that constructor validation receives the
// three Factory Runtime root roles. These tests do not execute their methods.
type constructorWorkflowContracts struct {
	factory.JavaScriptWorkflowDefinitions
	factory.JavaScriptWorkflowRuntime
	factory.JavaScriptChildValues
}

func (c constructorWorkflowContracts) RunJavaScript(
	ctx context.Context,
	req factory.JavaScriptRuntimeRequest,
	hooks factory.JavaScriptRuntimeHooks,
) (factory.JavaScriptRuntimeOutcome, error) {
	return c.Run(ctx, req, hooks)
}

func (c constructorWorkflowContracts) ResumeJavaScript(
	summary factory.JavaScriptCompletedCheckpointSummary,
	records []factory.JavaScriptRuntimeRecord,
) factory.JavaScriptResumeContext {
	return c.ResumeContext(summary, records)
}

// serviceConfig keeps table-driven constructor tests compact without restoring
// a production dependency bag.
type serviceConfig struct {
	ProjectRoot       string
	ChildExecutorMode string
	Provider          providers.Service
	ProviderExecutor  workers.InvocationExecutor
	FakeScenarios     []FakeScenario
	Persistence       PersistenceChoice
	Clock             factory.Clock
	WorkerPresetIDs   map[string]struct{}
	WorkerSettings    factory.JavaScriptWorkerSettings
}

func newExecutionService(provider ExecutionProvider, config serviceConfig) (Service, error) {
	switch provider {
	case ExecutionProviderFake:
		clock := config.Clock
		if clock == nil {
			clock = durableFixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
		}
		return NewFakeService(clock, config.FakeScenarios...)
	case ExecutionProviderJavaScriptRuntime:
		workflows := constructorWorkflowContracts{}
		return NewJavaScriptExecutionService(
			config.ProjectRoot,
			config.ChildExecutorMode,
			firstInvocationExecutor(config.ProviderExecutor, config.Provider),
			config.Persistence,
			config.Clock,
			testSyncWaitScheduler{},
			checkpointfixtures.CheckpointSummariesFixture{
				BuildResult:  checkpointfixtures.ResumableCheckpointSummaryResult(),
				LatestResult: checkpointfixtures.ResumableCheckpointSummaryResult(),
			},
			workflows,
			workflows,
			workflows,
			config.WorkerPresetIDs,
			config.WorkerSettings,
			mustTestRecordingWriter(),
			testSessionIDGenerator,
			nil, nil, nil,
		)
	default:
		return nil, NewValidationError("provider", "unsupported execution provider")
	}
}

func firstInvocationExecutor(executor workers.InvocationExecutor, provider providers.Service) workers.InvocationExecutor {
	if executor != nil {
		return executor
	}
	if provider == nil {
		return nil
	}
	return constructorInvocationExecutor{}
}

// constructorInvocationExecutor is an inert root-contract value. Constructor
// tests validate dependency presence only; Workers owns invocation behavior.
type constructorInvocationExecutor struct {
	workers.InvocationExecutor
}

type testSyncWaitScheduler struct{}

func (testSyncWaitScheduler) Now() time.Time { return time.Now() }

func (testSyncWaitScheduler) After(duration time.Duration) <-chan time.Time {
	return time.After(duration)
}

func TestNewJavaScriptExecutionServiceRequiresSyncWaitScheduler(t *testing.T) {
	t.Parallel()
	workflows := constructorWorkflowContracts{}
	_, err := NewJavaScriptExecutionService(
		t.TempDir(),
		ChildExecutorModeFake,
		nil,
		DisabledPersistence(),
		durableFixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		nil,
		checkpointfixtures.CheckpointSummariesFixture{},
		workflows,
		workflows,
		workflows,
		nil,
		factory.JavaScriptWorkerSettings{},
		mustTestRecordingWriter(),
		testSessionIDGenerator,
		nil, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "sync wait scheduler is required") {
		t.Fatalf("NewJavaScriptExecutionService error = %v, want missing sync wait scheduler", err)
	}
}

func TestWaitSyncCompletionUsesInjectedClockAndRecurringScheduler(t *testing.T) {
	t.Parallel()
	clock := newControlledSyncWaitClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	service := &JavaScriptRuntimeService{
		clock:     clock,
		syncWaits: clock,
		sessions: map[string]*runtimeSessionState{
			"session-1": {session: SessionReadResult{SessionID: "session-1", Status: LifecycleStatusRunning}},
		},
	}

	result := make(chan SyncStartResult, 1)
	errs := make(chan error, 1)
	go func() {
		got, err := service.waitSyncCompletion(context.Background(), "session-1", 20*time.Millisecond, false)
		result <- got
		errs <- err
	}()

	if duration := <-clock.requests; duration != 10*time.Millisecond {
		t.Fatalf("first scheduled wait = %s, want 10ms", duration)
	}
	clock.Advance(10 * time.Millisecond)
	if duration := <-clock.requests; duration != 10*time.Millisecond {
		t.Fatalf("recurring scheduled wait = %s, want 10ms", duration)
	}
	select {
	case got := <-result:
		t.Fatalf("wait completed before injected deadline: %#v", got)
	default:
	}

	clock.Advance(10 * time.Millisecond)
	if err := <-errs; err != nil {
		t.Fatalf("waitSyncCompletion: %v", err)
	}
	got := <-result
	if !got.TimedOut || got.SyncOutcome != SyncOutcomeTimedOut {
		t.Fatalf("result = %#v, want injected-clock timeout", got)
	}
}

type controlledSyncWaitClock struct {
	mu       sync.Mutex
	now      time.Time
	waiters  []chan time.Time
	requests chan time.Duration
}

func newControlledSyncWaitClock(now time.Time) *controlledSyncWaitClock {
	return &controlledSyncWaitClock{now: now, requests: make(chan time.Duration, 4)}
}

func (c *controlledSyncWaitClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *controlledSyncWaitClock) After(duration time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	waiter := make(chan time.Time, 1)
	c.waiters = append(c.waiters, waiter)
	c.requests <- duration
	return waiter
}

func (c *controlledSyncWaitClock) Advance(duration time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(duration)
	now := c.now
	waiters := c.waiters
	c.waiters = nil
	c.mu.Unlock()
	for _, waiter := range waiters {
		waiter <- now
	}
}

func contractFixturesPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "..", "transports", "http", "testdata", "durable-session-contract-fixtures.json")
}

func newContractFakeService(t *testing.T) *FakeService {
	t.Helper()
	service, err := NewFakeServiceFromContractFixtures(contractFixturesPath(t), fakeServiceTestClock(), fileeffects.ContractFixtureReader(os.ReadFile))
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}
	return service
}

func fakeServiceTestClock() durableFixedClock {
	return durableFixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func mustNewFakeService(t *testing.T, scenarios ...FakeScenario) *FakeService {
	t.Helper()
	service, err := NewFakeService(fakeServiceTestClock(), scenarios...)
	if err != nil {
		t.Fatalf("NewFakeService: %v", err)
	}
	return service
}

func int64Ptr(value int64) *int64 {
	return &value
}

func assertCanonicalEventEnvelope(t *testing.T, raw json.RawMessage, eventType, id string) {
	t.Helper()
	var envelope struct {
		SchemaVersion string `json:"schemaVersion"`
		ID            string `json:"id"`
		Type          string `json:"type"`
		Context       struct {
			Sequence int `json:"sequence"`
		} `json:"context"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("Unmarshal event: %v", err)
	}
	if envelope.SchemaVersion != canonicalFactoryEventSchemaVersion {
		t.Fatalf("schemaVersion = %q, want %q", envelope.SchemaVersion, canonicalFactoryEventSchemaVersion)
	}
	if id != "" && envelope.ID != id {
		t.Fatalf("id = %q, want %q", envelope.ID, id)
	}
	if eventType != "" && envelope.Type != eventType {
		t.Fatalf("type = %q, want %q", envelope.Type, eventType)
	}
	if envelope.Context.Sequence <= 0 {
		t.Fatalf("sequence = %d, want positive", envelope.Context.Sequence)
	}
	if len(envelope.Payload) == 0 {
		t.Fatal("payload missing")
	}
}

func jsonEqual(left, right json.RawMessage) bool {
	var leftValue any
	var rightValue any
	return json.Unmarshal(left, &leftValue) == nil &&
		json.Unmarshal(right, &rightValue) == nil &&
		reflect.DeepEqual(leftValue, rightValue)
}

func canonicalTypedInternalEvent(t *testing.T, eventType, sessionID string, payload any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"schemaVersion": canonicalFactoryEventSchemaVersion,
		"id":            "internal/" + eventType,
		"type":          eventType,
		"context": map[string]any{
			"sequence":  99,
			"sessionId": sessionID,
		},
		"payload": payload,
	})
	if err != nil {
		t.Fatalf("marshal %s event: %v", eventType, err)
	}
	return raw
}
func TestDirectChildExecutor_MapsWorkersCanceledAndTimeoutToOneTerminalChild(t *testing.T) {
	for _, test := range []struct {
		name    string
		outcome workers.ExecutionOutcome
	}{
		{name: "canceled", outcome: workers.ExecutionOutcomeCanceled},
		{name: "timeout", outcome: workers.ExecutionOutcomeFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			invoker := &recordingWorkerExecution{result: workers.ExecuteResult{
				Outcome: test.outcome,
				Failure: &workers.ExecutionFailure{
					Type:      workers.WorkFailureTypeTimeout,
					Message:   "child timed out",
					RetryHint: true,
				},
			}}
			sink := newChildRecordSink()
			executor := newDirectChildExecutor("direct-session", invoker, sink, childTestValues{}, "/project", 0)

			result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{Prompt: "run"})
			if err == nil {
				t.Fatal("Execute error = nil, want terminal child failure")
			}
			if result.Status != factory.JavaScriptChildDispatchStatusFailed {
				t.Fatalf("child status = %q, want FAILED", result.Status)
			}
			if len(sink.records) != 1 {
				t.Fatalf("terminal child records = %d, want exactly one", len(sink.records))
			}
			terminal := sink.terminalChildDispatch(t)
			if terminal.FailureClassification != workers.WorkFailureTypeTimeout || terminal.FailureDetail == nil || terminal.FailureDetail.Message != "child timed out" {
				t.Fatalf("terminal failure = %#v, want typed timeout detail", terminal)
			}
		})
	}
}

func TestJavaScriptRuntimeService_CloseRejectsStartWithoutOrphaningReservation(t *testing.T) {
	t.Parallel()

	t.Run("async", func(t *testing.T) {
		exerciseStartAdmissionAfterClose(t, func(service *JavaScriptRuntimeService) error {
			_, err := service.StartAsync(context.Background(), inlineWorkflowStartRequest(
				"req-runtime-close-admission-async-001",
				simpleFinalWorkflowSource,
				map[string]any{"subject": "shutdown"},
				nil,
			))
			return err
		})
	})

	t.Run("wait sync", func(t *testing.T) {
		waitMillis := int64(100)
		exerciseStartAdmissionAfterClose(t, func(service *JavaScriptRuntimeService) error {
			request := inlineWorkflowStartRequest(
				"req-runtime-close-admission-sync-001",
				simpleFinalWorkflowSource,
				map[string]any{"subject": "shutdown"},
				nil,
			)
			request.Wait = &WaitOptions{TimeoutMillis: &waitMillis}
			_, err := service.StartSync(context.Background(), request)
			return err
		})
	})
}

func exerciseStartAdmissionAfterClose(
	t *testing.T,
	start func(*JavaScriptRuntimeService) error,
) {
	t.Helper()
	entered := make(chan struct{})
	release := make(chan struct{})
	workflows := &blockingWorkflowDefinitions{
		JavaScriptWorkflows: scriptedSuccessfulRuntimeWorkflows(map[string]any{"status": "started"}),
		entered:             entered,
		release:             release,
	}
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Workflows:   workflows,
	})
	startDone := make(chan error, 1)
	go func() { startDone <- start(service) }()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for start preparation")
	}
	if err := service.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	close(release)
	if err := <-startDone; !errors.Is(err, ErrDurableExecutionClosed) {
		t.Fatalf("start error = %v, want ErrDurableExecutionClosed", err)
	}

	service.mu.RLock()
	sessionCount := len(service.sessions)
	replayCount := len(service.startReplay)
	inflightCount := len(service.startInflight)
	service.mu.RUnlock()
	if sessionCount != 0 || replayCount != 0 || inflightCount != 0 {
		t.Fatalf(
			"close-rejected start left durable state: sessions=%d replay=%d inflight=%d",
			sessionCount, replayCount, inflightCount,
		)
	}
}

type blockingWorkflowDefinitions struct {
	factory.JavaScriptWorkflows
	entered chan struct{}
	release chan struct{}
}

func (workflows *blockingWorkflowDefinitions) ResolveSource(
	request factory.WorkflowSourceRequest,
	context factory.WorkflowSourceContext,
) factory.WorkflowSourceResolution {
	close(workflows.entered)
	<-workflows.release
	return workflows.JavaScriptWorkflows.ResolveSource(request, context)
}
