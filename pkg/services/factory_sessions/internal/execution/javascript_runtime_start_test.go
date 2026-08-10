package factorysessionexecution

import (
	"context"
	"errors"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

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
