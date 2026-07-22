package fixtures_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factoryruntimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution/fixtures"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

const simpleFinalWorkflowSource = `// Simple final-only workflow fixture for runtime boundary tests.
return {
  label: meta.name,
  description: meta.description,
  subject: args.subject,
  repeat: args.count,
  echo: args.prefix + ":" + args.subject,
};
`

const busyLoopWorkflowSource = `// Busy loop fixture for async running/cancel/timeout tests.
var spin = 0;
while (true) {
  spin += 1;
}
`

const throwErrorWorkflowSource = `// Fixture that throws during workflow execution.
throw new Error("workflow execution failed: " + args.subject);
`

type fixtureClock struct{ now time.Time }

func (c fixtureClock) Now() time.Time { return c.now }

func TestJavaScriptRuntimeService_StartSync_SimpleWorkflowCompletesWithPrimaryResult(t *testing.T) {
	projectRoot := writeSimpleFinalWorkflowProject(t)
	service, err := newExecutionService(
		fse.ExecutionProviderJavaScriptRuntime,
		executionServiceConfig{
			ProjectRoot: projectRoot, Persistence: fse.DisabledPersistence(),
			Clock: fixtureClock{now: time.Now()}, Workflows: successfulFixtureWorkflows(map[string]any{
				"label": "simple-final", "description": "returns a structured final value",
				"subject": "workflows", "repeat": float64(3), "echo": "you:workflows",
			}),
		},
	)
	if err != nil {
		t.Fatalf("NewExecutionService: %v", err)
	}

	started, err := service.StartSync(context.Background(), simpleFinalSyncStartRequest())
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	assertSyncStartCompleted(t, started)
	assertSucceededSessionReads(t, service, started.SessionID)

	result, err := service.GetResult(context.Background(), started.SessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	assertSimpleFinalPrimaryResult(t, result)
}

func TestJavaScriptRuntimeService_StartSyncStreamsCanonicalPhaseBeforeCompletion(t *testing.T) {
	phasePublished := make(chan struct{})
	release := make(chan struct{})
	workflows := factoryruntimefixtures.ScriptedJavaScriptWorkflows{
		RunFunc: func(
			_ context.Context,
			_ factory.JavaScriptRuntimeRequest,
			hooks factory.JavaScriptRuntimeHooks,
		) (factory.JavaScriptRuntimeOutcome, error) {
			records := []factory.JavaScriptRuntimeRecord{{
				Sequence: 1, Kind: factory.JavaScriptRecordKindPhase,
				Phase: &factory.JavaScriptPhaseRecord{Name: "execute"},
			}}
			hooks.OnRecord(records[0])
			close(phasePublished)
			<-release
			records = append(records, factory.JavaScriptRuntimeRecord{
				Sequence: 2, Kind: factory.JavaScriptRecordKindCheckpoint,
				Checkpoint: &factory.JavaScriptCheckpointRecord{ID: "checkpoint-live", Label: "live-ready"},
			})
			hooks.OnRecord(records[1])
			value, err := json.Marshal(map[string]any{"status": "complete"})
			return factory.JavaScriptRuntimeOutcome{
				OK: true, Value: factory.TypedValue{JSON: value}, Records: records,
			}, err
		},
	}
	service := newJavaScriptRuntimeService(t, workflows)
	request := simpleFinalSyncStartRequest()
	request.RequestID = "req-runtime-sync-live-events-001"
	eventBatches := make(chan []interfaces.FactoryEvent, 8)
	request.EventConsumer = func(events []interfaces.FactoryEvent) {
		eventBatches <- events
	}
	done := make(chan error, 1)
	go func() {
		_, err := service.StartSync(context.Background(), request)
		done <- err
	}()

	select {
	case <-phasePublished:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live JavaScript phase")
	}
	observed := collectFactoryEventTypes(eventBatches)
	if !containsFactoryEventType(observed, interfaces.FactoryEventTypeOrchestratorPhaseChanged) {
		t.Fatalf("events before completion = %v, want ORCHESTRATOR_PHASE_CHANGED", observed)
	}
	select {
	case err := <-done:
		t.Fatalf("StartSync completed before workflow release: %v", err)
	default:
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	observed = append(observed, collectFactoryEventTypes(eventBatches)...)
	phaseIndex := indexFactoryEventType(observed, interfaces.FactoryEventTypeOrchestratorPhaseChanged)
	checkpointIndex := indexFactoryEventType(observed, interfaces.FactoryEventTypeOrchestratorCheckpointWritten)
	if phaseIndex < 0 || checkpointIndex <= phaseIndex {
		t.Fatalf("canonical JavaScript event order = %v", observed)
	}
}

func collectFactoryEventTypes(batches <-chan []interfaces.FactoryEvent) []interfaces.FactoryEventType {
	var eventTypes []interfaces.FactoryEventType
	for {
		select {
		case events := <-batches:
			for _, event := range events {
				eventTypes = append(eventTypes, event.Type)
			}
		default:
			return eventTypes
		}
	}
}

func containsFactoryEventType(
	eventTypes []interfaces.FactoryEventType,
	want interfaces.FactoryEventType,
) bool {
	return indexFactoryEventType(eventTypes, want) >= 0
}

func indexFactoryEventType(
	eventTypes []interfaces.FactoryEventType,
	want interfaces.FactoryEventType,
) int {
	for index, eventType := range eventTypes {
		if eventType == want {
			return index
		}
	}
	return -1
}

func simpleFinalSyncStartRequest() fse.StartRequest {
	return fse.StartRequest{
		RequestID: "req-runtime-sync-simple-final-001",
		Source: fse.Source{
			Kind: factory.WorkflowSourceKindInlineWorkflow,
			InlineWorkflow: &fse.InlineWorkflowSource{
				InlineSource: simpleFinalWorkflowSource,
				Dialect:      "you-workflow-v1",
				Metadata: map[string]string{
					"name":        "simple-final",
					"description": "returns a structured final value",
				},
			},
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   3,
			"prefix":  "you",
		},
		RequestedPolicy: map[string]any{
			"policyHash": "req-policy-runtime-sync-001",
		},
	}
}

func assertSyncStartCompleted(t *testing.T, started fse.SyncStartResult) {
	t.Helper()
	if started.SyncOutcome != fse.SyncOutcomeCompleted {
		t.Fatalf("syncOutcome = %q, want COMPLETED", started.SyncOutcome)
	}
	if started.Status != string(fse.LifecycleStatusSucceeded) {
		t.Fatalf("status = %q, want SUCCEEDED", started.Status)
	}
	if started.OrchestratorKind != interfaces.OrchestratorKindJavaScript {
		t.Fatalf("orchestratorKind = %q, want JAVASCRIPT", started.OrchestratorKind)
	}
	if started.SessionID == "" {
		t.Fatal("expected durable session id")
	}
	if started.ResolvedSource.SourceRef == "" || started.SourceHash == "" {
		t.Fatalf("resolved source = %#v", started.ResolvedSource)
	}
}

func assertSucceededSessionReads(t *testing.T, service fse.Service, sessionID string) {
	t.Helper()
	session, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Status != fse.LifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", session.Status)
	}
	if session.ResultSummary == nil || session.ResultSummary.ResultStatus != string(fse.ResultStatusFinal) {
		t.Fatalf("resultSummary = %#v, want FINAL", session.ResultSummary)
	}
	if err := fse.ValidateResultMatchesSessionRead(session, fse.ResultReadResult{
		SessionID:     session.SessionID,
		ResultStatus:  fse.ResultStatusFinal,
		SessionStatus: session.Status,
	}); err != nil {
		t.Fatalf("ValidateResultMatchesSessionRead: %v", err)
	}
}

func assertSimpleFinalPrimaryResult(t *testing.T, result fse.ResultReadResult) {
	t.Helper()
	if result.ResultStatus != fse.ResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", result.ResultStatus)
	}
	projected := decodePrimaryResultMap(t, result.PrimaryResult)
	want := map[string]any{
		"label":       "simple-final",
		"description": "returns a structured final value",
		"subject":     "workflows",
		"repeat":      float64(3),
		"echo":        "you:workflows",
	}
	for key, wantValue := range want {
		if projected[key] != wantValue {
			t.Fatalf("primaryResult[%q] = %#v, want %#v", key, projected[key], wantValue)
		}
	}

	hash, err := fixtures.ProjectedResultReadHash(result)
	if err != nil {
		t.Fatalf("fixtures.ProjectedResultReadHash: %v", err)
	}
	if hash == "" {
		t.Fatal("expected stable projected result hash")
	}
}

func TestJavaScriptRuntimeService_StartAsync_SimpleWorkflowCompletesWithInspectableResult(t *testing.T) {
	service := newJavaScriptRuntimeService(t, successfulFixtureWorkflows(map[string]any{"echo": "you:workflows"}))

	started, err := service.StartAsync(context.Background(), inlineWorkflowStartRequest(
		"req-runtime-async-simple-final-001",
		simpleFinalWorkflowSource,
		map[string]any{
			"subject": "workflows",
			"count":   2,
			"prefix":  "you",
		},
		nil,
	))
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.SessionID == "" {
		t.Fatal("expected durable session id")
	}
	if started.Status != string(fse.LifecycleStatusRunning) {
		t.Fatalf("start status = %q, want RUNNING", started.Status)
	}

	session := waitUntilSessionStatus(t, service, started.SessionID, fse.LifecycleStatusSucceeded, 5*time.Second)
	if session.ResultSummary == nil || session.ResultSummary.ResultStatus != string(fse.ResultStatusFinal) {
		t.Fatalf("resultSummary = %#v, want FINAL", session.ResultSummary)
	}

	result, err := service.GetResult(context.Background(), started.SessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", result.ResultStatus)
	}
	projected := decodePrimaryResultMap(t, result.PrimaryResult)
	if projected["echo"] != "you:workflows" {
		t.Fatalf("primaryResult echo = %#v, want you:workflows", projected["echo"])
	}
}

func TestJavaScriptRuntimeService_StartAsync_RunningBeforeCompletion(t *testing.T) {
	service := newJavaScriptRuntimeService(t, blockingFixtureWorkflows())

	started, err := service.StartAsync(context.Background(), inlineWorkflowStartRequest(
		"req-runtime-async-running-001",
		busyLoopWorkflowSource,
		map[string]any{"subject": "workflows"},
		nil,
	))
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.Status != string(fse.LifecycleStatusRunning) {
		t.Fatalf("start status = %q, want RUNNING", started.Status)
	}

	session, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Status != fse.LifecycleStatusRunning {
		t.Fatalf("session status = %q, want RUNNING", session.Status)
	}

	result, err := service.GetResult(context.Background(), started.SessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusNotReady {
		t.Fatalf("resultStatus = %q, want NOT_READY", result.ResultStatus)
	}
	if result.Availability == nil || result.Availability.Reason != "RESULT_NOT_READY" {
		t.Fatalf("availability = %#v, want RESULT_NOT_READY", result.Availability)
	}

	cancelled, err := service.Cancel(context.Background(), started.SessionID, fse.ControlRequest{})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Outcome != fse.LifecycleControlOutcomeAccepted {
		t.Fatalf("cancel outcome = %q, want ACCEPTED", cancelled.Outcome)
	}
}

func TestJavaScriptRuntimeService_StartAsync_Failed(t *testing.T) {
	service := newJavaScriptRuntimeService(t, failedFixtureWorkflows())
	started, err := service.StartAsync(context.Background(), inlineWorkflowStartRequest(
		"req-runtime-async-failed-001",
		throwErrorWorkflowSource,
		map[string]any{"subject": "workflows"},
		nil,
	))
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	session := waitUntilSessionStatus(t, service, started.SessionID, fse.LifecycleStatusFailed, 5*time.Second)
	if session.Failure == nil || session.Failure.Reason == "" {
		t.Fatalf("failure = %#v, want runtime failure summary", session.Failure)
	}

	result, err := service.GetResult(context.Background(), started.SessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusUnavailable {
		t.Fatalf("resultStatus = %q, want UNAVAILABLE", result.ResultStatus)
	}
	if result.SessionStatus != fse.LifecycleStatusFailed {
		t.Fatalf("sessionStatus = %q, want FAILED", result.SessionStatus)
	}
}

func TestJavaScriptRuntimeService_StartAsync_TimedOut(t *testing.T) {
	service := newJavaScriptRuntimeService(t, blockingFixtureWorkflows())
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

	session := waitUntilSessionStatus(t, service, started.SessionID, fse.LifecycleStatusTimedOut, 5*time.Second)
	if session.Failure == nil || session.Failure.Reason != "WORKFLOW_RUNTIME_TIMEOUT" {
		t.Fatalf("failure = %#v, want WORKFLOW_RUNTIME_TIMEOUT", session.Failure)
	}

	result, err := service.GetResult(context.Background(), started.SessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusUnavailable {
		t.Fatalf("resultStatus = %q, want UNAVAILABLE", result.ResultStatus)
	}
	if result.Availability != nil && result.Availability.Reason == "SYNC_WAIT_TIMED_OUT" {
		t.Fatalf("availability = %#v, must not use sync-wait reason for runtime policy timeout", result.Availability)
	}
	if result.SessionStatus != fse.LifecycleStatusTimedOut {
		t.Fatalf("sessionStatus = %q, want TIMED_OUT", result.SessionStatus)
	}
}

func TestJavaScriptRuntimeService_StartAsync_Canceled(t *testing.T) {
	service := newJavaScriptRuntimeService(t, blockingFixtureWorkflows())
	started, err := service.StartAsync(context.Background(), inlineWorkflowStartRequest(
		"req-runtime-async-canceled-001",
		busyLoopWorkflowSource,
		map[string]any{"subject": "workflows"},
		nil,
	))
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	if _, err := service.Cancel(context.Background(), started.SessionID, fse.ControlRequest{}); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	session := waitUntilSessionStatus(t, service, started.SessionID, fse.LifecycleStatusCanceled, 5*time.Second)
	if session.Failure == nil || session.Failure.Reason != "WORKFLOW_RUNTIME_CANCELED" {
		t.Fatalf("failure = %#v, want WORKFLOW_RUNTIME_CANCELED", session.Failure)
	}

	result, err := service.GetResult(context.Background(), started.SessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.SessionStatus != fse.LifecycleStatusCanceled {
		t.Fatalf("sessionStatus = %q, want CANCELED", result.SessionStatus)
	}
	if result.ResultStatus != fse.ResultStatusUnavailable {
		t.Fatalf("resultStatus = %q, want UNAVAILABLE", result.ResultStatus)
	}
}

func TestJavaScriptRuntimeService_StartSync_WaitTimeoutWithoutCancelKeepsSessionRunning(t *testing.T) {
	service := newJavaScriptRuntimeService(t, blockingFixtureWorkflows())
	waitMillis := int64(50)
	started, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-sync-wait-timeout-001",
		Source: fse.Source{
			Kind: factory.WorkflowSourceKindInlineWorkflow,
			InlineWorkflow: &fse.InlineWorkflowSource{
				InlineSource: busyLoopWorkflowSource,
				Dialect:      "you-workflow-v1",
				Metadata:     map[string]string{"name": "runtime-sync-wait-fixture"},
			},
		},
		Args: map[string]any{"subject": "workflows"},
		Wait: &fse.WaitOptions{TimeoutMillis: &waitMillis},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if started.SyncOutcome != fse.SyncOutcomeTimedOut || !started.TimedOut {
		t.Fatalf("sync response = %#v, want TIMED_OUT", started)
	}
	if started.SessionCanceledByTimeout {
		t.Fatal("sessionCanceledByTimeout = true, want false")
	}
	if started.Status != string(fse.LifecycleStatusRunning) {
		t.Fatalf("status = %q, want RUNNING", started.Status)
	}

	session, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Status != fse.LifecycleStatusRunning {
		t.Fatalf("session status = %q, want RUNNING after sync wait timeout", session.Status)
	}

	result, err := service.GetResult(context.Background(), started.SessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.Availability == nil || result.Availability.Reason != "SYNC_WAIT_TIMED_OUT" {
		t.Fatalf("availability = %#v, want SYNC_WAIT_TIMED_OUT", result.Availability)
	}
}

func TestNewExecutionService_SelectsFakeAndJavaScriptRuntimeProviders(t *testing.T) {
	fakeService, err := newExecutionService(
		fse.ExecutionProviderFake,
		executionServiceConfig{},
	)
	if err != nil {
		t.Fatalf("NewExecutionService(fake): %v", err)
	}
	if _, ok := fakeService.(*fse.FakeService); !ok {
		t.Fatalf("fake provider type = %T, want *fse.FakeService", fakeService)
	}

	projectRoot := t.TempDir()
	runtimeService, err := newExecutionService(
		fse.ExecutionProviderJavaScriptRuntime,
		executionServiceConfig{ProjectRoot: projectRoot, Persistence: fse.DisabledPersistence(), Clock: fixtureClock{now: time.Now()}},
	)
	if err != nil {
		t.Fatalf("NewExecutionService(runtime): %v", err)
	}
	if _, ok := runtimeService.(*fse.JavaScriptRuntimeService); !ok {
		t.Fatalf("runtime provider type = %T, want *fse.JavaScriptRuntimeService", runtimeService)
	}
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

func newJavaScriptRuntimeService(t *testing.T, workflows ...factory.JavaScriptWorkflows) fse.Service {
	t.Helper()
	projectRoot, err := os.MkdirTemp("", "runtime-execution-fixture-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	config := executionServiceConfig{
		ProjectRoot: projectRoot, Persistence: fse.DisabledPersistence(),
		Clock: fixtureClock{now: time.Now()},
	}
	if len(workflows) > 0 {
		config.Workflows = workflows[0]
	}
	service, err := newExecutionService(
		fse.ExecutionProviderJavaScriptRuntime,
		config,
	)
	if err != nil {
		t.Fatalf("NewExecutionService: %v", err)
	}
	t.Cleanup(func() {
		drainRuntimeSessions(t, service)
		removeRuntimeProjectState(t, projectRoot)
	})
	return service
}

func successfulFixtureWorkflows(value map[string]any) factory.JavaScriptWorkflows {
	return factoryruntimefixtures.ScriptedJavaScriptWorkflows{
		RunFunc: func(context.Context, factory.JavaScriptRuntimeRequest, factory.JavaScriptRuntimeHooks) (factory.JavaScriptRuntimeOutcome, error) {
			encoded, err := json.Marshal(value)
			return factory.JavaScriptRuntimeOutcome{
				OK: true, Value: factory.TypedValue{JSON: encoded},
			}, err
		},
	}
}

func blockingFixtureWorkflows() factory.JavaScriptWorkflows {
	return factoryruntimefixtures.ScriptedJavaScriptWorkflows{
		RunFunc: func(ctx context.Context, _ factory.JavaScriptRuntimeRequest, _ factory.JavaScriptRuntimeHooks) (factory.JavaScriptRuntimeOutcome, error) {
			<-ctx.Done()
			code := factory.JavaScriptRuntimeCodeCanceled
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				code = factory.JavaScriptRuntimeCodeTimeout
			}
			return factory.JavaScriptRuntimeOutcome{
				Failure: factory.JavaScriptRuntimeFailure{Code: code, Message: ctx.Err().Error()},
			}, nil
		},
	}
}

func failedFixtureWorkflows() factory.JavaScriptWorkflows {
	return factoryruntimefixtures.ScriptedJavaScriptWorkflows{
		RunFunc: func(context.Context, factory.JavaScriptRuntimeRequest, factory.JavaScriptRuntimeHooks) (factory.JavaScriptRuntimeOutcome, error) {
			return factory.JavaScriptRuntimeOutcome{
				Failure: factory.JavaScriptRuntimeFailure{
					Code: factory.JavaScriptRuntimeCodeScriptError, Message: "scripted failure",
				},
			}, nil
		},
	}
}

func drainRuntimeSessions(t *testing.T, service fse.Service) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		list, err := service.ListSessions(context.Background(), fse.ListSessionsRequest{
			Scope: fse.SessionListScopeAll,
		})
		if err != nil {
			return
		}

		pending := false
		for _, session := range list.DurableSessions {
			if fse.IsTerminalLifecycleStatus(session.Status) {
				continue
			}
			pending = true
			_, _ = service.Terminate(context.Background(), session.SessionID, fse.ControlRequest{
				Reason: "test cleanup",
			})
		}
		if !pending {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func removeRuntimeProjectState(t *testing.T, projectRoot string) {
	t.Helper()
	runtimeStateRoot := filepath.Join(projectRoot, ".you-agent-factory")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := os.RemoveAll(runtimeStateRoot); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	_ = os.RemoveAll(runtimeStateRoot)
}

func inlineWorkflowStartRequest(
	requestID string,
	source string,
	args map[string]any,
	requestedPolicy map[string]any,
) fse.StartRequest {
	return fse.StartRequest{
		RequestID: requestID,
		Source: fse.Source{
			Kind: factory.WorkflowSourceKindInlineWorkflow,
			InlineWorkflow: &fse.InlineWorkflowSource{
				InlineSource: source,
				Dialect:      "you-workflow-v1",
				Metadata: map[string]string{
					"name": "runtime-async-fixture",
				},
			},
		},
		Args:            args,
		RequestedPolicy: requestedPolicy,
	}
}

func waitUntilSessionStatus(
	t *testing.T,
	service fse.Service,
	sessionID string,
	want fse.LifecycleStatus,
	timeout time.Duration,
) fse.SessionReadResult {
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
		if fse.IsTerminalLifecycleStatus(session.Status) && session.Status != want {
			t.Fatalf("session %s reached terminal %q before %q", sessionID, session.Status, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session %s did not reach status %q within %s", sessionID, want, timeout)
	return fse.SessionReadResult{}
}

func TestJavaScriptRuntimeService_EventReplay_ReconstructsCompletedSessionProjection(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final",
		successfulFixtureWorkflows(map[string]any{"status": "completed"}))

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-event-replay-complete",
		Source: fse.Source{
			Kind:         factory.WorkflowSourceKindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   3,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	liveSession, liveResult, events := readRuntimeSessionEvents(t, service, completed.SessionID)
	assertRuntimeEventSource(t, events.Events)

	replayedSession, replayedResult, err := fse.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	assertReplayedSessionMatchesLive(t, liveSession, replayedSession)
	assertReplayedResultStatusMatchesLive(t, liveResult, replayedResult)
	assertReplayedResultMatchesEventProjection(t, replayedResult, events.Events)
	assertReplayedResultMatchesSessionRead(t, replayedSession, replayedResult)

	mappedSession := factorysession.SessionReadResponseToAPI(replayedSession)
	if mappedSession.Status != factorysession.SessionReadResponseToAPI(liveSession).Status {
		t.Fatalf("mapped replayed status = %q, want %q", mappedSession.Status, factorysession.SessionReadResponseToAPI(liveSession).Status)
	}
}

func TestJavaScriptRuntimeService_EventReplay_ReconstructsRunningSessionProjection(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop",
		blockingFixtureWorkflows())

	started, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-event-replay-running",
		Source: fse.Source{
			Kind:         factory.WorkflowSourceKindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	liveSession, _, events := readRuntimeSessionEvents(t, service, started.SessionID)
	assertRuntimeEventSource(t, events.Events)
	if len(events.Events) < 2 {
		t.Fatalf("events = %d, want start and result-updated", len(events.Events))
	}

	replayedSession, replayedResult, err := fse.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	assertReplayedSessionMatchesLive(t, liveSession, replayedSession)
	if replayedResult.ResultStatus != fse.ResultStatusNotReady {
		t.Fatalf("replayed resultStatus = %q, want NOT_READY", replayedResult.ResultStatus)
	}
}

func TestJavaScriptRuntimeService_EventReplay_ReconstructsSyncTimeoutProjection(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop",
		blockingFixtureWorkflows())
	timeoutMillis := int64(25)

	timedOut, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-event-replay-timeout",
		Source: fse.Source{
			Kind:         factory.WorkflowSourceKindWorkflowName,
			WorkflowName: "busy-loop",
		},
		Wait: &fse.WaitOptions{
			TimeoutMillis: &timeoutMillis,
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	liveSession, liveResult, events := readRuntimeSessionEvents(t, service, timedOut.SessionID)
	assertRuntimeEventSource(t, events.Events)

	replayedSession, replayedResult, err := fse.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	assertReplayedSessionMatchesLive(t, liveSession, replayedSession)
	assertReplayedResultStatusMatchesLive(t, liveResult, replayedResult)
	if replayedResult.Availability == nil || replayedResult.Availability.Reason != "SYNC_WAIT_TIMED_OUT" {
		t.Fatalf("replayed availability = %#v, want SYNC_WAIT_TIMED_OUT", replayedResult.Availability)
	}
}

func TestJavaScriptRuntimeService_EventReplay_IsIdempotent(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final",
		successfulFixtureWorkflows(map[string]any{"status": "completed"}))

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-event-replay-idempotent",
		Source: fse.Source{
			Kind:         factory.WorkflowSourceKindWorkflowName,
			WorkflowName: "simple-final",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	events, err := service.ReadEvents(context.Background(), completed.SessionID, fse.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	assertRuntimeEventSource(t, events.Events)

	firstSession, firstResult, err := fse.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection first: %v", err)
	}
	secondSession, secondResult, err := fse.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection second: %v", err)
	}
	assertReplayProjectionStable(t, firstSession, secondSession, firstResult, secondResult)
}

func TestJavaScriptRuntimeService_EventReplay_ReconstructsAsyncCompletedSession(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final",
		successfulFixtureWorkflows(map[string]any{"status": "completed"}))

	started, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-event-replay-async-complete",
		Source: fse.Source{
			Kind:         factory.WorkflowSourceKindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   3,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		liveSession, err := service.GetSession(context.Background(), started.SessionID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if liveSession.Status == fse.LifecycleStatusSucceeded {
			events, err := service.ReadEvents(context.Background(), started.SessionID, fse.EventReconnectRequest{})
			if err != nil {
				t.Fatalf("ReadEvents: %v", err)
			}
			assertRuntimeEventSource(t, events.Events)
			replayedSession, replayedResult, err := fse.ReplaySessionProjection(events.Events)
			if err != nil {
				t.Fatalf("ReplaySessionProjection: %v", err)
			}
			assertReplayedSessionMatchesLive(t, liveSession, replayedSession)
			if replayedResult.ResultStatus != fse.ResultStatusFinal {
				t.Fatalf("replayed resultStatus = %q, want FINAL", replayedResult.ResultStatus)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("session still %q after wait, want SUCCEEDED", liveSession.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
