package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
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
