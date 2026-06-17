package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"
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

func TestFakeService_StartAsync_ErrorBranches(t *testing.T) {
	service := newContractFakeService(t)

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.StartAsync(canceledCtx, StartRequest{
		RequestID: "req-petri-run-001",
		Source:    Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled StartAsync error = %v", err)
	}

	if _, err := service.StartAsync(context.Background(), StartRequest{}); err == nil {
		t.Fatal("invalid StartAsync request should fail")
	}

	if _, err := service.StartAsync(context.Background(), StartRequest{
		RequestID: "missing-scenario",
		Source:    Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
	}); err == nil {
		t.Fatal("unknown scenario should fail")
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
	if timedOut.SessionCanceledByTimeout {
		t.Fatal("sessionCanceledByTimeout = true, want false without cancel-on-timeout")
	}
}

func TestFakeService_StartSync_AppliesCancelOnTimeoutOverlay(t *testing.T) {
	service := newContractFakeService(t)
	timedOut, err := service.StartSync(context.Background(), StartRequest{
		RequestID: "req-js-timeout-001",
		Source:    Source{Kind: workflowsource.KindWorkflowName, WorkflowName: "long-running-audit"},
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
	service := newContractFakeService(t)

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.StartSync(canceledCtx, StartRequest{
		RequestID: "req-petri-success-001",
		Source:    Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled StartSync error = %v", err)
	}

	if _, err := service.StartSync(context.Background(), StartRequest{}); err == nil {
		t.Fatal("invalid StartSync request should fail")
	}

	asyncService := newContractFakeService(t)
	if _, err := asyncService.StartAsync(context.Background(), StartRequest{
		RequestID: "req-petri-run-001",
		Source:    Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
	}); err != nil {
		t.Fatalf("seed StartAsync: %v", err)
	}
	if _, err := asyncService.StartSync(context.Background(), StartRequest{
		RequestID: "req-petri-run-001",
		Source:    Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
	}); !errors.Is(err, ErrExecutionRequestIDConflict) {
		t.Fatalf("async replay StartSync error = %v, want ErrExecutionRequestIDConflict", err)
	}
}

func TestFakeService_LifecycleControl_IdempotentReplayAndConflict(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-js-run-n-001")

	first, err := service.Pause(context.Background(), "dur-sess-js-run-n-001", ControlRequest{
		RequestID: "ctrl-pause-replay-001",
	})
	if err != nil {
		t.Fatalf("first Pause: %v", err)
	}
	second, err := service.Pause(context.Background(), "dur-sess-js-run-n-001", ControlRequest{
		RequestID: "ctrl-pause-replay-001",
	})
	if err != nil {
		t.Fatalf("replay Pause: %v", err)
	}
	if second.Outcome != first.Outcome || second.Status != first.Status {
		t.Fatalf("replay result = %#v, want %#v", second, first)
	}

	_, err = service.Resume(context.Background(), "dur-sess-js-run-n-001", ControlRequest{
		RequestID: "ctrl-pause-replay-001",
	})
	var controlErr *ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != LifecycleControlOutcomeConflict {
		t.Fatalf("conflict error = %v, want CONFLICT ControlError", err)
	}
	if controlErr.Status != LifecycleStatusPaused {
		t.Fatalf("conflict status = %q, want PAUSED", controlErr.Status)
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

func TestFakeService_LifecycleControl_ErrorBranches(t *testing.T) {
	service := newContractFakeService(t)

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Pause(canceledCtx, "dur-sess-js-run-n-001", ControlRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Pause error = %v", err)
	}
	if _, err := service.Pause(context.Background(), " ", ControlRequest{}); err == nil {
		t.Fatal("blank session id should fail")
	}
	if _, err := service.Pause(context.Background(), "missing-session", ControlRequest{}); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("missing session Pause error = %v", err)
	}

	startAsyncByRequestID(t, service, "req-petri-success-001")
	if _, err := service.Terminate(context.Background(), "dur-sess-petri-success-001", ControlRequest{RequestID: "term-1"}); err == nil {
		t.Fatal("terminate on terminal session should fail")
	}
	if _, err := service.Terminate(context.Background(), "dur-sess-petri-success-001", ControlRequest{RequestID: "term-1"}); err == nil {
		t.Fatal("terminate replay should repeat stored error")
	}

	startAsyncByRequestID(t, service, "req-js-awaiting-001")
	if _, err := service.Approve(context.Background(), "dur-sess-js-awaiting-001", ApproveRequest{}); err != nil {
		t.Fatalf("Approve awaiting session: %v", err)
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

func TestFakeService_ReadMethods_ErrorAndFallbackBranches(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-success-001")

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.GetSession(canceledCtx, "dur-sess-petri-success-001"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled GetSession error = %v", err)
	}
	if _, err := service.GetSession(context.Background(), " "); err == nil {
		t.Fatal("blank GetSession id should fail")
	}
	if _, err := service.GetSession(context.Background(), "missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("missing GetSession error = %v", err)
	}

	if _, err := service.GetResult(canceledCtx, "dur-sess-petri-success-001", ResultRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled GetResult error = %v", err)
	}
	if _, err := service.GetResult(context.Background(), "dur-sess-petri-success-001", ResultRequest{Mode: ResultMode("bad")}); err == nil {
		t.Fatal("invalid GetResult mode should fail")
	}

	if _, err := service.ListDispatches(canceledCtx, "dur-sess-petri-success-001"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ListDispatches error = %v", err)
	}
	detail, err := service.GetDispatch(context.Background(), "dur-sess-petri-success-001", "disp-petri-success-001")
	if err != nil {
		t.Fatalf("GetDispatch detail: %v", err)
	}
	if detail.OrchestratorKind == "" {
		t.Fatalf("dispatch detail = %#v", detail)
	}
	fallbackService := NewFakeService(WithFakeScenarios(FakeScenario{
		RequestID: "fallback-dispatch",
		Session: SessionReadResult{
			SessionID:        "dur-sess-fallback",
			Status:           LifecycleStatusRunning,
			OrchestratorKind: "PETRI",
		},
		Result: ResultReadResult{
			SessionID:     "dur-sess-fallback",
			SessionStatus: LifecycleStatusRunning,
			ResultStatus:  ResultStatusNotReady,
		},
		Dispatches: []DispatchSummary{{ID: "disp-fallback", DispatchKind: "PETRI", Status: DispatchStatusQueued}},
	}))
	startAsyncByRequestID(t, fallbackService, "fallback-dispatch")
	fallback, err := fallbackService.GetDispatch(context.Background(), "dur-sess-fallback", "disp-fallback")
	if err != nil {
		t.Fatalf("GetDispatch fallback: %v", err)
	}
	if fallback.ID != "disp-fallback" || fallback.OrchestratorKind != "PETRI" {
		t.Fatalf("fallback dispatch detail = %#v", fallback)
	}
	if _, err := service.GetDispatch(context.Background(), "dur-sess-petri-success-001", "missing"); !errors.Is(err, ErrDispatchNotFound) {
		t.Fatalf("missing GetDispatch error = %v", err)
	}

	if _, err := service.ListArtifacts(canceledCtx, "dur-sess-petri-success-001"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ListArtifacts error = %v", err)
	}
	artifact, err := service.GetArtifact(context.Background(), "dur-sess-petri-success-001", "art-petri-final-001")
	if err != nil {
		t.Fatalf("GetArtifact detail: %v", err)
	}
	if artifact.ID != "art-petri-final-001" {
		t.Fatalf("artifact detail = %#v", artifact)
	}
	fallbackArtifactService := NewFakeService(WithFakeScenarios(FakeScenario{
		RequestID: "fallback-artifact",
		Session: SessionReadResult{
			SessionID: "dur-sess-art-fallback",
			Status:    LifecycleStatusSucceeded,
		},
		Result: ResultReadResult{
			SessionID:     "dur-sess-art-fallback",
			SessionStatus: LifecycleStatusSucceeded,
			ResultStatus:  ResultStatusFinal,
		},
		Artifacts: []ArtifactSummary{{ID: "art-fallback", Kind: "LOG", Visibility: "PUBLIC"}},
	}))
	startAsyncByRequestID(t, fallbackArtifactService, "fallback-artifact")
	fallbackArtifact, err := fallbackArtifactService.GetArtifact(context.Background(), "dur-sess-art-fallback", "art-fallback")
	if err != nil {
		t.Fatalf("GetArtifact fallback: %v", err)
	}
	if fallbackArtifact.ID != "art-fallback" {
		t.Fatalf("fallback artifact detail = %#v", fallbackArtifact)
	}
	if _, err := service.GetArtifact(context.Background(), "dur-sess-petri-success-001", "missing"); !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("missing GetArtifact error = %v", err)
	}

	if _, err := service.ReadEvents(canceledCtx, "dur-sess-petri-success-001", EventReconnectRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ReadEvents error = %v", err)
	}
	sequence := -1
	if _, err := service.ReadEvents(context.Background(), "dur-sess-petri-success-001", EventReconnectRequest{AfterSequence: &sequence}); err == nil {
		t.Fatal("negative reconnect sequence should fail")
	}

	if _, err := service.ListSessions(canceledCtx, ListSessionsRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ListSessions error = %v", err)
	}
	if _, err := service.ListSessions(context.Background(), ListSessionsRequest{Scope: SessionListScope("bad")}); err == nil {
		t.Fatal("invalid list sessions scope should fail")
	}
}

func TestFakeService_ReadEvents_ReturnsCanonicalFixtureEventsAndHonorsCursor(t *testing.T) {
	service := newContractFakeService(t)
	cases := []struct {
		requestID string
		sessionID string
		wantCount int
	}{
		{"req-js-run-n-001", "dur-sess-js-run-n-001", 2},
		{"req-js-success-002", "dur-sess-js-success-002", 3},
		{"req-js-awaiting-001", "dur-sess-js-awaiting-001", 2},
	}
	for _, tc := range cases {
		t.Run(tc.sessionID, func(t *testing.T) {
			startAsyncByRequestID(t, service, tc.requestID)
			all, err := service.ReadEvents(context.Background(), tc.sessionID, EventReconnectRequest{})
			if err != nil {
				t.Fatalf("ReadEvents: %v", err)
			}
			if len(all.Events) != tc.wantCount {
				t.Fatalf("events = %d, want %d", len(all.Events), tc.wantCount)
			}
			for index, raw := range all.Events {
				assertCanonicalEventEnvelope(t, raw, "", "")
				_ = index
			}

			afterStart, err := service.ReadEvents(context.Background(), tc.sessionID, EventReconnectRequest{
				AfterEventID: "session-started/" + tc.sessionID,
			})
			if err != nil {
				t.Fatalf("ReadEvents after start: %v", err)
			}
			if len(afterStart.Events) != tc.wantCount-1 {
				t.Fatalf("after start events = %d, want %d", len(afterStart.Events), tc.wantCount-1)
			}
		})
	}
}

func TestFakeService_ReadEvents_InvalidCursorReturnsError(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-js-run-n-001")
	_, err := service.ReadEvents(context.Background(), "dur-sess-js-run-n-001", EventReconnectRequest{
		AfterEventID: "missing-event-id",
	})
	if !errors.Is(err, ErrReconnectCursorNotFound) {
		t.Fatalf("error = %v, want ErrReconnectCursorNotFound", err)
	}
}

func TestFakeService_DerivedProjectionEvents_AreCanonicalWhenFixtureEventsMissing(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-run-001")
	events, err := service.ReadEvents(context.Background(), "dur-sess-petri-run-001", EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events.Events) == 0 {
		t.Fatal("derived events missing")
	}
	for _, raw := range events.Events {
		assertCanonicalEventEnvelope(t, raw, "", "")
	}
}

func TestFakeService_ListSessions_ScopedPersistedAndLive(t *testing.T) {
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-run-001")

	live, err := service.ListSessions(context.Background(), ListSessionsRequest{
		Scope: SessionListScopeLive,
	})
	if err != nil {
		t.Fatalf("ListSessions live: %v", err)
	}
	foundLiveRunning := false
	for _, session := range live.LiveSessions {
		if session.ID == "dur-sess-petri-run-001" {
			foundLiveRunning = true
			break
		}
	}
	if !foundLiveRunning {
		t.Fatalf("live sessions = %#v, want current-process running row", live.LiveSessions)
	}
	if len(live.DurableSessions) != 0 {
		t.Fatalf("live durable sessions = %#v, want none", live.DurableSessions)
	}

	persisted, err := service.ListSessions(context.Background(), ListSessionsRequest{
		Scope: SessionListScopePersisted,
	})
	if err != nil {
		t.Fatalf("ListSessions persisted: %v", err)
	}
	if len(persisted.DurableSessions) == 0 {
		t.Fatal("persisted sessions missing seeded terminal rows")
	}
	for _, summary := range persisted.DurableSessions {
		if summary.SessionID == "dur-sess-petri-run-001" {
			t.Fatalf("persisted scope unexpectedly contains running session %#v", summary)
		}
	}

	all, err := service.ListSessions(context.Background(), ListSessionsRequest{
		Scope: SessionListScopeAll,
	})
	if err != nil {
		t.Fatalf("ListSessions all: %v", err)
	}
	foundAllLiveRunning := false
	for _, session := range all.LiveSessions {
		if session.ID == "dur-sess-petri-run-001" {
			foundAllLiveRunning = true
			break
		}
	}
	if !foundAllLiveRunning {
		t.Fatalf("all live sessions = %#v, want current-process running row", all.LiveSessions)
	}
}

func TestFakeService_StartAsync_ConcurrentIdempotentStarts(t *testing.T) {
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
				Source:    Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
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
	scenario := BuiltinInterruptedRecoverableScenario()
	service := NewFakeService(WithFakeScenarios(
		FakeScenario{},
		scenario,
		FakeScenario{
			RequestID: "seeded-terminal",
			ListSummary: &DurableSessionListSummary{
				SessionID: "dur-sess-seeded-terminal",
				Status:    LifecycleStatusSucceeded,
			},
		},
	))
	if service == nil {
		t.Fatal("NewFakeService returned nil")
	}
	if _, ok := service.scenariosByRequestID[scenario.RequestID]; !ok {
		t.Fatal("scenario was not registered")
	}
	if len(service.persistedSeeds) != 2 {
		t.Fatalf("persistedSeeds = %#v", service.persistedSeeds)
	}

	loaded, err := NewFakeServiceFromContractFixtures(contractFixturesPath(t))
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

func int64Ptr(value int64) *int64 {
	return &value
}
func TestProjectResultRead_ModePartialAndFinal(t *testing.T) {
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

func TestFakeService_InternalLifecycleHelpers(t *testing.T) {
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

	if err := validateLifecycleControlRequest(LifecycleControlApprove, ControlRequest{}, ApproveRequest{}, RetryDispatchRequest{}); err != nil {
		t.Fatalf("approve validation: %v", err)
	}
	if err := validateLifecycleControlRequest(LifecycleControlRetryDispatch, ControlRequest{}, ApproveRequest{}, RetryDispatchRequest{}); err == nil {
		t.Fatal("retry without dispatch id should fail validation")
	}
	if err := validateLifecycleControlRequest(LifecycleControlPause, ControlRequest{}, ApproveRequest{}, RetryDispatchRequest{}); err != nil {
		t.Fatalf("pause validation: %v", err)
	}

	accepted := lifecycleControlResultFromState(state, "dur-sess-1", LifecycleControlApprove, LifecycleControlOutcomeAccepted, RetryDispatchRequest{})
	if accepted.Session == nil || accepted.Session.Status != LifecycleStatusAwaitingApproval {
		t.Fatalf("accepted lifecycle control result = %#v", accepted)
	}
	noop := lifecycleControlResultFromState(state, "dur-sess-1", LifecycleControlPause, LifecycleControlOutcomeNoOp, RetryDispatchRequest{})
	if noop.Session == nil {
		t.Fatalf("noop lifecycle control result = %#v", noop)
	}
	retry := lifecycleControlResultFromState(state, "dur-sess-1", LifecycleControlRetryDispatch, LifecycleControlOutcomeAccepted, RetryDispatchRequest{DispatchID: "disp-1"})
	if retry.DispatchID != "disp-1" || retry.RetryDispatchID != "disp-1" {
		t.Fatalf("retry lifecycle control result = %#v", retry)
	}

	service := &FakeService{}
	service.mutateSessionForControl(state, LifecycleControlApprove, "")
	if state.session.Status != LifecycleStatusRunning {
		t.Fatalf("approve mutate status = %q, want RUNNING", state.session.Status)
	}
	state.session.Status = LifecycleStatusFailed
	state.result.SessionStatus = LifecycleStatusFailed
	service.mutateSessionForControl(state, LifecycleControlRetryDispatch, "disp-1")
	if state.session.Status != LifecycleStatusRunning || state.dispatches[0].Status != DispatchStatusQueued || state.dispatches[0].Attempt != 2 {
		t.Fatalf("retry mutate state = %#v / %#v", state.session, state.dispatches[0])
	}
}

func TestFakeService_InternalStartAndProjectionHelpers(t *testing.T) {
	state := &fakeSessionState{
		session: SessionReadResult{
			SessionID:        "dur-sess-1",
			Status:           LifecycleStatusSucceeded,
			OrchestratorKind: "JAVASCRIPT",
			Dialect:          "v1",
			ResolvedSource:   ResolvedSource{Kind: workflowsource.KindWorkflowName, SourceRef: "audit", SourceHash: "hash", Dialect: "v1"},
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

	service := &FakeService{}
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

	if cloneFailureSummary(nil) != nil || cloneResultAvailability(nil) != nil || cloneRawJSON(nil) != nil {
		t.Fatal("nil clones should stay nil")
	}
	if clone := cloneFailureSummary(canonical.Failure); clone == canonical.Failure || clone.Reason != "warn" {
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
	cases := []struct {
		operation LifecycleControlKind
		status    LifecycleStatus
		want      LifecycleControlOutcome
	}{
		{LifecycleControlPause, LifecycleStatusRunning, LifecycleControlOutcomeAccepted},
		{LifecycleControlPause, LifecycleStatusPaused, LifecycleControlOutcomeNoOp},
		{LifecycleControlResume, LifecycleStatusPaused, LifecycleControlOutcomeAccepted},
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
	retry := RetryDispatchRequest{
		ControlRequest: ControlRequest{RequestID: "req-retry-001"},
		DispatchID:     "disp-js-success-002",
	}
	first, err := ControlIdempotencyTupleHash(LifecycleControlRetryDispatch, "dur-sess-js-success-002", ApproveRequest{}, retry)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	second, err := ControlIdempotencyTupleHash(LifecycleControlRetryDispatch, "dur-sess-js-success-002", ApproveRequest{}, retry)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if first != second {
		t.Fatalf("hash mismatch: %q vs %q", first, second)
	}
}

func TestCheckControlRequestIDReplay_Conflict(t *testing.T) {
	err := CheckControlRequestIDReplay("req-1", "sha256:abc", "sha256:def")
	if !errors.Is(err, ErrControlRequestIDConflict) {
		t.Fatalf("error = %v, want ErrControlRequestIDConflict", err)
	}
}

func TestServiceMethods_PropagateContextCancellation(t *testing.T) {
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

func TestJavaScriptRuntimeService_StartSync_SimpleWorkflowCompletesWithPrimaryResult(t *testing.T) {
	service := newJavaScriptRuntimeService(t)

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

	session, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Status != LifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", session.Status)
	}
	if session.ResultSummary == nil || session.ResultSummary.ResultStatus != string(ResultStatusFinal) {
		t.Fatalf("resultSummary = %#v, want FINAL", session.ResultSummary)
	}

	result, err := service.GetResult(context.Background(), started.SessionID, ResultRequest{Mode: ResultModeFinal})
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

	events, err := service.ReadEvents(context.Background(), started.SessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events.Events) != 3 {
		t.Fatalf("events = %d, want 3 canonical lifecycle events", len(events.Events))
	}
}

func TestJavaScriptRuntimeService_StartAsync_RunningCancelAndReads(t *testing.T) {
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
	service := newJavaScriptRuntimeService(t)
	waitMillis := int64(50)

	started, err := service.StartSync(context.Background(), StartRequest{
		RequestID: "req-runtime-sync-wait-timeout-001",
		Source: Source{
			Kind: workflowsource.KindInlineWorkflow,
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
	fakeService, err := NewExecutionService(ExecutionProviderFake, ServiceConfig{})
	if err != nil {
		t.Fatalf("NewExecutionService(fake): %v", err)
	}
	if _, ok := fakeService.(*FakeService); !ok {
		t.Fatalf("fake provider type = %T, want *FakeService", fakeService)
	}

	projectRoot := t.TempDir()
	runtimeService, err := NewExecutionService(ExecutionProviderJavaScriptRuntime, ServiceConfig{ProjectRoot: projectRoot})
	if err != nil {
		t.Fatalf("NewExecutionService(runtime): %v", err)
	}
	jsService, ok := runtimeService.(*JavaScriptRuntimeService)
	if !ok {
		t.Fatalf("runtime provider type = %T, want *JavaScriptRuntimeService", runtimeService)
	}
	if jsService.sessionPersistDir == "" {
		t.Fatal("expected runtime service to enable persisted session dir when project root is set")
	}

	if _, err := NewExecutionService(ExecutionProviderJavaScriptRuntime, ServiceConfig{}); err == nil {
		t.Fatal("NewExecutionService(runtime without projectRoot) error = nil, want validation error")
	}
	if _, err := NewExecutionService(ExecutionProvider("unknown"), ServiceConfig{}); err == nil {
		t.Fatal("NewExecutionService(unknown) error = nil, want validation error")
	}
	if err := validateLiveChildExecutorConfig(ChildExecutorModeLive, nil); err == nil {
		t.Fatal("validateLiveChildExecutorConfig(live,nil) error = nil, want validation error")
	}
	if err := validateLiveChildExecutorConfig(ChildExecutorModeFake, nil); err != nil {
		t.Fatalf("validateLiveChildExecutorConfig(fake,nil) error = %v", err)
	}

	smoke := SmokeLiveChildProvider()
	response, err := smoke.Infer(context.Background(), interfaces.ProviderInferenceRequest{})
	if err != nil {
		t.Fatalf("SmokeLiveChildProvider().Infer: %v", err)
	}
	if response.ProviderSession == nil || response.ProviderSession.ID == "" {
		t.Fatalf("provider session = %#v, want stable session metadata", response.ProviderSession)
	}

	inlineReq := startSourceRequest(Source{
		Kind: workflowsource.KindInlineWorkflow,
		InlineWorkflow: &InlineWorkflowSource{
			InlineSource: "return 1;",
		},
	})
	if inlineReq.Value != "return 1;" || inlineReq.InlineSource != "return 1;" {
		t.Fatalf("startSourceRequest(inline) = %#v", inlineReq)
	}
	if resolutionOrderForLookupStage(workflowsource.LookupStageProjectClaude) != "PROJECT_CLAUDE_WORKFLOWS" {
		t.Fatal("unexpected lookup stage mapping for project claude")
	}
	if resolutionOrderForLookupStage(workflowsource.LookupStageNamedJavaScript) != "BUILTIN_GLOBAL_JAVASCRIPT_FACTORIES" {
		t.Fatal("unexpected lookup stage mapping for named javascript")
	}
	if resolutionOrderForLookupStage("unknown") != "" {
		t.Fatal("unexpected lookup stage mapping for unknown stage")
	}
}

func TestNormalizationAndIdempotencyHelpers(t *testing.T) {
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
		Kind: workflowsource.KindInlineWorkflow,
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

	if _, err := normalizeSourceForIdempotency(Source{Kind: workflowsource.KindInlineWorkflow}); err == nil {
		t.Fatal("normalizeSourceForIdempotency(missing inline) error = nil, want validation error")
	}

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
	projectRoot := writeSimpleFinalWorkflowProject(t)
	service := NewJavaScriptRuntimeService(JavaScriptRuntimeServiceConfig{
		ProjectRoot:     projectRoot,
		PersistSessions: true,
	})

	prepared, err := service.prepareStart(StartRequest{
		RequestID: "req-prepare-start-001",
		Source: Source{
			Kind:         workflowsource.KindWorkflowName,
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
	sessionID := NewDurableSessionID()
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

func newJavaScriptRuntimeService(t *testing.T) *JavaScriptRuntimeService {
	t.Helper()

	service, err := NewExecutionService(
		ExecutionProviderJavaScriptRuntime,
		ServiceConfig{ProjectRoot: t.TempDir()},
	)
	if err != nil {
		t.Fatalf("NewExecutionService: %v", err)
	}
	jsService, ok := service.(*JavaScriptRuntimeService)
	if !ok {
		t.Fatalf("service type = %T, want *JavaScriptRuntimeService", service)
	}
	return jsService
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
			Kind: workflowsource.KindInlineWorkflow,
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

func TestFakeService_DetailReadersAndRemainingControlWrappers(t *testing.T) {
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
	now := time.Now().UTC()
	service := NewJavaScriptRuntimeService(JavaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	service.sessions["dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"] = &runtimeSessionState{
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
	normalized, err := NormalizeStartRequest(StartRequest{
		RequestID: " req-1 ",
		Source: Source{
			Kind:          workflowsource.KindFactoryInline,
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
			Kind:         workflowsource.KindWorkflowFile,
			WorkflowFile: " path/to/workflow.js ",
		},
		Orchestrator: &OrchestratorOverride{
			Raw: json.RawMessage("{"),
		},
	}); err == nil {
		t.Fatal("NormalizeStartRequest(invalid orchestrator) error = nil, want validation error")
	}

	if _, err := normalizeSource(Source{Kind: workflowsource.KindWorkflowName, WorkflowName: "  "}); err == nil {
		t.Fatal("normalizeSource(empty workflow name) error = nil, want validation error")
	}
	if got := normalizeChildExecutorMode(" live-provider "); got != ChildExecutorModeLive {
		t.Fatalf("normalizeChildExecutorMode = %q, want live", got)
	}
	if got := resolveChildExecutorMode("fake", StartRequest{Runtime: &RuntimeOptions{ChildExecutorMode: "live-provider"}}); got != ChildExecutorModeLive {
		t.Fatalf("resolveChildExecutorMode = %q, want live override", got)
	}

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
	service := NewJavaScriptRuntimeService(JavaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	if hooks := service.childExecutorHooks(ChildExecutorModeFake); hooks.NewChildExecutor != nil {
		t.Fatalf("fake hooks = %#v, want no child executor override", hooks)
	}
	liveService := NewJavaScriptRuntimeService(JavaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Provider:    SmokeLiveChildProvider(),
	})
	if hooks := liveService.childExecutorHooks(ChildExecutorModeLive); hooks.NewChildExecutor == nil {
		t.Fatal("expected live child executor hook")
	}

	if raw, err := marshalStartArgs(nil); err != nil || raw != nil {
		t.Fatalf("marshalStartArgs(nil) = %q, %v, want nil,nil", raw, err)
	}
	if _, err := marshalStartArgs(map[string]any{"bad": func() {}}); err == nil {
		t.Fatal("marshalStartArgs(non-json) error = nil, want validation error")
	}

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
	if err := validationErrorFromSourceIssues([]workflowvalidation.Issue{{Message: "bad source", Line: 3, Column: 5}}); err == nil || err.Error() != "bad source (line 3, column 5)" {
		t.Fatalf("validationErrorFromSourceIssues(location) = %v", err)
	}
	if err := validationErrorFromSourceIssues([]workflowvalidation.Issue{{}}); err == nil || err.Error() != "workflow source validation failed" {
		t.Fatalf("validationErrorFromSourceIssues(default message) = %v", err)
	}
	if err := validationErrorFromPolicyIssues(nil); err != nil {
		t.Fatalf("validationErrorFromPolicyIssues(nil) = %v, want nil", err)
	}
	if err := validationErrorFromPolicyIssues([]workflowpolicy.Issue{{Message: "blocked"}}); err == nil || err.Error() != "blocked" {
		t.Fatalf("validationErrorFromPolicyIssues = %v, want blocked", err)
	}
	if err := validationErrorFromPolicyIssues([]workflowpolicy.Issue{{}}); err == nil || err.Error() != "requested policy is invalid" {
		t.Fatalf("validationErrorFromPolicyIssues(default message) = %v", err)
	}
}

func TestStartSourceRequestAndResolutionOrderBranches(t *testing.T) {
	cases := []struct {
		source Source
		want   string
	}{
		{Source{Kind: workflowsource.KindFactoryID, FactoryID: "factory-1"}, "factory-1"},
		{Source{Kind: workflowsource.KindFactoryInline, FactoryInline: json.RawMessage(`{"name":"factory"}`)}, `{"name":"factory"}`},
		{Source{Kind: workflowsource.KindWorkflowFile, WorkflowFile: "wf.js"}, "wf.js"},
		{Source{Kind: workflowsource.KindWorkflowName, WorkflowName: "name"}, "name"},
	}
	for _, tc := range cases {
		if got := startSourceRequest(tc.source); got.Value != tc.want {
			t.Fatalf("startSourceRequest(%s) value = %q, want %q", tc.source.Kind, got.Value, tc.want)
		}
	}
	if got := startSourceRequest(Source{Kind: workflowsource.KindInlineWorkflow}); got.Value != "" || got.InlineSource != "" {
		t.Fatalf("startSourceRequest(missing inline) = %#v, want empty inline request", got)
	}

	stages := []workflowsource.LookupStage{
		workflowsource.LookupStageProjectClaude,
		workflowsource.LookupStageExplicitSourceKind,
		workflowsource.LookupStageGlobalUser,
		workflowsource.LookupStagePackageRelative,
		workflowsource.LookupStageNamedJavaScript,
		workflowsource.LookupStageExplicitFactory,
	}
	for _, stage := range stages {
		if resolutionOrderForLookupStage(stage) == "" {
			t.Fatalf("resolutionOrderForLookupStage(%q) returned empty mapping", stage)
		}
	}
}

func TestJavaScriptRuntimeService_ReplayAndReadErrorBranches(t *testing.T) {
	service := newJavaScriptRuntimeService(t)
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
	if err := (&JavaScriptRuntimeService{}).persistTerminalSessionState(runtimeSessionState{}); err != nil {
		t.Fatalf("persistTerminalSessionState(no dir) = %v, want nil", err)
	}

	service := NewJavaScriptRuntimeService(JavaScriptRuntimeServiceConfig{
		ProjectRoot:     t.TempDir(),
		PersistSessions: true,
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
	cases := []StartRequest{
		{
			RequestID: "req-file",
			Source: Source{
				Kind:         workflowsource.KindWorkflowFile,
				WorkflowFile: " workflow.js ",
			},
		},
		{
			RequestID: "req-name",
			Source: Source{
				Kind:         workflowsource.KindWorkflowName,
				WorkflowName: " named-workflow ",
			},
		},
		{
			RequestID: "req-inline",
			Source: Source{
				Kind: workflowsource.KindInlineWorkflow,
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
	now := time.Now().UTC()
	later := now.Add(2 * time.Hour)
	summary := DurableSessionListSummary{
		SessionID:        "dur-sess-filter-1",
		Status:           LifecycleStatusRunning,
		OrchestratorKind: "JAVASCRIPT",
		ResolvedSource: ResolvedSource{
			Kind:      workflowsource.KindWorkflowName,
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
		SourceKind:        workflowsource.KindWorkflowName,
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
			SourceKind:        workflowsource.KindWorkflowName,
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
			SourceKind:    workflowsource.Kind("unknown"),
			CreatedAfter:  &before,
			CreatedBefore: &after,
		},
	}); err == nil {
		t.Fatal("NormalizeListSessionsRequest(invalid filters) error = nil, want validation error")
	}
}

func TestSmallHelperBranches(t *testing.T) {
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
	observedAt := time.Now().UTC()
	artifact := artifactSummaryFromRuntimeRecord("dur-sess-helper-1", workflowruntime.ArtifactRecord{
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
