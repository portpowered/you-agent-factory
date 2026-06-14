package fixtures_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
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

func TestJavaScriptRuntimeService_StartSync_SimpleWorkflowCompletesWithPrimaryResult(t *testing.T) {
	projectRoot := writeSimpleFinalWorkflowProject(t)
	service, err := fse.NewExecutionService(
		fse.ExecutionProviderJavaScriptRuntime,
		fse.ServiceConfig{ProjectRoot: projectRoot},
	)
	if err != nil {
		t.Fatalf("NewExecutionService: %v", err)
	}

	started, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-sync-simple-final-001",
		Source: fse.Source{
			Kind: workflowsource.KindInlineWorkflow,
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
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
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

	session, err := service.GetSession(context.Background(), started.SessionID)
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
	service := newJavaScriptRuntimeService(t)

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
	service := newJavaScriptRuntimeService(t)

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

func TestJavaScriptRuntimeService_StartAsync_TerminalOutcomes(t *testing.T) {
	t.Run("failed", func(t *testing.T) {
		service := newJavaScriptRuntimeService(t)
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
	})

	t.Run("timed-out", func(t *testing.T) {
		service := newJavaScriptRuntimeService(t)
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
	})

	t.Run("canceled", func(t *testing.T) {
		service := newJavaScriptRuntimeService(t)
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
	})
}

func TestJavaScriptRuntimeService_StartSync_WaitTimeoutWithoutCancelKeepsSessionRunning(t *testing.T) {
	service := newJavaScriptRuntimeService(t)
	waitMillis := int64(50)
	started, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-sync-wait-timeout-001",
		Source: fse.Source{
			Kind: workflowsource.KindInlineWorkflow,
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
	fakeService, err := fse.NewExecutionService(
		fse.ExecutionProviderFake,
		fse.ServiceConfig{},
	)
	if err != nil {
		t.Fatalf("NewExecutionService(fake): %v", err)
	}
	if _, ok := fakeService.(*fse.FakeService); !ok {
		t.Fatalf("fake provider type = %T, want *fse.FakeService", fakeService)
	}

	projectRoot := t.TempDir()
	runtimeService, err := fse.NewExecutionService(
		fse.ExecutionProviderJavaScriptRuntime,
		fse.ServiceConfig{ProjectRoot: projectRoot},
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
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
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
	var projected map[string]any
	if err := json.Unmarshal(raw, &projected); err != nil {
		t.Fatalf("unmarshal primary result: %v", err)
	}
	return projected
}

func newJavaScriptRuntimeService(t *testing.T) fse.Service {
	t.Helper()
	projectRoot := t.TempDir()
	service, err := fse.NewExecutionService(
		fse.ExecutionProviderJavaScriptRuntime,
		fse.ServiceConfig{ProjectRoot: projectRoot},
	)
	if err != nil {
		t.Fatalf("NewExecutionService: %v", err)
	}
	return service
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
			Kind: workflowsource.KindInlineWorkflow,
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
