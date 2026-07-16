package fixtures_test

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	fse "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"github.com/portpowered/infinite-you/pkg/workers/provider"
)

func TestJavaScriptRuntimeService_AgentRunLiveChild_ProjectsRealDispatchInspection(t *testing.T) {
	provider := newFixtureMockProvider(workerexecution.InferenceResponse{
		Content: `{"text":"live:agent-run-fake-child:summarize-findings:summarize workflows:workflows"}`,
		ProviderSession: &workerexecution.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "live-provider-session-1",
		},
	})
	projectRoot := setupRuntimeWorkflowFixture(t, "agent-run-preset-child.workflow.js", "agent-run-preset-child")
	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
		Persistence:       runtimePersistence(projectRoot),
		WorkerSettings:    presetWorkerSettings(),
	})

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-agent-run-live-child",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-preset-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &fse.RuntimeOptions{
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	_, live, _ := loadLiveChildDispatchReads(t, service, completed)
	assertLiveChildDispatchInspection(t, service, completed, provider.callCount)
	assertSharedLiveChildDispatchContract(t, live)

	reloaded := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
		Persistence:       runtimePersistence(projectRoot),
		WorkerSettings:    presetWorkerSettings(),
	})
	_, replayed, _ := loadLiveChildDispatchReads(t, reloaded, completed)
	assertSharedLiveChildDispatchContract(t, replayed)
	if !reflect.DeepEqual(sharedDispatchContract(live), sharedDispatchContract(replayed)) {
		t.Fatalf("replayed shared dispatch = %#v, want live %#v", sharedDispatchContract(replayed), sharedDispatchContract(live))
	}
}

func assertSharedLiveChildDispatchContract(t *testing.T, dispatch fse.DispatchSummary) {
	t.Helper()
	want := sharedDispatchProjection{
		ID:              "dispatch-1",
		PresetID:        "careful-review",
		ModelProvider:   "CODEX",
		Model:           "gpt-test",
		ReasoningEffort: "medium",
		RunnerID:        "",
		Provider:        "mock",
		ProviderSession: fse.ProviderSessionRef{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "live-provider-session-1",
		},
	}
	if got := sharedDispatchContract(dispatch); !reflect.DeepEqual(got, want) {
		t.Fatalf("shared dispatch contract = %#v, want %#v", got, want)
	}
}

type sharedDispatchProjection struct {
	ID              string
	PresetID        string
	ModelProvider   string
	Model           string
	ReasoningEffort string
	RunnerID        string
	Provider        string
	ProviderSession fse.ProviderSessionRef
}

func sharedDispatchContract(dispatch fse.DispatchSummary) sharedDispatchProjection {
	projection := sharedDispatchProjection{
		ID:              dispatch.ID,
		PresetID:        dispatch.PresetID,
		ModelProvider:   dispatch.ModelProvider,
		Model:           dispatch.Model,
		ReasoningEffort: dispatch.ReasoningEffort,
		RunnerID:        dispatch.RunnerID,
		Provider:        dispatch.Provider,
	}
	if len(dispatch.ProviderSessionRefs) == 1 {
		projection.ProviderSession = dispatch.ProviderSessionRefs[0]
	}
	return projection
}

type fixtureMockProvider struct {
	response  workerexecution.InferenceResponse
	callCount int
}

func newFixtureMockProvider(response workerexecution.InferenceResponse) *fixtureMockProvider {
	return &fixtureMockProvider{response: response}
}

func (m *fixtureMockProvider) Infer(_ context.Context, _ workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	m.callCount++
	return m.response, nil
}

func (m *fixtureMockProvider) CallCount() int {
	return m.callCount
}

func TestJavaScriptRuntimeService_AgentRunFakeChild_RemainsDefaultWithoutRuntimeOverride(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-agent-run-fake-child-default",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	dispatchDetail, err := service.GetDispatch(context.Background(), completed.SessionID, "dispatch-1")
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatchDetail.JavaScript == nil || dispatchDetail.JavaScript.ExecutionMode != workflowruntime.ChildExecutionModeFake {
		t.Fatalf("dispatch javascript = %#v, want fake execution mode", dispatchDetail.JavaScript)
	}
}

func TestJavaScriptRuntimeService_AgentRunLiveChild_TimeoutInterruptsProviderInfer(t *testing.T) {
	provider := newBlockingFixtureProvider()
	projectRoot := setupRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
	})

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-agent-run-live-child-timeout",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &fse.RuntimeOptions{
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
		RequestedPolicy: map[string]any{
			"maxRunDurationMs": (2 * time.Second).Milliseconds(),
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	read, err := service.GetSession(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Status != fse.LifecycleStatusTimedOut {
		t.Fatalf("session status = %q, want TIMED_OUT", read.Status)
	}
	if read.Failure == nil || read.Failure.Reason != "WORKFLOW_RUNTIME_TIMEOUT" {
		t.Fatalf("session failure = %#v, want WORKFLOW_RUNTIME_TIMEOUT", read.Failure)
	}
	provider.waitForInferStart(t)
	waitForInferContextHonored(t, provider, 2*time.Second)
}

func TestJavaScriptRuntimeService_AgentRunLiveChild_StartAsyncProjectsRunningDispatchForInterrupt(t *testing.T) {
	provider := newBlockingFixtureProvider()
	projectRoot := setupRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
	})

	started, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-agent-run-live-child-interrupt",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &fse.RuntimeOptions{
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	provider.waitForInferStart(t)
	dispatch := waitForListedDispatch(t, service, started.SessionID, "dispatch-1", 2*time.Second)
	if dispatch.Status != fse.DispatchStatusRunning {
		t.Fatalf("dispatch status before interrupt = %q, want RUNNING", dispatch.Status)
	}

	interruptResult, err := service.InterruptDispatch(context.Background(), started.SessionID, fse.InterruptDispatchRequest{
		ControlRequest: fse.ControlRequest{Reason: "operator stop"},
		DispatchID:     "dispatch-1",
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}
	if interruptResult.Outcome != fse.LifecycleControlOutcomeAccepted {
		t.Fatalf("interrupt outcome = %q, want ACCEPTED", interruptResult.Outcome)
	}

	waitForInferContextHonored(t, provider, 2*time.Second)
	detail, err := service.GetDispatch(context.Background(), started.SessionID, "dispatch-1")
	if err != nil {
		t.Fatalf("GetDispatch after interrupt: %v", err)
	}
	if detail.Status != fse.DispatchStatusInterrupted {
		t.Fatalf("dispatch status after interrupt = %q, want INTERRUPTED", detail.Status)
	}
	if detail.FailureDetail == nil || detail.FailureDetail.Message != "operator stop" {
		t.Fatalf("failureDetail = %#v, want operator stop", detail.FailureDetail)
	}
}

type blockingFixtureProvider struct {
	mu              sync.Mutex
	inferStarted    chan struct{}
	inferStartedSet bool
	contextCanceled int
}

func newBlockingFixtureProvider() *blockingFixtureProvider {
	return &blockingFixtureProvider{
		inferStarted: make(chan struct{}),
	}
}

func (m *blockingFixtureProvider) Infer(ctx context.Context, _ workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	m.mu.Lock()
	if !m.inferStartedSet {
		m.inferStartedSet = true
		close(m.inferStarted)
	}
	m.mu.Unlock()

	<-ctx.Done()
	m.mu.Lock()
	m.contextCanceled++
	m.mu.Unlock()
	return workerexecution.InferenceResponse{}, ctx.Err()
}

func (m *blockingFixtureProvider) inferContextsHonored() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.contextCanceled
}

func (m *blockingFixtureProvider) waitForInferStart(t *testing.T) {
	t.Helper()
	select {
	case <-m.inferStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("provider Infer did not start before timeout assertion")
	}
}

func waitForInferContextHonored(t *testing.T, provider *blockingFixtureProvider, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if provider.inferContextsHonored() > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("provider Infer did not observe timed-out workflow context")
}

func waitForListedDispatch(
	t *testing.T,
	service *fse.JavaScriptRuntimeService,
	sessionID string,
	dispatchID string,
	timeout time.Duration,
) fse.DispatchSummary {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed, err := service.ListDispatches(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("ListDispatches: %v", err)
		}
		for _, dispatch := range listed.Dispatches {
			if dispatch.ID == dispatchID {
				return dispatch
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("dispatch %q did not appear in ListDispatches before timeout", dispatchID)
	return fse.DispatchSummary{}
}

func TestJavaScriptRuntimeService_ParallelLiveChildFailure_ProjectsTypedFailureAndPreservesSiblings(t *testing.T) {
	provider := newParallelLiveChildMockProvider()
	projectRoot := setupRuntimeWorkflowFixture(t, "parallel-live-child-failure.workflow.js", "parallel-live-child-failure")
	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
	})

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-parallel-live-child-failure",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "parallel-live-child-failure",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &fse.RuntimeOptions{
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	assertParallelLiveChildFailureInspection(t, service, completed)
}

// pkgmaintcheck:ignore-cyclomatic-complexity this runtime fixture keeps terminal provider failure inspection in one end-to-end scenario.
func TestJavaScriptRuntimeService_AgentRunLiveChildFailure_ProjectsFailedDispatchOnWorkflowFailure(t *testing.T) {
	provider := newFailingFixtureMockProvider(provider.NewProviderError(
		workerexecution.WorkFailureTypePermanentBadRequest,
		"simulated live child error",
		nil,
	))
	projectRoot := setupRuntimeWorkflowFixture(t, "agent-run-live-child-failure.workflow.js", "agent-run-live-child-failure")
	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
	})

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-agent-run-live-child-failure",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-live-child-failure",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &fse.RuntimeOptions{
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	read, err := service.GetSession(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Status != fse.LifecycleStatusFailed {
		t.Fatalf("session status = %q, want FAILED", read.Status)
	}
	if read.Failure == nil || read.Failure.Reason == "" {
		t.Fatalf("session failure = %#v, want typed workflow failure", read.Failure)
	}
	if read.Progress == nil || read.Progress.TotalDispatches != 1 || read.Progress.FailedDispatches != 1 {
		t.Fatalf("progress = %#v, want one failed dispatch", read.Progress)
	}

	dispatchDetail, err := service.GetDispatch(context.Background(), completed.SessionID, "dispatch-1")
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatchDetail.Status != fse.DispatchStatusFailed {
		t.Fatalf("dispatch status = %q, want FAILED", dispatchDetail.Status)
	}
	if dispatchDetail.FailureDetail == nil || dispatchDetail.FailureDetail.Reason != string(workerexecution.WorkFailureTypePermanentBadRequest) {
		t.Fatalf("dispatch failureDetail = %#v", dispatchDetail.FailureDetail)
	}
	if dispatchDetail.FailureDetail.Message != "Provider rejected the request as invalid." {
		t.Fatalf("dispatch failure message = %q", dispatchDetail.FailureDetail.Message)
	}
	if dispatchDetail.Attempt != 1 || dispatchDetail.Retryable == nil || *dispatchDetail.Retryable || dispatchDetail.FailureClassification != string(workerexecution.WorkFailureTypePermanentBadRequest) {
		t.Fatalf("dispatch retry diagnostics = %#v", dispatchDetail.DispatchSummary)
	}
	if provider.inferCallCount != 1 {
		t.Fatalf("provider infer call count = %d, want 1", provider.inferCallCount)
	}
}

type failingFixtureMockProvider struct {
	err            error
	inferCallCount int
}

func newFailingFixtureMockProvider(err error) *failingFixtureMockProvider {
	return &failingFixtureMockProvider{err: err}
}

func (m *failingFixtureMockProvider) Infer(_ context.Context, _ workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	m.inferCallCount++
	return workerexecution.InferenceResponse{}, m.err
}

type parallelLiveChildMockProvider struct {
	callCount int
}

func newParallelLiveChildMockProvider() *parallelLiveChildMockProvider {
	return &parallelLiveChildMockProvider{}
}

func (m *parallelLiveChildMockProvider) Infer(_ context.Context, req workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	m.callCount++
	if strings.Contains(req.UserMessage, "force provider failure") {
		return workerexecution.InferenceResponse{}, provider.NewProviderError(
			workerexecution.WorkFailureTypePermanentBadRequest,
			"simulated child error",
			nil,
		)
	}
	return workerexecution.InferenceResponse{
		Content: `{"text":"live:` + req.Dispatch.DispatchID + `:` + req.UserMessage + `"}`,
		ProviderSession: &workerexecution.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "live-provider-" + req.Dispatch.DispatchID,
		},
	}, nil
}

func assertLiveChildDispatchInspection(
	t *testing.T,
	service fse.Service,
	completed fse.SyncStartResult,
	providerCallCount int,
) {
	t.Helper()

	read, dispatch, dispatchDetail := loadLiveChildDispatchReads(t, service, completed)
	assertLiveChildDispatchSummary(t, read, dispatch, providerCallCount)
	assertLiveChildDispatchDetail(t, dispatchDetail)
	assertLiveChildPrimaryChildResult(t, service, completed)
	assertLiveChildArtifacts(t, service, completed, read, dispatch, dispatchDetail)
}

func loadLiveChildDispatchReads(
	t *testing.T,
	service fse.Service,
	completed fse.SyncStartResult,
) (fse.SessionReadResult, fse.DispatchSummary, fse.DispatchDetail) {
	t.Helper()

	read, err := service.GetSession(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	dispatches, err := service.ListDispatches(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one dispatch", dispatches.Dispatches)
	}
	dispatch := dispatches.Dispatches[0]
	dispatchDetail, err := service.GetDispatch(context.Background(), completed.SessionID, dispatch.ID)
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	return read, dispatch, dispatchDetail
}

func assertLiveChildDispatchSummary(
	t *testing.T,
	read fse.SessionReadResult,
	dispatch fse.DispatchSummary,
	providerCallCount int,
) {
	t.Helper()

	if read.Progress == nil || read.Progress.TotalDispatches != 1 || read.Progress.CompletedDispatches != 1 {
		t.Fatalf("progress = %#v, want one completed dispatch", read.Progress)
	}
	if dispatch.ID != "dispatch-1" {
		t.Fatalf("dispatch id = %q, want dispatch-1", dispatch.ID)
	}
	if dispatch.Status != fse.DispatchStatusCompleted {
		t.Fatalf("dispatch status = %q, want COMPLETED", dispatch.Status)
	}
	if dispatch.Attempt != 1 {
		t.Fatalf("dispatch attempt = %d, want 1", dispatch.Attempt)
	}
	if dispatch.Provider != "mock" {
		t.Fatalf("dispatch provider = %q, want mock", dispatch.Provider)
	}
	if len(dispatch.ProviderSessionRefs) != 1 || dispatch.ProviderSessionRefs[0] != (fse.ProviderSessionRef{
		Provider: "mock", Kind: "session_id", ID: "live-provider-session-1",
	}) {
		t.Fatalf("providerSessionRefs = %#v", dispatch.ProviderSessionRefs)
	}
	if dispatch.Model != "gpt-test" {
		t.Fatalf("dispatch model = %q, want gpt-test", dispatch.Model)
	}
	if providerCallCount != 1 {
		t.Fatalf("provider call count = %d, want 1", providerCallCount)
	}
	if len(dispatch.OutputArtifactIDs) != 1 || dispatch.OutputArtifactIDs[0] != "child-artifact-1" {
		t.Fatalf("outputArtifactIds = %#v, want [child-artifact-1]", dispatch.OutputArtifactIDs)
	}
}

func assertLiveChildDispatchDetail(t *testing.T, dispatchDetail fse.DispatchDetail) {
	t.Helper()

	if dispatchDetail.JavaScript == nil {
		t.Fatalf("dispatch javascript projection = nil, want execution mode")
	}
	if dispatchDetail.JavaScript.ExecutionMode != workflowruntime.ChildExecutionModeLive {
		t.Fatalf("executionMode = %q, want %q", dispatchDetail.JavaScript.ExecutionMode, workflowruntime.ChildExecutionModeLive)
	}
	assertDispatchStatusTransitions(t, dispatchDetail.StatusTransitions, []fse.DispatchStatus{
		fse.DispatchStatusQueued,
		fse.DispatchStatusRunning,
		fse.DispatchStatusCompleted,
	})
	if len(dispatchDetail.ArtifactIDs) != 1 || dispatchDetail.ArtifactIDs[0] != "child-artifact-1" {
		t.Fatalf("dispatch artifactIds = %#v, want [child-artifact-1]", dispatchDetail.ArtifactIDs)
	}
}

func assertLiveChildPrimaryChildResult(t *testing.T, service fse.Service, completed fse.SyncStartResult) {
	t.Helper()

	result, err := service.GetResult(context.Background(), completed.SessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	primary := decodePrimaryResultMap(t, result.PrimaryResult)
	child, ok := primary["child"].(map[string]any)
	if !ok {
		t.Fatalf("primary child = %#v, want object", primary["child"])
	}
	if child["executionMode"] != workflowruntime.ChildExecutionModeLive {
		t.Fatalf("child executionMode = %#v, want %q", child["executionMode"], workflowruntime.ChildExecutionModeLive)
	}
	if child["dispatchId"] != "dispatch-1" {
		t.Fatalf("child dispatchId = %#v, want dispatch-1", child["dispatchId"])
	}
}

func assertLiveChildArtifacts(
	t *testing.T,
	service fse.Service,
	completed fse.SyncStartResult,
	read fse.SessionReadResult,
	dispatch fse.DispatchSummary,
	dispatchDetail fse.DispatchDetail,
) {
	t.Helper()
	_ = dispatchDetail

	artifacts, err := service.ListArtifacts(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(artifacts.Artifacts) != 1 {
		t.Fatalf("artifacts = %#v, want one child artifact", artifacts.Artifacts)
	}
	childArtifact := artifacts.Artifacts[0]
	if childArtifact.ID != "child-artifact-1" || childArtifact.DispatchID != dispatch.ID {
		t.Fatalf("child artifact = %#v", childArtifact)
	}
	wantHref := "/factory-sessions/" + completed.SessionID + "/artifacts/child-artifact-1"
	if childArtifact.RetrievalRef == nil || childArtifact.RetrievalRef.Href != wantHref {
		t.Fatalf("child artifact retrieval = %#v, want %q", childArtifact.RetrievalRef, wantHref)
	}
	if read.ArtifactCount != 1 || len(read.ArtifactRefs) != 1 || read.ArtifactRefs[0].ID != "child-artifact-1" {
		t.Fatalf("session artifact refs = count %d refs %#v", read.ArtifactCount, read.ArtifactRefs)
	}
}

func assertParallelLiveChildFailureInspection(
	t *testing.T,
	service fse.Service,
	completed fse.SyncStartResult,
) {
	t.Helper()

	read, dispatchByID := loadParallelFailureDispatchMap(t, service, completed)
	assertParallelFailureSessionProgress(t, read)
	assertParallelFailureSiblingDispatches(t, dispatchByID)
	assertParallelFailureFailedDispatch(t, service, completed, dispatchByID["dispatch-2"])
	assertParallelFailurePrimaryResult(t, service, completed, dispatchByID["dispatch-2"].ID)
}

func loadParallelFailureDispatchMap(
	t *testing.T,
	service fse.Service,
	completed fse.SyncStartResult,
) (fse.SessionReadResult, map[string]fse.DispatchSummary) {
	t.Helper()

	read, err := service.GetSession(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	dispatches, err := service.ListDispatches(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 3 {
		t.Fatalf("dispatches = %#v, want three dispatches", dispatches.Dispatches)
	}
	dispatchByID := map[string]fse.DispatchSummary{}
	for _, dispatch := range dispatches.Dispatches {
		dispatchByID[dispatch.ID] = dispatch
	}
	for _, dispatchID := range []string{"dispatch-1", "dispatch-2", "dispatch-3"} {
		if _, ok := dispatchByID[dispatchID]; !ok {
			t.Fatalf("missing dispatch %q in %#v", dispatchID, dispatches.Dispatches)
		}
	}
	return read, dispatchByID
}

func assertParallelFailureSessionProgress(t *testing.T, read fse.SessionReadResult) {
	t.Helper()

	if read.Status != fse.LifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", read.Status)
	}
	if read.Progress == nil || read.Progress.TotalDispatches != 3 || read.Progress.CompletedDispatches != 2 || read.Progress.FailedDispatches != 1 {
		t.Fatalf("progress = %#v, want two completed and one failed dispatch", read.Progress)
	}
}

func assertParallelFailureSiblingDispatches(t *testing.T, dispatchByID map[string]fse.DispatchSummary) {
	t.Helper()

	successOne := dispatchByID["dispatch-1"]
	successThree := dispatchByID["dispatch-3"]
	if successOne.Status != fse.DispatchStatusCompleted || successThree.Status != fse.DispatchStatusCompleted {
		t.Fatalf("sibling dispatches = %#v/%#v, want COMPLETED", successOne, successThree)
	}
	if len(successOne.OutputArtifactIDs) == 0 || len(successThree.OutputArtifactIDs) == 0 {
		t.Fatalf("sibling output artifacts = %#v/%#v, want one artifact each", successOne.OutputArtifactIDs, successThree.OutputArtifactIDs)
	}
	if len(successOne.ProviderSessionRefs) != 1 || len(successThree.ProviderSessionRefs) != 1 {
		t.Fatalf("sibling provider refs = %#v/%#v", successOne.ProviderSessionRefs, successThree.ProviderSessionRefs)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this assertion verifies the complete failed-dispatch public projection.
func assertParallelFailureFailedDispatch(
	t *testing.T,
	service fse.Service,
	completed fse.SyncStartResult,
	failed fse.DispatchSummary,
) {
	t.Helper()

	if failed.Status != fse.DispatchStatusFailed {
		t.Fatalf("failed dispatch status = %q, want FAILED", failed.Status)
	}
	if failed.FailureDetail == nil {
		t.Fatal("failed dispatch missing failureDetail")
	}
	if failed.FailureDetail.Reason != string(workerexecution.WorkFailureTypePermanentBadRequest) {
		t.Fatalf("failure reason = %q, want %q", failed.FailureDetail.Reason, workerexecution.WorkFailureTypePermanentBadRequest)
	}
	if failed.FailureDetail.Message != "Provider rejected the request as invalid." {
		t.Fatalf("failure message = %q, want sanitized provider detail", failed.FailureDetail.Message)
	}
	if failed.Attempt != 1 || failed.Retryable == nil || *failed.Retryable || failed.FailureClassification != string(workerexecution.WorkFailureTypePermanentBadRequest) {
		t.Fatalf("retry diagnostics = %#v", failed)
	}
	if len(failed.OutputArtifactIDs) != 0 {
		t.Fatalf("failed outputArtifactIds = %#v, want none", failed.OutputArtifactIDs)
	}

	failedDetail, err := service.GetDispatch(context.Background(), completed.SessionID, failed.ID)
	if err != nil {
		t.Fatalf("GetDispatch failed child: %v", err)
	}
	if failedDetail.JavaScript == nil || failedDetail.JavaScript.ExecutionMode != workflowruntime.ChildExecutionModeLive {
		t.Fatalf("failed javascript projection = %#v, want live-provider", failedDetail.JavaScript)
	}
	assertDispatchStatusTransitions(t, failedDetail.StatusTransitions, []fse.DispatchStatus{
		fse.DispatchStatusQueued,
		fse.DispatchStatusRunning,
		fse.DispatchStatusFailed,
	})
	if failedDetail.FailureDetail == nil || failedDetail.FailureDetail.Reason != string(workerexecution.WorkFailureTypePermanentBadRequest) {
		t.Fatalf("failed dispatch detail failure = %#v", failedDetail.FailureDetail)
	}

	successDetail, err := service.GetDispatch(context.Background(), completed.SessionID, "dispatch-1")
	if err != nil {
		t.Fatalf("GetDispatch success child: %v", err)
	}
	if successDetail.FailureDetail != nil {
		t.Fatalf("success dispatch failureDetail = %#v, want nil", successDetail.FailureDetail)
	}
	if len(successDetail.ArtifactIDs) != 1 {
		t.Fatalf("success dispatch artifactIds = %#v, want one artifact", successDetail.ArtifactIDs)
	}
}

func assertParallelFailurePrimaryResult(t *testing.T, service fse.Service, completed fse.SyncStartResult, failedDispatchID string) {
	t.Helper()

	result, err := service.GetResult(context.Background(), completed.SessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusFinal || result.SessionStatus != fse.LifecycleStatusSucceeded {
		t.Fatalf("result = status %q session %q, want FINAL/SUCCEEDED", result.ResultStatus, result.SessionStatus)
	}
	primary := decodePrimaryResultMap(t, result.PrimaryResult)
	results, ok := primary["results"].([]any)
	if !ok || len(results) != 3 {
		t.Fatalf("primary results = %#v, want three child entries", primary["results"])
	}
	failedChild, ok := results[1].(map[string]any)
	if !ok || failedChild["status"] != workflowruntime.ChildDispatchStatusFailed {
		t.Fatalf("primary failed child = %#v", results[1])
	}

	artifacts, err := service.ListArtifacts(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	for _, artifact := range artifacts.Artifacts {
		if artifact.DispatchID == failedDispatchID {
			t.Fatalf("failed dispatch produced artifact = %#v", artifact)
		}
	}
}

func assertDispatchStatusTransitions(t *testing.T, got []fse.DispatchStatus, want []fse.DispatchStatus) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("statusTransitions = %#v, want %#v", got, want)
	}
	for index, status := range got {
		if status != want[index] {
			t.Fatalf("statusTransitions[%d] = %q, want %q", index, status, want[index])
		}
	}
}

func TestJavaScriptRuntimeService_ChildExecutorModes_CoexistOnSameWorkflowSource(t *testing.T) {
	projectRoot := setupRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	provider := newFixtureMockProvider(workerexecution.InferenceResponse{
		Content: `{"text":"live child output"}`,
		ProviderSession: &workerexecution.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "live-provider-session-1",
		},
	})

	fakeService := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	liveService := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
	})

	fakeCompleted := startAgentRunFakeChild(t, fakeService, "req-runtime-child-mode-fake")
	liveCompleted := startAgentRunFakeChild(t, liveService, "req-runtime-child-mode-live")

	fakeDetail := dispatchExecutionMode(t, fakeService, fakeCompleted.SessionID, "dispatch-1")
	liveDetail := dispatchExecutionMode(t, liveService, liveCompleted.SessionID, "dispatch-1")

	if fakeDetail != workflowruntime.ChildExecutionModeFake {
		t.Fatalf("fake dispatch executionMode = %q, want %q", fakeDetail, workflowruntime.ChildExecutionModeFake)
	}
	if liveDetail != workflowruntime.ChildExecutionModeLive {
		t.Fatalf("live dispatch executionMode = %q, want %q", liveDetail, workflowruntime.ChildExecutionModeLive)
	}
	if provider.callCount != 1 {
		t.Fatalf("provider call count = %d, want 1 (live path only)", provider.callCount)
	}
}

func TestJavaScriptRuntimeService_ExplicitFakeMode_OverridesLiveServiceConfig(t *testing.T) {
	projectRoot := setupRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	provider := newFixtureMockProvider(workerexecution.InferenceResponse{
		Content: `{"text":"unused live output"}`,
	})
	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
	})

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-child-mode-explicit-fake",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &fse.RuntimeOptions{
			ChildExecutorMode: fse.ChildExecutorModeFake,
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	mode := dispatchExecutionMode(t, service, completed.SessionID, "dispatch-1")
	if mode != workflowruntime.ChildExecutionModeFake {
		t.Fatalf("dispatch executionMode = %q, want %q", mode, workflowruntime.ChildExecutionModeFake)
	}
	if provider.callCount != 0 {
		t.Fatalf("provider call count = %d, want 0 for explicit fake override", provider.callCount)
	}
}

func TestJavaScriptRuntimeService_ParallelFakeChildren_RemainsDeterministicWithoutProvider(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "parallel-fake-children.workflow.js", "parallel-fake-children")

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-parallel-fake-children",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "parallel-fake-children",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		RequestedPolicy: map[string]any{
			"maxAgents":   8,
			"concurrency": 2,
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	dispatches, err := service.ListDispatches(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 4 {
		t.Fatalf("dispatches = %#v, want four fake children", dispatches.Dispatches)
	}
	for _, dispatch := range dispatches.Dispatches {
		detail, err := service.GetDispatch(context.Background(), completed.SessionID, dispatch.ID)
		if err != nil {
			t.Fatalf("GetDispatch(%s): %v", dispatch.ID, err)
		}
		if detail.JavaScript == nil || detail.JavaScript.ExecutionMode != workflowruntime.ChildExecutionModeFake {
			t.Fatalf("dispatch %s javascript = %#v, want fake execution mode", dispatch.ID, detail.JavaScript)
		}
	}

	result, err := service.GetResult(context.Background(), completed.SessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	primary := decodePrimaryResultMap(t, result.PrimaryResult)
	results, ok := primary["results"].([]any)
	if !ok || len(results) != 4 {
		t.Fatalf("primary results = %#v, want four child results", primary["results"])
	}
	for index, entry := range results {
		child, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("results[%d] = %#v, want object", index, entry)
		}
		if child["executionMode"] != workflowruntime.ChildExecutionModeFake {
			t.Fatalf("results[%d].executionMode = %#v, want fake", index, child["executionMode"])
		}
	}
}

func TestJavaScriptRuntimeService_PipelineFakeChildren_RemainsDeterministicWithoutProvider(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "pipeline-staged-fake-children.workflow.js", "pipeline-staged-fake-children")

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-pipeline-fake-children",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "pipeline-staged-fake-children",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	dispatches, err := service.ListDispatches(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 6 {
		t.Fatalf("dispatches = %#v, want six staged fake children", dispatches.Dispatches)
	}
	for _, dispatch := range dispatches.Dispatches {
		detail, err := service.GetDispatch(context.Background(), completed.SessionID, dispatch.ID)
		if err != nil {
			t.Fatalf("GetDispatch(%s): %v", dispatch.ID, err)
		}
		if detail.JavaScript == nil || detail.JavaScript.ExecutionMode != workflowruntime.ChildExecutionModeFake {
			t.Fatalf("dispatch %s javascript = %#v, want fake execution mode", dispatch.ID, detail.JavaScript)
		}
	}
}

func startAgentRunFakeChild(t *testing.T, service fse.Service, requestID string) fse.SyncStartResult {
	t.Helper()
	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: requestID,
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
	})
	if err != nil {
		t.Fatalf("StartSync(%s): %v", requestID, err)
	}
	return completed
}

func dispatchExecutionMode(t *testing.T, service fse.Service, sessionID, dispatchID string) string {
	t.Helper()
	detail, err := service.GetDispatch(context.Background(), sessionID, dispatchID)
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if detail.JavaScript == nil {
		t.Fatal("dispatch javascript projection is nil")
	}
	return detail.JavaScript.ExecutionMode
}
