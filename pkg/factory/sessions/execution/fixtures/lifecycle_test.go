package fixtures_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	fse "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/fixtures"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestFakeService_PublishedScenarios_LifecycleControlPauseResumeOutcomes(t *testing.T) {
	service := newContractFakeService(t)

	pausedRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeLifecycleControl)
	startPublishedScenario(t, service, pausedRow)

	pauseNoOp, err := service.Pause(context.Background(), pausedRow.SessionID, fse.ControlRequest{})
	if err != nil {
		t.Fatalf("fse.Pause on paused session: %v", err)
	}
	if pauseNoOp.Outcome != fse.LifecycleControlOutcomeNoOp || pauseNoOp.Status != fse.LifecycleStatusPaused {
		t.Fatalf("pause on paused = %#v, want NO_OP/PAUSED", pauseNoOp)
	}
	pauseNoOpHash, err := fixtures.LifecycleControlResultHash(pauseNoOp)
	if err != nil {
		t.Fatalf("fixtures.LifecycleControlResultHash pause no-op: %v", err)
	}
	if pauseNoOpHash != "sha256:dff882b64856e2bf56d03e29643fc65abe7129532ff38615e1033ae39873df7c" {
		t.Fatalf("pause no-op hash = %q", pauseNoOpHash)
	}

	resumed, err := service.Resume(context.Background(), pausedRow.SessionID, fse.ControlRequest{})
	if err != nil {
		t.Fatalf("fse.Resume paused session: %v", err)
	}
	if resumed.Outcome != fse.LifecycleControlOutcomeAccepted || resumed.Status != fse.LifecycleStatusRunning {
		t.Fatalf("resume = %#v, want ACCEPTED/RUNNING", resumed)
	}
	resumeHash, err := fixtures.LifecycleControlResultHash(resumed)
	if err != nil {
		t.Fatalf("fixtures.LifecycleControlResultHash resume: %v", err)
	}
	if resumeHash != "sha256:c12be84234b44996999436577f3967f4bccfc9b5be1d9ad179146b064d56df5a" {
		t.Fatalf("resume hash = %q", resumeHash)
	}

	runningRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeAsyncRunning)
	startPublishedScenario(t, service, runningRow)

	pauseAccepted, err := service.Pause(context.Background(), runningRow.SessionID, fse.ControlRequest{})
	if err != nil {
		t.Fatalf("fse.Pause running session: %v", err)
	}
	if pauseAccepted.Outcome != fse.LifecycleControlOutcomeAccepted || pauseAccepted.Status != fse.LifecycleStatusPaused {
		t.Fatalf("pause running = %#v, want ACCEPTED/PAUSED", pauseAccepted)
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlCancelTerminateOutcomes(t *testing.T) {
	service := newContractFakeService(t)
	runningRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeAsyncRunning)
	startPublishedScenario(t, service, runningRow)

	canceled, err := service.Cancel(context.Background(), runningRow.SessionID, fse.ControlRequest{})
	if err != nil {
		t.Fatalf("fse.Cancel: %v", err)
	}
	if canceled.Outcome != fse.LifecycleControlOutcomeAccepted || canceled.Status != fse.LifecycleStatusCanceling {
		t.Fatalf("cancel = %#v, want ACCEPTED/CANCELING", canceled)
	}

	service = newContractFakeService(t)
	startPublishedScenario(t, service, runningRow)
	terminated, err := service.Terminate(context.Background(), runningRow.SessionID, fse.ControlRequest{})
	if err != nil {
		t.Fatalf("fse.Terminate: %v", err)
	}
	if terminated.Outcome != fse.LifecycleControlOutcomeAccepted || terminated.Status != fse.LifecycleStatusTerminated {
		t.Fatalf("terminate = %#v, want ACCEPTED/TERMINATED", terminated)
	}

	terminalRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeSyncSuccess)
	startPublishedScenario(t, service, terminalRow)
	_, err = service.Cancel(context.Background(), terminalRow.SessionID, fse.ControlRequest{})
	var controlErr *fse.ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != fse.LifecycleControlOutcomeTerminalSession {
		t.Fatalf("cancel on terminal = %v, want TERMINAL_SESSION fse.ControlError", err)
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlApproveAwaitingApproval(t *testing.T) {
	service := newContractFakeService(t)
	startAwaitingApprovalSession(t, service)

	_, err := service.Pause(context.Background(), "dur-sess-js-awaiting-001", fse.ControlRequest{})
	var invalidErr *fse.ControlError
	if !errors.As(err, &invalidErr) || invalidErr.Outcome != fse.LifecycleControlOutcomeInvalidState {
		t.Fatalf("pause on awaiting approval = %v, want INVALID_STATE fse.ControlError", err)
	}

	approved, err := service.Approve(context.Background(), "dur-sess-js-awaiting-001", fse.ApproveRequest{
		ControlRequest: fse.ControlRequest{RequestID: "ctrl-approve-001"},
		ApprovedPolicy: map[string]any{"maxAgents": 2},
	})
	if err != nil {
		t.Fatalf("fse.Approve: %v", err)
	}
	if approved.Outcome != fse.LifecycleControlOutcomeAccepted || approved.Status != fse.LifecycleStatusRunning {
		t.Fatalf("approve = %#v, want ACCEPTED/RUNNING", approved)
	}
	approveHash, err := fixtures.LifecycleControlResultHash(approved)
	if err != nil {
		t.Fatalf("fixtures.LifecycleControlResultHash approve: %v", err)
	}
	if approveHash != "sha256:100080e8a28d1703922847890405ca20f554419b8e6d2f3690b227c845447633" {
		t.Fatalf("approve hash = %q", approveHash)
	}
	if approved.Links.Results != "/factory-sessions/dur-sess-js-awaiting-001/results" {
		t.Fatalf("approve links = %#v", approved.Links)
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlRetryDispatchPaths(t *testing.T) {
	service := newContractFakeService(t)

	terminalRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeSyncSuccess)
	startPublishedScenario(t, service, terminalRow)
	_, err := service.RetryDispatch(context.Background(), terminalRow.SessionID, fse.RetryDispatchRequest{
		ControlRequest: fse.ControlRequest{},
		DispatchID:     "disp-petri-success-001",
	})
	var terminalErr *fse.ControlError
	if !errors.As(err, &terminalErr) || terminalErr.Outcome != fse.LifecycleControlOutcomeTerminalSession {
		t.Fatalf("retry on terminal = %v, want TERMINAL_SESSION fse.ControlError", err)
	}

	recoverableRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeFailedRecoverable)
	startPublishedScenario(t, service, recoverableRow)
	_, err = service.RetryDispatch(context.Background(), recoverableRow.SessionID, fse.RetryDispatchRequest{
		ControlRequest: fse.ControlRequest{},
		DispatchID:     "disp-js-interrupted-002",
	})
	if !errors.As(err, &terminalErr) || terminalErr.Outcome != fse.LifecycleControlOutcomeTerminalSession {
		t.Fatalf("retry on interrupted = %v, want TERMINAL_SESSION fse.ControlError", err)
	}

	startFailedPartialSession(t, service)
	retry, err := service.RetryDispatch(context.Background(), "dur-sess-js-failed-partial-001", fse.RetryDispatchRequest{
		ControlRequest: fse.ControlRequest{RequestID: "ctrl-retry-fail-001"},
		DispatchID:     "disp-js-fail-002",
	})
	if err != nil {
		t.Fatalf("fse.RetryDispatch failed partial: %v", err)
	}
	if retry.Outcome != fse.LifecycleControlOutcomeAccepted || retry.Status != fse.LifecycleStatusRunning {
		t.Fatalf("retry = %#v, want ACCEPTED/RUNNING", retry)
	}
	if retry.RetryDispatchID != "disp-js-fail-002" {
		t.Fatalf("retryDispatchId = %q", retry.RetryDispatchID)
	}
	retryHash, err := fixtures.LifecycleControlResultHash(retry)
	if err != nil {
		t.Fatalf("fixtures.LifecycleControlResultHash retry: %v", err)
	}
	if retryHash != "sha256:ff4b53b67a11b90eeb9a667c68dd206cb2156265067325eb150b31877882852b" {
		t.Fatalf("retry hash = %q", retryHash)
	}

	dispatches, err := service.ListDispatches(context.Background(), "dur-sess-js-failed-partial-001")
	if err != nil {
		t.Fatalf("fse.ListDispatches: %v", err)
	}
	for _, dispatch := range dispatches.Dispatches {
		if dispatch.ID == "disp-js-fail-002" && dispatch.Status != fse.DispatchStatusQueued {
			t.Fatalf("retried dispatch = %#v, want QUEUED", dispatch)
		}
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlAcceptedInspectionLinks(t *testing.T) {
	service := newContractFakeService(t)
	row := publishedScenarioByPurpose(t, fixtures.FixturePurposeAsyncRunning)
	startPublishedScenario(t, service, row)

	paused, err := service.Pause(context.Background(), row.SessionID, fse.ControlRequest{})
	if err != nil {
		t.Fatalf("fse.Pause: %v", err)
	}
	want := fse.LifecycleControlLinksForSession(row.SessionID, true)
	if paused.Links != want {
		t.Fatalf("links = %#v, want %#v", paused.Links, want)
	}
	if paused.Session == nil || paused.Session.Status != fse.LifecycleStatusPaused {
		t.Fatalf("session projection = %#v", paused.Session)
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlIdempotentReplayAndConflict(t *testing.T) {
	service := newContractFakeService(t)
	row := publishedScenarioByPurpose(t, fixtures.FixturePurposeAsyncRunning)
	startPublishedScenario(t, service, row)

	first, err := service.Pause(context.Background(), row.SessionID, fse.ControlRequest{
		RequestID: "ctrl-lifecycle-replay-001",
	})
	if err != nil {
		t.Fatalf("first fse.Pause: %v", err)
	}
	second, err := service.Pause(context.Background(), row.SessionID, fse.ControlRequest{
		RequestID: "ctrl-lifecycle-replay-001",
	})
	if err != nil {
		t.Fatalf("replay fse.Pause: %v", err)
	}
	firstHash, err := fixtures.LifecycleControlResultHash(first)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	secondHash, err := fixtures.LifecycleControlResultHash(second)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if firstHash != secondHash {
		t.Fatalf("replay hash drift: %q vs %q", firstHash, secondHash)
	}

	_, err = service.Resume(context.Background(), row.SessionID, fse.ControlRequest{
		RequestID: "ctrl-lifecycle-replay-001",
	})
	var controlErr *fse.ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != fse.LifecycleControlOutcomeConflict {
		t.Fatalf("conflict error = %v, want CONFLICT fse.ControlError", err)
	}
	if controlErr.Status != fse.LifecycleStatusPaused {
		t.Fatalf("conflict status = %q, want PAUSED", controlErr.Status)
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlIsolationAcrossSessions(t *testing.T) {
	service := newContractFakeService(t)
	pausedRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeLifecycleControl)
	runningRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeAsyncRunning)
	startPublishedScenario(t, service, pausedRow)
	startPublishedScenario(t, service, runningRow)

	beforePaused, err := service.GetSession(context.Background(), pausedRow.SessionID)
	if err != nil {
		t.Fatalf("fse.GetSession paused before: %v", err)
	}
	if beforePaused.Status != fse.LifecycleStatusPaused {
		t.Fatalf("paused status before = %q", beforePaused.Status)
	}

	if _, err := service.Terminate(context.Background(), runningRow.SessionID, fse.ControlRequest{}); err != nil {
		t.Fatalf("fse.Terminate running: %v", err)
	}

	afterPaused, err := service.GetSession(context.Background(), pausedRow.SessionID)
	if err != nil {
		t.Fatalf("fse.GetSession paused after: %v", err)
	}
	if afterPaused.Status != fse.LifecycleStatusPaused {
		t.Fatalf("paused status after = %q, want PAUSED unchanged", afterPaused.Status)
	}
	pausedHash, err := fixtures.LifecycleControlResultHash(fse.LifecycleControlResult{
		SessionID: afterPaused.SessionID,
		Operation: fse.LifecycleControlPause,
		Outcome:   fse.LifecycleControlOutcomeNoOp,
		Status:    afterPaused.Status,
		Links:     fse.LifecycleControlLinksForSession(afterPaused.SessionID, true),
	})
	if err != nil {
		t.Fatalf("fixtures.LifecycleControlResultHash: %v", err)
	}
	if pausedHash != "sha256:dff882b64856e2bf56d03e29643fc65abe7129532ff38615e1033ae39873df7c" {
		t.Fatalf("isolation hash = %q", pausedHash)
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlDeterministicAcrossServiceReload(t *testing.T) {
	row := publishedScenarioByPurpose(t, fixtures.FixturePurposeLifecycleControl)

	runControl := func(t *testing.T) string {
		t.Helper()
		service := newContractFakeService(t)
		startPublishedScenario(t, service, row)
		resumed, err := service.Resume(context.Background(), row.SessionID, fse.ControlRequest{})
		if err != nil {
			t.Fatalf("fse.Resume: %v", err)
		}
		hash, err := fixtures.LifecycleControlResultHash(resumed)
		if err != nil {
			t.Fatalf("fixtures.LifecycleControlResultHash: %v", err)
		}
		return hash
	}

	first := runControl(t)
	second := runControl(t)
	if first != second {
		t.Fatalf("reload hash drift: %q vs %q", first, second)
	}
	if first != "sha256:c12be84234b44996999436577f3967f4bccfc9b5be1d9ad179146b064d56df5a" {
		t.Fatalf("reload resume hash = %q", first)
	}
}
func TestJavaScriptRuntimeService_CancelRunningSessionReturnsCanceling(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")

	started, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-cancel-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	canceled, err := service.Cancel(context.Background(), started.SessionID, fse.ControlRequest{})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if canceled.Outcome != fse.LifecycleControlOutcomeAccepted ||
		canceled.Status != fse.LifecycleStatusCanceling {
		t.Fatalf("cancel = %#v, want ACCEPTED/CANCELING", canceled)
	}
	if canceled.Operation != fse.LifecycleControlCancel {
		t.Fatalf("operation = %q, want CANCEL", canceled.Operation)
	}
	if canceled.Links.Results == "" || canceled.Links.Session == "" {
		t.Fatalf("links = %#v, want inspection links", canceled.Links)
	}
}

func TestJavaScriptRuntimeService_TerminateRunningSessionReturnsTerminated(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")

	started, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-terminate-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	terminated, err := service.Terminate(context.Background(), started.SessionID, fse.ControlRequest{})
	if err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if terminated.Outcome != fse.LifecycleControlOutcomeAccepted ||
		terminated.Status != fse.LifecycleStatusTerminated {
		t.Fatalf("terminate = %#v, want ACCEPTED/TERMINATED", terminated)
	}
}

func TestJavaScriptRuntimeService_CancelTerminalSessionReturnsTypedControlError(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")

	started, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-cancel-terminal-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   2,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		read, readErr := service.GetSession(context.Background(), started.SessionID)
		if readErr != nil {
			t.Fatalf("GetSession: %v", readErr)
		}
		if read.Status == fse.LifecycleStatusSucceeded {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	_, err = service.Cancel(context.Background(), started.SessionID, fse.ControlRequest{})
	var controlErr *fse.ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != fse.LifecycleControlOutcomeTerminalSession {
		t.Fatalf("cancel on terminal = %v, want TERMINAL_SESSION ControlError", err)
	}
}

func TestJavaScriptRuntimeService_CancelMissingSessionReturnsNotFound(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")
	_, err := service.Cancel(context.Background(), "dur-sess-missing-999", fse.ControlRequest{})
	if !errors.Is(err, fse.ErrSessionNotFound) {
		t.Fatalf("cancel missing = %v, want ErrSessionNotFound", err)
	}
}

func TestJavaScriptRuntimeService_PauseRunningSessionReturnsPaused(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")

	started, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-pause-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	paused, err := service.Pause(context.Background(), started.SessionID, fse.ControlRequest{})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.Outcome != fse.LifecycleControlOutcomeAccepted ||
		paused.Status != fse.LifecycleStatusPaused {
		t.Fatalf("pause = %#v, want ACCEPTED/PAUSED", paused)
	}
	if paused.Operation != fse.LifecycleControlPause {
		t.Fatalf("operation = %q, want PAUSE", paused.Operation)
	}
}

func TestJavaScriptRuntimeService_ResumePausedSessionReturnsRunning(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")

	started, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-resume-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	if _, err := service.Pause(context.Background(), started.SessionID, fse.ControlRequest{}); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	resumed, err := service.Resume(context.Background(), started.SessionID, fse.ControlRequest{})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if resumed.Outcome != fse.LifecycleControlOutcomeAccepted ||
		resumed.Status != fse.LifecycleStatusRunning {
		t.Fatalf("resume = %#v, want ACCEPTED/RUNNING", resumed)
	}
}

func TestJavaScriptRuntimeService_PauseTerminalSessionReturnsTypedControlError(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")

	started, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-pause-terminal-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   2,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		read, readErr := service.GetSession(context.Background(), started.SessionID)
		if readErr != nil {
			t.Fatalf("GetSession: %v", readErr)
		}
		if read.Status == fse.LifecycleStatusSucceeded {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	_, err = service.Pause(context.Background(), started.SessionID, fse.ControlRequest{})
	var controlErr *fse.ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != fse.LifecycleControlOutcomeTerminalSession {
		t.Fatalf("pause on terminal = %v, want TERMINAL_SESSION ControlError", err)
	}
}

func TestJavaScriptRuntimeService_ApproveRunningSessionReturnsTypedControlError(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")

	started, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-approve-invalid-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	_, err = service.Approve(context.Background(), started.SessionID, fse.ApproveRequest{})
	var controlErr *fse.ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != fse.LifecycleControlOutcomeInvalidState {
		t.Fatalf("approve on running = %v, want INVALID_STATE ControlError", err)
	}
}

func TestJavaScriptRuntimeService_RetryDispatchMissingDispatchReturnsNotFound(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")

	started, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-retry-missing-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	_, err = service.RetryDispatch(context.Background(), started.SessionID, fse.RetryDispatchRequest{
		DispatchID: "disp-missing-001",
	})
	if !errors.Is(err, fse.ErrDispatchNotFound) {
		t.Fatalf("retry missing dispatch = %v, want ErrDispatchNotFound", err)
	}
}

func TestJavaScriptRuntimeService_ControlIdempotentReplayAndConflict(t *testing.T) {
	service := newJavaScriptRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")

	started, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-control-replay-start-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	first, err := service.Pause(context.Background(), started.SessionID, fse.ControlRequest{
		RequestID: "ctrl-runtime-replay-001",
	})
	if err != nil {
		t.Fatalf("first Pause: %v", err)
	}
	second, err := service.Pause(context.Background(), started.SessionID, fse.ControlRequest{
		RequestID: "ctrl-runtime-replay-001",
	})
	if err != nil {
		t.Fatalf("replay Pause: %v", err)
	}
	if first.Outcome != second.Outcome || first.Status != second.Status || first.Operation != second.Operation {
		t.Fatalf("replay drift: first=%#v second=%#v", first, second)
	}

	read, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession after replay: %v", err)
	}
	if read.Status != fse.LifecycleStatusPaused {
		t.Fatalf("status after replay = %q, want PAUSED", read.Status)
	}

	_, err = service.Resume(context.Background(), started.SessionID, fse.ControlRequest{
		RequestID: "ctrl-runtime-replay-001",
	})
	var controlErr *fse.ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != fse.LifecycleControlOutcomeConflict {
		t.Fatalf("conflict = %v, want CONFLICT ControlError", err)
	}
	if controlErr.Status != fse.LifecycleStatusPaused {
		t.Fatalf("conflict status = %q, want PAUSED unchanged", controlErr.Status)
	}
}

func TestBuildCanonicalRuntimeSessionEvents_ProjectsLiveProviderDispatchLifecycle(t *testing.T) {
	startedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(2 * time.Second)
	sessionID := "dur-sess-dispatch-events-001"
	session := fse.SessionReadResult{
		SessionID:        sessionID,
		Status:           fse.LifecycleStatusSucceeded,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Dialect:          "you-workflow-v1",
		Phase:            "execute",
		ResolvedSource:   fse.ResolvedSource{SourceRef: "workflow/agent-run-fake-child"},
		SourceHash:       "sha256:fixture",
		Policy:           fse.PolicyProjection{EffectiveHash: "sha256:policy"},
		ResultSummary: &fse.ResultSummary{
			ResultStatus: string(fse.ResultStatusFinal),
		},
		Lifecycle: &fse.LifecycleTimestamps{
			StartedAt:  &startedAt,
			FinishedAt: &finishedAt,
		},
	}
	result := fse.ResultReadResult{
		SessionID:     sessionID,
		ResultStatus:  fse.ResultStatusFinal,
		SessionStatus: fse.LifecycleStatusSucceeded,
	}
	dispatch := fse.DispatchSummary{
		ID:              "dispatch-1",
		Status:          fse.DispatchStatusCompleted,
		DispatchKind:    "JAVASCRIPT_AGENT",
		Phase:           "execute",
		Label:           "summarize findings",
		Provider:        "mock",
		PresetID:        "careful-review",
		ModelProvider:   "CODEX",
		Model:           "gpt-test",
		ReasoningEffort: "high",
		RunnerID:        "review",
		ProviderSessionRefs: []fse.ProviderSessionRef{{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "live-provider-session-1",
		}},
		OutputArtifactIDs: []string{"child-artifact-1"},
		JavaScript: &fse.DispatchJavaScriptProjection{
			TaskKind:      "AGENT",
			TaskLabel:     "summarize findings",
			ExecutionMode: fse.ChildExecutorModeLive,
		},
	}

	events := fse.BuildCanonicalRuntimeSessionEvents(session, result, fse.RuntimeDispatchEventInput{
		Dispatches: []fse.DispatchSummary{dispatch},
	})

	queued := findCanonicalDispatchEventByType(events, "DISPATCH_QUEUED", sessionID, "dispatch-1")
	if queued == nil {
		t.Fatalf("events = %#v, want DISPATCH_QUEUED", events)
	}
	var queuedPayload struct {
		DispatchKind    string `json:"dispatchKind"`
		Provider        string `json:"provider"`
		PresetID        string `json:"presetId"`
		ModelProvider   string `json:"modelProvider"`
		Model           string `json:"model"`
		ReasoningEffort string `json:"reasoningEffort"`
		RunnerID        string `json:"runnerId"`
	}
	if err := json.Unmarshal(queued.Payload, &queuedPayload); err != nil {
		t.Fatalf("unmarshal DISPATCH_QUEUED payload: %v", err)
	}
	if queuedPayload.DispatchKind != "JAVASCRIPT_AGENT" || queuedPayload.Provider != "mock" {
		t.Fatalf("queued payload = %#v, want JAVASCRIPT_AGENT/mock", queuedPayload)
	}
	assertQueuedResolvedWorkerSelection(t, queuedPayload.PresetID, queuedPayload.ModelProvider, queuedPayload.Model, queuedPayload.ReasoningEffort, queuedPayload.RunnerID)

	reconciled := findCanonicalDispatchEventByType(events, "DISPATCH_RECONCILED", sessionID, "dispatch-1")
	if reconciled == nil {
		t.Fatalf("events = %#v, want DISPATCH_RECONCILED", events)
	}
	var reconciledPayload struct {
		ReconciledStatus     string   `json:"reconciledStatus"`
		ReconciliationSource string   `json:"reconciliationSource"`
		ArtifactIDs          []string `json:"artifactIds"`
	}
	if err := json.Unmarshal(reconciled.Payload, &reconciledPayload); err != nil {
		t.Fatalf("unmarshal DISPATCH_RECONCILED payload: %v", err)
	}
	if reconciledPayload.ReconciledStatus != string(fse.DispatchStatusCompleted) {
		t.Fatalf("reconciledStatus = %q, want COMPLETED", reconciledPayload.ReconciledStatus)
	}
	if reconciledPayload.ReconciliationSource != "PROVIDER_SESSION" {
		t.Fatalf("reconciliationSource = %q, want PROVIDER_SESSION", reconciledPayload.ReconciliationSource)
	}
	if len(reconciledPayload.ArtifactIDs) != 1 || reconciledPayload.ArtifactIDs[0] != "child-artifact-1" {
		t.Fatalf("artifactIds = %#v, want [child-artifact-1]", reconciledPayload.ArtifactIDs)
	}
}

func assertQueuedResolvedWorkerSelection(t *testing.T, presetID, modelProvider, model, reasoningEffort, runnerID string) {
	t.Helper()
	if presetID != "careful-review" || modelProvider != "CODEX" || model != "gpt-test" || reasoningEffort != "high" || runnerID != "review" {
		t.Fatalf("queued resolved worker selection = %q/%q/%q/%q/%q", presetID, modelProvider, model, reasoningEffort, runnerID)
	}
}

func TestBuildCanonicalRuntimeSessionEvents_ProjectsFailedDispatchReconciliation(t *testing.T) {
	startedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(2 * time.Second)
	sessionID := "dur-sess-dispatch-events-failed-001"
	session := fse.SessionReadResult{
		SessionID:        sessionID,
		Status:           fse.LifecycleStatusFailed,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Dialect:          "you-workflow-v1",
		Phase:            "execute",
		Lifecycle:        &fse.LifecycleTimestamps{StartedAt: &startedAt, FinishedAt: &finishedAt},
	}
	result := fse.ResultReadResult{
		SessionID:     sessionID,
		ResultStatus:  fse.ResultStatusUnavailable,
		SessionStatus: fse.LifecycleStatusFailed,
	}
	dispatch := fse.DispatchSummary{
		ID:           "dispatch-1",
		Status:       fse.DispatchStatusFailed,
		DispatchKind: "JAVASCRIPT_AGENT",
		Phase:        "execute",
		Label:        "failing child",
		JavaScript: &fse.DispatchJavaScriptProjection{
			TaskKind:      "AGENT",
			ExecutionMode: fse.ChildExecutorModeLive,
		},
		FailureDetail: &fse.DispatchFailureDetail{
			Reason:  "permanent_bad_request",
			Message: "provider rejected child request",
		},
	}

	events := fse.BuildCanonicalRuntimeSessionEvents(session, result, fse.RuntimeDispatchEventInput{
		Dispatches: []fse.DispatchSummary{dispatch},
	})

	reconciled := findCanonicalDispatchEventByType(events, "DISPATCH_RECONCILED", sessionID, "dispatch-1")
	if reconciled == nil {
		t.Fatalf("events = %#v, want DISPATCH_RECONCILED", events)
	}
	var reconciledPayload struct {
		ReconciledStatus     string `json:"reconciledStatus"`
		ReconciliationSource string `json:"reconciliationSource"`
		FailureDetail        *struct {
			Reason string `json:"reason"`
		} `json:"failureDetail"`
	}
	if err := json.Unmarshal(reconciled.Payload, &reconciledPayload); err != nil {
		t.Fatalf("unmarshal DISPATCH_RECONCILED payload: %v", err)
	}
	if reconciledPayload.ReconciledStatus != string(fse.DispatchStatusFailed) {
		t.Fatalf("reconciledStatus = %q, want FAILED", reconciledPayload.ReconciledStatus)
	}
	if reconciledPayload.ReconciliationSource != "RUNTIME_RECONCILER" {
		t.Fatalf("reconciliationSource = %q, want RUNTIME_RECONCILER", reconciledPayload.ReconciliationSource)
	}
	if reconciledPayload.FailureDetail == nil || reconciledPayload.FailureDetail.Reason != "permanent_bad_request" {
		t.Fatalf("failureDetail = %#v, want permanent_bad_request", reconciledPayload.FailureDetail)
	}
}

func TestMapCanonicalRuntimeSessionEvents_EquivalentOrchestratorsHaveSharedPublicMeaning(t *testing.T) {
	startedAt := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		name           string
		sessionStatus  fse.LifecycleStatus
		resultStatus   fse.ResultStatus
		dispatchStatus fse.DispatchStatus
		artifactIDs    []string
	}{
		{name: "successful", sessionStatus: fse.LifecycleStatusSucceeded, resultStatus: fse.ResultStatusFinal, dispatchStatus: fse.DispatchStatusCompleted, artifactIDs: []string{"artifact-1"}},
		{name: "failed", sessionStatus: fse.LifecycleStatusFailed, resultStatus: fse.ResultStatusUnavailable, dispatchStatus: fse.DispatchStatusFailed},
		{name: "canceled", sessionStatus: fse.LifecycleStatusCanceled, resultStatus: fse.ResultStatusUnavailable, dispatchStatus: fse.DispatchStatusCanceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base, result, input := canonicalBoundaryFacts(startedAt, tc.name, tc.sessionStatus, tc.resultStatus, tc.dispatchStatus, tc.artifactIDs)
			petriEvents := mapCanonicalFactsForOrchestrator(t, base, result, input, interfaces.OrchestratorKindPetri, "")
			javascriptEvents := mapCanonicalFactsForOrchestrator(t, base, result, input, interfaces.OrchestratorKindJavaScript, "you-workflow-v1")
			if got, want := sharedEventMeaning(t, javascriptEvents), sharedEventMeaning(t, petriEvents); !reflect.DeepEqual(got, want) {
				t.Fatalf("shared public meaning differs:\nJavaScript: %#v\nPetri: %#v", got, want)
			}
			petriSession, petriResult := replaySharedProjection(t, petriEvents)
			javascriptSession, javascriptResult := replaySharedProjection(t, javascriptEvents)
			if !reflect.DeepEqual(javascriptSession, petriSession) || !reflect.DeepEqual(javascriptResult, petriResult) {
				t.Fatalf("replayed shared projection differs:\nJavaScript: %#v %#v\nPetri: %#v %#v", javascriptSession, javascriptResult, petriSession, petriResult)
			}
			if javascriptSession.Status != tc.sessionStatus || javascriptResult.ResultStatus != tc.resultStatus {
				t.Fatalf("replayed terminal projection = %#v %#v", javascriptSession, javascriptResult)
			}
			duplicated := append(append([]json.RawMessage(nil), javascriptEvents...), javascriptEvents...)
			duplicateSession, duplicateResult := replaySharedProjection(t, duplicated)
			if !reflect.DeepEqual(duplicateSession, javascriptSession) || !reflect.DeepEqual(duplicateResult, javascriptResult) {
				t.Fatalf("duplicate input advanced projection: %#v %#v", duplicateSession, duplicateResult)
			}
		})
	}
}

func canonicalBoundaryFacts(startedAt time.Time, suffix string, sessionStatus fse.LifecycleStatus, resultStatus fse.ResultStatus, dispatchStatus fse.DispatchStatus, artifactIDs []string) (fse.SessionReadResult, fse.ResultReadResult, fse.RuntimeDispatchEventInput) {
	sessionID := "session-shared-facts-" + suffix
	base := fse.SessionReadResult{SessionID: sessionID, Status: sessionStatus, Phase: "execute", Lifecycle: &fse.LifecycleTimestamps{StartedAt: &startedAt, FinishedAt: timePtr(startedAt.Add(time.Second))}}
	result := fse.ResultReadResult{SessionID: sessionID, ResultStatus: resultStatus, SessionStatus: sessionStatus, ArtifactIDs: artifactIDs}
	dispatch := fse.DispatchSummary{ID: "dispatch-1", Status: dispatchStatus, DispatchKind: "AGENT", Phase: "execute", Provider: "mock", ProviderSessionRefs: []fse.ProviderSessionRef{{Provider: "mock", Kind: "session_id", ID: "provider-session-1"}}, OutputArtifactIDs: artifactIDs}
	input := fse.RuntimeDispatchEventInput{Dispatches: []fse.DispatchSummary{dispatch}}
	if len(artifactIDs) > 0 {
		input.Artifacts = []fse.ArtifactSummary{{ID: artifactIDs[0], Kind: "worker-output", Visibility: "session", Label: "shared output", ContentHash: "sha256:shared-output", SizeBytes: 42, CreatedAt: timePtr(startedAt.Add(500 * time.Millisecond)), DispatchID: dispatch.ID}}
	}
	return base, result, input
}

func mapCanonicalFactsForOrchestrator(t *testing.T, base fse.SessionReadResult, result fse.ResultReadResult, input fse.RuntimeDispatchEventInput, kind, dialect string) []json.RawMessage {
	t.Helper()
	base.OrchestratorKind, base.Dialect = kind, dialect
	events, err := fse.MapCanonicalRuntimeSessionEvents(base, result, input)
	if err != nil {
		t.Fatalf("map %s facts: %v", kind, err)
	}
	return events
}

func replaySharedProjection(t *testing.T, events []json.RawMessage) (fse.SessionReadResult, fse.ResultReadResult) {
	t.Helper()
	session, result, err := fse.ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("replay canonical history: %v", err)
	}
	session.OrchestratorKind, session.Dialect, session.ResolvedSource.Dialect = "", "", ""
	return session, result
}

func TestMapCanonicalRuntimeSessionEvents_RejectsMalformedFactsWithoutPartialEvents(t *testing.T) {
	session := fse.SessionReadResult{
		SessionID:        "session-invalid-facts-001",
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
	}
	events, err := fse.MapCanonicalRuntimeSessionEvents(session, fse.ResultReadResult{SessionID: session.SessionID}, fse.RuntimeDispatchEventInput{
		Dispatches: []fse.DispatchSummary{{Status: fse.DispatchStatusQueued, DispatchKind: "AGENT"}},
	})
	if err == nil || !strings.Contains(err.Error(), "dispatch 0 ID is required") {
		t.Fatalf("error = %v, want missing dispatch ID", err)
	}
	if events != nil {
		t.Fatalf("events = %#v, want nil on invalid input", events)
	}
}

func sharedEventMeaning(t *testing.T, events []json.RawMessage) []map[string]any {
	t.Helper()
	meaning := make([]map[string]any, 0, len(events))
	for _, raw := range events {
		var event map[string]any
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("unmarshal canonical event: %v", err)
		}
		context, ok := event["context"].(map[string]any)
		if !ok {
			t.Fatalf("event context = %#v", event["context"])
		}
		delete(context, "orchestratorKind")
		delete(context, "orchestratorDialect")
		meaning = append(meaning, event)
	}
	return meaning
}

func timePtr(value time.Time) *time.Time { return &value }

func findCanonicalDispatchEventByType(events []json.RawMessage, eventType, sessionID, dispatchID string) *struct {
	Payload json.RawMessage `json:"payload"`
	Context struct {
		SessionID  *string `json:"sessionId"`
		DispatchID *string `json:"dispatchId"`
	} `json:"context"`
	Type string `json:"type"`
} {
	for _, raw := range events {
		var envelope struct {
			Payload json.RawMessage `json:"payload"`
			Context struct {
				SessionID  *string `json:"sessionId"`
				DispatchID *string `json:"dispatchId"`
			} `json:"context"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if envelope.Type != eventType {
			continue
		}
		if envelope.Context.SessionID == nil || *envelope.Context.SessionID != sessionID {
			continue
		}
		if dispatchID != "" && (envelope.Context.DispatchID == nil || *envelope.Context.DispatchID != dispatchID) {
			continue
		}
		return &struct {
			Payload json.RawMessage `json:"payload"`
			Context struct {
				SessionID  *string `json:"sessionId"`
				DispatchID *string `json:"dispatchId"`
			} `json:"context"`
			Type string `json:"type"`
		}{
			Payload: envelope.Payload,
			Context: envelope.Context,
			Type:    envelope.Type,
		}
	}
	return nil
}
