package factorysessionexecution

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/workflowsource"
)

func contractFixturesPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "api", "testdata", "durable-session-contract-fixtures.json")
}

func newContractFakeService(t *testing.T) *FakeService {
	t.Helper()
	service, err := NewFakeServiceFromContractFixtures(contractFixturesPath(t))
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}
	return service
}

func startAsyncByRequestID(t *testing.T, service *FakeService, requestID string) AsyncStartResult {
	t.Helper()
	result, err := service.StartAsync(context.Background(), StartRequest{
		RequestID: requestID,
		Source:    Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
	})
	if err != nil {
		t.Fatalf("StartAsync(%q): %v", requestID, err)
	}
	return result
}

func TestFakeService_StartAsync_ProjectsFixtureScenarios(t *testing.T) {
	service := newContractFakeService(t)
	cases := []struct {
		requestID string
		sessionID string
		status    LifecycleStatus
		result    ResultStatus
	}{
		{"req-petri-run-001", "dur-sess-petri-run-001", LifecycleStatusRunning, ResultStatusNotReady},
		{"req-js-run-n-001", "dur-sess-js-run-n-001", LifecycleStatusRunning, ResultStatusPartial},
		{"req-petri-success-001", "dur-sess-petri-success-001", LifecycleStatusSucceeded, ResultStatusFinal},
		{"req-js-failed-partial-001", "dur-sess-js-failed-partial-001", LifecycleStatusFailed, ResultStatusFailedWithPartial},
		{"req-petri-cancel-001", "dur-sess-petri-cancel-001", LifecycleStatusCanceled, ResultStatusUnavailable},
		{"req-js-timeout-001", "dur-sess-js-timeout-001", LifecycleStatusRunning, ResultStatusNotReady},
		{"req-js-interrupted-001", "dur-sess-js-interrupted-001", LifecycleStatusInterrupted, ResultStatusPartial},
	}
	for _, tc := range cases {
		t.Run(tc.requestID, func(t *testing.T) {
			started, err := service.StartAsync(context.Background(), StartRequest{
				RequestID: tc.requestID,
				Source:    Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
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
			result, err := service.GetResult(context.Background(), tc.sessionID, ResultRequest{})
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
	service := newContractFakeService(t)
	req := StartRequest{
		RequestID: "req-idempotent-replay-001",
		Source: Source{
			Kind:         workflowsource.KindWorkflowFile,
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

func TestFakeService_StartSync_TerminalAndTimeoutFixtures(t *testing.T) {
	service := newContractFakeService(t)

	terminal, err := service.StartSync(context.Background(), StartRequest{
		RequestID: "req-petri-success-001",
		Source:    Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
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
		Source:    Source{Kind: workflowsource.KindWorkflowName, WorkflowName: "long-running-audit"},
		Wait:      &WaitOptions{TimeoutMillis: int64Ptr(30000)},
	})
	if err != nil {
		t.Fatalf("StartSync timeout: %v", err)
	}
	if timedOut.SyncOutcome != SyncOutcomeTimedOut || !timedOut.TimedOut {
		t.Fatalf("timeout response = %#v", timedOut)
	}
}

func TestFakeService_LifecycleControls_UpdateStateAndErrors(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-js-run-n-001")

	paused, err := service.Pause(context.Background(), "dur-sess-js-run-n-001", ControlRequest{})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.Outcome != LifecycleControlOutcomeAccepted || paused.Status != LifecycleStatusPaused {
		t.Fatalf("pause result = %#v", paused)
	}

	resumed, err := service.Resume(context.Background(), "dur-sess-js-run-n-001", ControlRequest{})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if resumed.Status != LifecycleStatusRunning {
		t.Fatalf("resume status = %q, want RUNNING", resumed.Status)
	}

	startAsyncByRequestID(t, service, "req-petri-success-001")
	_, err = service.RetryDispatch(context.Background(), "dur-sess-petri-success-001", RetryDispatchRequest{
		ControlRequest: ControlRequest{},
		DispatchID:     "disp-petri-success-001",
	})
	var controlErr *ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != LifecycleControlOutcomeTerminalSession {
		t.Fatalf("retry on terminal error = %v, want TERMINAL_SESSION", err)
	}

	startAsyncByRequestID(t, service, "req-js-failed-partial-001")
	retry, err := service.RetryDispatch(context.Background(), "dur-sess-js-failed-partial-001", RetryDispatchRequest{
		ControlRequest: ControlRequest{},
		DispatchID:     "disp-js-fail-002",
	})
	if err != nil {
		t.Fatalf("RetryDispatch on failed session: %v", err)
	}
	if retry.Outcome != LifecycleControlOutcomeAccepted || retry.Status != LifecycleStatusRunning {
		t.Fatalf("retry result = %#v", retry)
	}

	_, err = service.RetryDispatch(context.Background(), "dur-sess-js-failed-partial-001", RetryDispatchRequest{
		ControlRequest: ControlRequest{},
		DispatchID:     "missing-dispatch",
	})
	if !errors.Is(err, ErrDispatchNotFound) {
		t.Fatalf("missing dispatch error = %v", err)
	}
}

func TestFakeService_ReadProjections_MatchFixtureDispatchesArtifactsEvents(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-success-001")

	dispatches, err := service.ListDispatches(context.Background(), "dur-sess-petri-success-001")
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 1 || dispatches.Dispatches[0].ID != "disp-petri-success-001" {
		t.Fatalf("dispatches = %#v", dispatches.Dispatches)
	}

	artifacts, err := service.ListArtifacts(context.Background(), "dur-sess-petri-success-001")
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(artifacts.Artifacts) != 1 || artifacts.Artifacts[0].ID != "art-petri-final-001" {
		t.Fatalf("artifacts = %#v", artifacts.Artifacts)
	}

	events, err := service.ReadEvents(context.Background(), "dur-sess-petri-success-001", EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events.Events) == 0 {
		t.Fatal("events missing")
	}
	result, err := service.GetResult(context.Background(), "dur-sess-petri-success-001", ResultRequest{})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if err := ValidateResultMatchesEventProjection(result, events.Events); err != nil {
		t.Fatalf("ValidateResultMatchesEventProjection: %v", err)
	}
}

func TestFakeService_ListSessions_ScopedPersistedAndLive(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-run-001")

	persisted, err := service.ListSessions(context.Background(), ListSessionsRequest{
		Scope: SessionListScopePersisted,
	})
	if err != nil {
		t.Fatalf("ListSessions persisted: %v", err)
	}
	if len(persisted.DurableSessions) == 0 {
		t.Fatal("persisted sessions missing seeded terminal rows")
	}

	all, err := service.ListSessions(context.Background(), ListSessionsRequest{
		Scope: SessionListScopeAll,
	})
	if err != nil {
		t.Fatalf("ListSessions all: %v", err)
	}
	foundRunning := false
	for _, summary := range all.DurableSessions {
		if summary.SessionID == "dur-sess-petri-run-001" {
			foundRunning = true
			break
		}
	}
	if !foundRunning {
		t.Fatalf("all durable sessions = %#v", all.DurableSessions)
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}
