package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/internal/testutil/checkpointfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/factoryruntimefixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livechange"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestChildWorkerExecutor_CompletedChildRecordsItsWorkerAndOutput pins the
// record a customer reads for a child that ran: its provider, its provider
// session, its decoded output, and the attempt count the Worker actually took.
func TestChildWorkerExecutor_CompletedChildRecordsItsWorkerAndOutput(t *testing.T) {
	invoker := &recordingWorkerExecution{}
	attempts := 0
	invoker.onExecute = func(_ workers.ExecuteRequest) {
		attempts++
		if attempts == 1 {
			invoker.result = workers.ExecuteResult{
				Outcome: workers.ExecutionOutcomeFailed,
				Failure: &workers.ExecutionFailure{
					Type:      workers.WorkFailureTypeInternalServerError,
					Message:   "transient provider failure",
					RetryHint: true,
				},
			}
			return
		}
		invoker.result = workers.ExecuteResult{
			Outcome: workers.ExecutionOutcomeAccepted,
			StructuredResult: map[string]any{
				"text": "child finished",
			},
			StructuredResultPresent: true,
			Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: `{"text":"child finished"}`,
			}}},
			Continuation: &workers.ProviderContinuationRef{
				Provider:          "codex",
				ProviderSessionID: "codex-session-1",
			},
		}
	}
	sink := newChildRecordSink()
	executor := newChildWorkerExecutor("dur-sess-1", invoker, sink, childTestValues{}, nil, "/project", 1)

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
		Prompt:        "summarize",
		Label:         "summarize-findings",
		ModelProvider: "codex",
		OutputSchema:  map[string]any{"type": "object"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("child status = %q, want COMPLETED", result.Status)
	}
	if got := result.Output["text"]; got != "child finished" {
		t.Fatalf("child output text = %v, want the provider's decoded content", got)
	}
	if !result.SchemaValidated {
		t.Fatal("child schemaValidated = false, want true for the accepted schema-declared result")
	}

	terminal := sink.terminalChildDispatch(t)
	if terminal.Provider != "codex" || terminal.ProviderSessionRef != "codex-session-1" {
		t.Fatalf("terminal record provider = %q/%q, want codex/codex-session-1", terminal.Provider, terminal.ProviderSessionRef)
	}
	if attempts != 2 || terminal.Attempt != 2 {
		t.Fatalf("attempts = %d, terminal record attempt = %d, want two attempts and terminal attempt 2", attempts, terminal.Attempt)
	}
	if !terminal.SchemaValidated {
		t.Fatal("terminal schemaValidated = false, want true")
	}
	if len(sink.statuses) != 2 || sink.statuses[0] != factory.JavaScriptChildDispatchStatusQueued || sink.statuses[1] != factory.JavaScriptChildDispatchStatusRunning {
		t.Fatalf("dispatch statuses = %v, want QUEUED then RUNNING before the terminal record", sink.statuses)
	}
}

func TestChildWorkerExecutor_StructuredSchemaMismatchConsumesChildRetryAllowance(t *testing.T) {
	attempts := 0
	invoker := &recordingWorkerExecution{
		result: workers.ExecuteResult{
			Outcome: workers.ExecutionOutcomeFailed,
			Failure: &workers.ExecutionFailure{
				Type:    workers.WorkFailureTypeStructuredOutputSchemaViolation,
				Family:  workers.WorkFailureFamilyTerminal,
				Message: "structured output schema violation: instance /answer; expected string",
				Detail: &workers.FailureDetail{
					Reason:  workers.WorkFailureTypeStructuredOutputSchemaViolation,
					Message: "structured output schema violation: instance /answer; expected string",
				},
			},
		},
	}
	invoker.onExecute = func(_ workers.ExecuteRequest) {
		attempts++
		if attempts == 2 {
			invoker.result = workers.ExecuteResult{
				Outcome: workers.ExecutionOutcomeAccepted,
				StructuredResult: map[string]any{
					"answer":          "validated answer",
					"schemaValidated": "customer-owned value",
				},
				StructuredResultPresent: true,
				// The raw provider text is deliberately different: the child
				// must return Workers' validated native result, not reparse it.
				Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeText,
					Text: `{"answer":"rejected raw text"}`,
				}}},
			}
		}
	}
	sink := newChildRecordSink()
	executor := newChildWorkerExecutor("dur-sess-structured-retry", invoker, sink, childTestValues{}, nil, "/project", 1)

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
		Prompt:        "return a structured answer",
		Label:         "structured-retry",
		ModelProvider: "codex",
		OutputSchema:  map[string]any{"type": "object"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("Workers attempts = %d, want initial attempt plus one child retry", attempts)
	}
	if result.Status != factory.JavaScriptChildDispatchStatusCompleted || !result.SchemaValidated {
		t.Fatalf("child result = %#v, want completed schema-validated result", result)
	}
	if result.Output["answer"] != "validated answer" {
		t.Fatalf("child answer = %#v, want native structured answer", result.Output["answer"])
	}
	if result.Output["schemaValidated"] != "customer-owned value" {
		t.Fatalf("customer schemaValidated field = %#v, want preserved customer value", result.Output["schemaValidated"])
	}
	terminal := sink.terminalChildDispatch(t)
	if terminal.Attempt != 2 || !terminal.SchemaValidated {
		t.Fatalf("terminal record = %#v, want attempt 2 and schemaValidated true", terminal)
	}
	if terminal.Output["schemaValidated"] != "customer-owned value" {
		t.Fatalf("recorded customer schemaValidated field = %#v, want preserved customer value", terminal.Output["schemaValidated"])
	}
	if invoker.request.Target.Prompt.OutputSchema == "" {
		t.Fatal("Workers request output schema = empty, want schema-declared child contract")
	}
}

func TestChildWorkerExecutor_ExhaustedStructuredSchemaMismatchFailsSafely(t *testing.T) {
	const diagnostic = "structured output schema violation: instance /answer; expected string"
	attempts := 0
	invoker := &recordingWorkerExecution{
		result: workers.ExecuteResult{
			Outcome: workers.ExecutionOutcomeFailed,
			Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: `{"answer":"rejected-value"}`,
			}}},
			Failure: &workers.ExecutionFailure{
				Type:      workers.WorkFailureTypeStructuredOutputSchemaViolation,
				Family:    workers.WorkFailureFamilyTerminal,
				Message:   diagnostic,
				RetryHint: false,
				Detail: &workers.FailureDetail{
					Reason:  workers.WorkFailureTypeStructuredOutputSchemaViolation,
					Message: diagnostic,
				},
			},
		},
	}
	invoker.onExecute = func(_ workers.ExecuteRequest) { attempts++ }
	sink := newChildRecordSink()
	executor := newChildWorkerExecutor("dur-sess-structured-exhausted", invoker, sink, childTestValues{}, nil, "/project", 2)

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
		Prompt:        "do not expose rejected output",
		Label:         "structured-exhausted",
		ModelProvider: "codex",
		OutputSchema:  map[string]any{"type": "object"},
	})
	if err == nil || !strings.Contains(err.Error(), "/answer") {
		t.Fatalf("Execute error = %v, want schema path diagnostic", err)
	}
	if strings.Contains(err.Error(), "rejected-value") || strings.Contains(err.Error(), "do not expose rejected output") {
		t.Fatalf("Execute error = %q, must not expose rejected output or prompt", err)
	}
	if attempts != 3 {
		t.Fatalf("Workers attempts = %d, want initial attempt plus two child retries", attempts)
	}
	if result.Status != factory.JavaScriptChildDispatchStatusFailed || len(result.Output) != 0 || result.SchemaValidated {
		t.Fatalf("failed child result = %#v, want failed with no output and false schema metadata", result)
	}
	terminal := sink.terminalChildDispatch(t)
	if terminal.Attempt != 3 || terminal.Output != nil || terminal.SchemaValidated {
		t.Fatalf("failed terminal record = %#v, want attempt 3 with no output and false schema metadata", terminal)
	}
	if terminal.FailureClassification != workers.WorkFailureTypeStructuredOutputSchemaViolation {
		t.Fatalf("failure classification = %q, want structured schema violation", terminal.FailureClassification)
	}
	if terminal.Retryable == nil || *terminal.Retryable {
		t.Fatalf("retryable = %#v, want false after exhausted child allowance", terminal.Retryable)
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

func TestNewJavaScriptRuntimeService_RetainsInjectedLiveChangeCoordinator(t *testing.T) {
	coordinator := livechange.NewCoordinator()
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot:           t.TempDir(),
		LiveChangeCoordinator: coordinator,
	})
	if service == nil {
		t.Fatal("NewJavaScriptRuntimeService returned nil")
	}
	if service.liveChangeCoordinator != coordinator {
		t.Fatalf("live-change coordinator = %p, want injected %p", service.liveChangeCoordinator, coordinator)
	}
}

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

type javaScriptRuntimeServiceConfig struct {
	ProjectRoot           string
	ChildExecutorMode     string
	InvocationExecutor    workers.InvocationExecutor
	Persistence           runtimepersist.Store
	Clock                 factory.Clock
	Workflows             factory.JavaScriptWorkflows
	LiveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator
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
		workflows, orchestrationJavaScriptFromWorkflows(workflows), workflows,
		nil, factory.JavaScriptWorkerSettings{}, mustTestRecordingWriter(),
		testSessionIDGenerator,
		nil, nil, config.LiveChangeCoordinator,
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

type orchestrationJavaScriptAdapter struct {
	factory.JavaScriptWorkflowRuntime
}

func orchestrationJavaScriptFromWorkflows(workflows factory.JavaScriptWorkflows) factory.OrchestrationJavaScriptExecution {
	if workflows == nil {
		return nil
	}
	return orchestrationJavaScriptAdapter{workflows}
}

func (a orchestrationJavaScriptAdapter) RunJavaScript(
	ctx context.Context,
	req factory.JavaScriptRuntimeRequest,
	hooks factory.JavaScriptRuntimeHooks,
) (factory.JavaScriptRuntimeOutcome, error) {
	return a.Run(ctx, req, hooks)
}

func (a orchestrationJavaScriptAdapter) ResumeJavaScript(
	summary factory.JavaScriptCompletedCheckpointSummary,
	records []factory.JavaScriptRuntimeRecord,
) factory.JavaScriptResumeContext {
	return a.ResumeContext(summary, records)
}

func newTerminalWorkersService(t *testing.T, provider providers.Service) WorkerExecution {
	t.Helper()
	return terminalWorkerService{provider: provider}
}

// terminalWorkerService is a service-root fake: the bridge test owns durable
// response publication, while Workers-owned wire tests cover construction and
// normalization of the real Execute implementation.
type terminalWorkerService struct {
	provider providers.Service
}

func (service terminalWorkerService) Execute(
	ctx context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	providerResult, err := service.provider.Execute(ctx, providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: request.Correlation.AttemptID,
		Correlation: providers.ExecuteCorrelation{
			FactorySessionID: request.Correlation.FactorySessionID,
			RuntimeID:        request.Correlation.RuntimeID,
			GenerationID:     request.Correlation.GenerationID,
			DispatchID:       request.Correlation.DispatchID,
			AttemptID:        request.Correlation.AttemptID,
			RequestID:        request.Correlation.RequestID,
			TraceID:          request.Correlation.TraceID,
		},
		UserMessage: request.Target.Prompt.UserMessage,
	})
	result := workers.ExecuteResult{Correlation: request.Correlation}
	if err != nil {
		outcome := workers.ExecutionOutcomeFailed
		failureType := workers.WorkFailureTypeUnknown
		if errors.Is(err, context.Canceled) {
			outcome = workers.ExecutionOutcomeCanceled
		}
		if errors.Is(err, context.DeadlineExceeded) {
			failureType = workers.WorkFailureTypeTimeout
		}
		result.Outcome = outcome
		result.Failure = &workers.ExecutionFailure{
			Type:    failureType,
			Family:  workers.WorkFailureFamilyTerminal,
			Message: err.Error(),
		}
		return result, err
	}
	result.Outcome = workers.ExecutionOutcomeAccepted
	result.Output.Primary = []work.WorkContentPart{{Text: providerResult.Content}}
	return result, nil
}

func TestPersistedTokenFailureHistoryRetainsHeadTailAndReloads(t *testing.T) {
	const historySize = defaultPersistedTokenFailureLogCapacity + 8
	history := failureHistoryForRetryCount(historySize)
	state := runtimeSessionState{
		petriMutations: []interfaces.TokenMutationRecord{failureMutation(history, 1)},
	}

	snapshot := persistedSnapshotFromRuntimeState(state)
	if len(state.petriMutations[0].Token.History.FailureLog) != historySize {
		t.Fatalf("live failure log length = %d, want %d", len(state.petriMutations[0].Token.History.FailureLog), historySize)
	}
	if got := state.petriMutations[0].Token.History.FailureLogDroppedCount; got != 0 {
		t.Fatalf("live dropped failure count = %d, want 0", got)
	}

	got := snapshot.Records[0].PetriMutation.Token.History
	if got.FailureLogDroppedCount != 8 {
		t.Fatalf("persisted dropped failure count = %d, want 8", got.FailureLogDroppedCount)
	}
	if len(got.FailureLog) != defaultPersistedTokenFailureLogCapacity {
		t.Fatalf("persisted failure log length = %d, want %d", len(got.FailureLog), defaultPersistedTokenFailureLogCapacity)
	}
	for index := range got.FailureLog {
		wantIndex := index
		if index >= defaultPersistedTokenFailureLogCapacity/2 {
			wantIndex = historySize - (defaultPersistedTokenFailureLogCapacity - defaultPersistedTokenFailureLogCapacity/2) + index - defaultPersistedTokenFailureLogCapacity/2
		}
		want := history.FailureLog[wantIndex]
		if got.FailureLog[index] != want {
			t.Fatalf("persisted failure log[%d] = %#v, want %#v", index, got.FailureLog[index], want)
		}
	}
	if got.LastError != history.LastError {
		t.Fatalf("persisted LastError = %q, want %q", got.LastError, history.LastError)
	}

	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatalf("marshal bounded snapshot: %v", err)
	}
	var reloaded PersistedRuntimeSessionState
	if err := json.Unmarshal(encoded, &reloaded); err != nil {
		t.Fatalf("unmarshal bounded snapshot: %v", err)
	}
	reloadedHistory := reloaded.Records[0].PetriMutation.Token.History
	if reloadedHistory.FailureLogDroppedCount != got.FailureLogDroppedCount ||
		reloadedHistory.LastError != got.LastError ||
		len(reloadedHistory.FailureLog) != len(got.FailureLog) {
		t.Fatalf("reloaded failure history = %#v, want %#v", reloadedHistory, got)
	}
}

func TestPersistedTokenFailureHistoryWithinCapacityIsUnchanged(t *testing.T) {
	history := failureHistoryForRetryCount(defaultPersistedTokenFailureLogCapacity)
	state := runtimeSessionState{
		petriMutations: []interfaces.TokenMutationRecord{failureMutation(history, 1)},
	}

	snapshot := persistedSnapshotFromRuntimeState(state)
	got := snapshot.Records[0].PetriMutation.Token.History
	if len(got.FailureLog) != len(history.FailureLog) {
		t.Fatalf("failure log length = %d, want %d", len(got.FailureLog), len(history.FailureLog))
	}
	for index := range history.FailureLog {
		if got.FailureLog[index] != history.FailureLog[index] {
			t.Fatalf("failure log[%d] changed: got %#v, want %#v", index, got.FailureLog[index], history.FailureLog[index])
		}
	}
	if got.FailureLogDroppedCount != history.FailureLogDroppedCount || got.LastError != history.LastError {
		t.Fatalf("history metadata changed: got %#v, want %#v", got, history)
	}
}

func TestDurablePetriFailureHistorySnapshotGrowthIsBounded(t *testing.T) {
	retryCounts := []int{10, 100, 1000}
	baselineBytes := make(map[int]int, len(retryCounts))
	boundedBytes := make(map[int]int, len(retryCounts))

	for _, retryCount := range retryCounts {
		t.Run(fmt.Sprintf("N=%d", retryCount), func(t *testing.T) {
			store := &runtimeRecordingStore{}
			mutations := make([]interfaces.TokenMutationRecord, retryCount)
			for retry := 1; retry <= retryCount; retry++ {
				mutations[retry-1] = failureMutation(failureHistoryForRetryCount(retry), retry)
			}
			state := runtimeSessionState{
				session: SessionReadResult{
					SessionID: "~default",
					Status:    LifecycleStatusRunning,
				},
				petriMutations: mutations,
			}
			service := &JavaScriptRuntimeService{
				clock:       runtimeTestClock{now: time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)},
				persistence: store,
			}
			if err := service.persistSessionSnapshot(state); err != nil {
				t.Fatalf("persist retry sequence: %v", err)
			}

			live := cloneRuntimeSessionState(&state)
			last := live.petriMutations[len(live.petriMutations)-1].Token.History
			if len(last.FailureLog) != retryCount {
				t.Fatalf("live final failure log length = %d, want %d", len(last.FailureLog), retryCount)
			}
			if last.FailureLogDroppedCount != 0 {
				t.Fatalf("live final dropped failure count = %d, want 0", last.FailureLogDroppedCount)
			}

			unbounded := persistedSnapshotFromRuntimeStateWithFailureLogCapacity(live, 0)
			before, err := json.MarshalIndent(unbounded, "", "  ")
			if err != nil {
				t.Fatalf("marshal unbounded baseline: %v", err)
			}
			baselineBytes[retryCount] = len(before)
			boundedBytes[retryCount] = len(store.payload)
			t.Logf("N=%d before_bytes=%d after_bytes=%d", retryCount, len(before), len(store.payload))
		})
	}

	if boundedBytes[10] != baselineBytes[10] {
		t.Fatalf("N=10 changed despite fitting capacity: before=%d after=%d", baselineBytes[10], boundedBytes[10])
	}
	for _, retryCount := range []int{100, 1000} {
		if boundedBytes[retryCount] >= baselineBytes[retryCount] {
			t.Fatalf("N=%d bounded snapshot = %d, want less than unbounded baseline %d", retryCount, boundedBytes[retryCount], baselineBytes[retryCount])
		}
	}
	if boundedBytes[1000] > boundedBytes[100]*12 {
		t.Fatalf("bounded snapshots grew superlinearly: N=100=%d, N=1000=%d", boundedBytes[100], boundedBytes[1000])
	}
}

func failureMutation(history workers.History, retry int) interfaces.TokenMutationRecord {
	return interfaces.TokenMutationRecord{
		DispatchID:   fmt.Sprintf("dispatch-%04d", retry),
		TransitionID: "retry",
		Outcome:      workers.OutcomeFailed,
		Type:         interfaces.MutationMove,
		TokenID:      fmt.Sprintf("token-%04d", retry),
		FromPlace:    "task:running",
		ToPlace:      "task:failed",
		Reason:       "worker failed",
		Token: &workers.Token{
			ID:    fmt.Sprintf("token-%04d", retry),
			State: "failed",
			Color: workers.Color{
				WorkID:     "work-1",
				WorkTypeID: "task",
				DataType:   workers.DataTypeWork,
			},
			History: history,
		},
	}
}

func failureHistoryForRetryCount(count int) workers.History {
	log := make([]workers.Failure, count)
	for index := range log {
		log[index] = workers.Failure{
			TransitionID: "retry",
			Timestamp:    time.Date(2026, 8, 22, 12, 0, index, 0, time.UTC),
			Error:        fmt.Sprintf("failure-%04d", index+1),
			Attempt:      index + 1,
		}
	}
	lastError := ""
	if len(log) > 0 {
		lastError = log[len(log)-1].Error
	}
	return workers.History{
		TotalVisits:         map[string]int{"retry": count},
		ConsecutiveFailures: map[string]int{"retry": count},
		PlaceVisits:         map[string]int{"task:failed": count},
		LastError:           lastError,
		FailureLog:          log,
	}
}
