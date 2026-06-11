package factorysessionexecution

import (
	"context"
	"errors"
	"testing"

	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func startPublishedScenario(t *testing.T, service *FakeService, row PublishedFixtureScenario) {
	t.Helper()
	req := startRequestForPublished(row)
	if row.Purpose == FixturePurposeSyncSuccess || row.Purpose == FixturePurposeSyncTimeout {
		if _, err := service.StartSync(context.Background(), req); err != nil {
			t.Fatalf("StartSync(%s): %v", row.Purpose, err)
		}
		return
	}
	if _, err := service.StartAsync(context.Background(), req); err != nil {
		t.Fatalf("StartAsync(%s): %v", row.Purpose, err)
	}
}

func startAwaitingApprovalSession(t *testing.T, service *FakeService) {
	t.Helper()
	_, err := service.StartAsync(context.Background(), StartRequest{
		RequestID: "req-js-awaiting-001",
		Source: Source{
			Kind:         workflowsource.KindWorkflowFile,
			WorkflowFile: ".claude/workflows/approval-gate.yaml",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync awaiting approval: %v", err)
	}
}

func startFailedPartialSession(t *testing.T, service *FakeService) {
	t.Helper()
	startAsyncByRequestID(t, service, "req-js-failed-partial-001")
}

func TestFakeService_PublishedScenarios_LifecycleControlPauseResumeOutcomes(t *testing.T) {
	service := newContractFakeService(t)

	pausedRow := publishedScenarioByPurpose(t, FixturePurposeLifecycleControl)
	startPublishedScenario(t, service, pausedRow)

	pauseNoOp, err := service.Pause(context.Background(), pausedRow.SessionID, ControlRequest{})
	if err != nil {
		t.Fatalf("Pause on paused session: %v", err)
	}
	if pauseNoOp.Outcome != LifecycleControlOutcomeNoOp || pauseNoOp.Status != LifecycleStatusPaused {
		t.Fatalf("pause on paused = %#v, want NO_OP/PAUSED", pauseNoOp)
	}
	pauseNoOpHash, err := LifecycleControlResultHash(pauseNoOp)
	if err != nil {
		t.Fatalf("LifecycleControlResultHash pause no-op: %v", err)
	}
	if pauseNoOpHash != "sha256:dff882b64856e2bf56d03e29643fc65abe7129532ff38615e1033ae39873df7c" {
		t.Fatalf("pause no-op hash = %q", pauseNoOpHash)
	}

	resumed, err := service.Resume(context.Background(), pausedRow.SessionID, ControlRequest{})
	if err != nil {
		t.Fatalf("Resume paused session: %v", err)
	}
	if resumed.Outcome != LifecycleControlOutcomeAccepted || resumed.Status != LifecycleStatusRunning {
		t.Fatalf("resume = %#v, want ACCEPTED/RUNNING", resumed)
	}
	resumeHash, err := LifecycleControlResultHash(resumed)
	if err != nil {
		t.Fatalf("LifecycleControlResultHash resume: %v", err)
	}
	if resumeHash != "sha256:c12be84234b44996999436577f3967f4bccfc9b5be1d9ad179146b064d56df5a" {
		t.Fatalf("resume hash = %q", resumeHash)
	}

	runningRow := publishedScenarioByPurpose(t, FixturePurposeAsyncRunning)
	startPublishedScenario(t, service, runningRow)

	pauseAccepted, err := service.Pause(context.Background(), runningRow.SessionID, ControlRequest{})
	if err != nil {
		t.Fatalf("Pause running session: %v", err)
	}
	if pauseAccepted.Outcome != LifecycleControlOutcomeAccepted || pauseAccepted.Status != LifecycleStatusPaused {
		t.Fatalf("pause running = %#v, want ACCEPTED/PAUSED", pauseAccepted)
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlCancelTerminateOutcomes(t *testing.T) {
	service := newContractFakeService(t)
	runningRow := publishedScenarioByPurpose(t, FixturePurposeAsyncRunning)
	startPublishedScenario(t, service, runningRow)

	canceled, err := service.Cancel(context.Background(), runningRow.SessionID, ControlRequest{})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if canceled.Outcome != LifecycleControlOutcomeAccepted || canceled.Status != LifecycleStatusCanceling {
		t.Fatalf("cancel = %#v, want ACCEPTED/CANCELING", canceled)
	}

	service = newContractFakeService(t)
	startPublishedScenario(t, service, runningRow)
	terminated, err := service.Terminate(context.Background(), runningRow.SessionID, ControlRequest{})
	if err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if terminated.Outcome != LifecycleControlOutcomeAccepted || terminated.Status != LifecycleStatusTerminated {
		t.Fatalf("terminate = %#v, want ACCEPTED/TERMINATED", terminated)
	}

	terminalRow := publishedScenarioByPurpose(t, FixturePurposeSyncSuccess)
	startPublishedScenario(t, service, terminalRow)
	_, err = service.Cancel(context.Background(), terminalRow.SessionID, ControlRequest{})
	var controlErr *ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != LifecycleControlOutcomeTerminalSession {
		t.Fatalf("cancel on terminal = %v, want TERMINAL_SESSION ControlError", err)
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlApproveAwaitingApproval(t *testing.T) {
	service := newContractFakeService(t)
	startAwaitingApprovalSession(t, service)

	_, err := service.Pause(context.Background(), "dur-sess-js-awaiting-001", ControlRequest{})
	var invalidErr *ControlError
	if !errors.As(err, &invalidErr) || invalidErr.Outcome != LifecycleControlOutcomeInvalidState {
		t.Fatalf("pause on awaiting approval = %v, want INVALID_STATE ControlError", err)
	}

	approved, err := service.Approve(context.Background(), "dur-sess-js-awaiting-001", ApproveRequest{
		ControlRequest: ControlRequest{RequestID: "ctrl-approve-001"},
		ApprovedPolicy: map[string]any{"maxAgents": 2},
	})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if approved.Outcome != LifecycleControlOutcomeAccepted || approved.Status != LifecycleStatusRunning {
		t.Fatalf("approve = %#v, want ACCEPTED/RUNNING", approved)
	}
	approveHash, err := LifecycleControlResultHash(approved)
	if err != nil {
		t.Fatalf("LifecycleControlResultHash approve: %v", err)
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

	terminalRow := publishedScenarioByPurpose(t, FixturePurposeSyncSuccess)
	startPublishedScenario(t, service, terminalRow)
	_, err := service.RetryDispatch(context.Background(), terminalRow.SessionID, RetryDispatchRequest{
		ControlRequest: ControlRequest{},
		DispatchID:     "disp-petri-success-001",
	})
	var terminalErr *ControlError
	if !errors.As(err, &terminalErr) || terminalErr.Outcome != LifecycleControlOutcomeTerminalSession {
		t.Fatalf("retry on terminal = %v, want TERMINAL_SESSION ControlError", err)
	}

	recoverableRow := publishedScenarioByPurpose(t, FixturePurposeFailedRecoverable)
	startPublishedScenario(t, service, recoverableRow)
	_, err = service.RetryDispatch(context.Background(), recoverableRow.SessionID, RetryDispatchRequest{
		ControlRequest: ControlRequest{},
		DispatchID:     "disp-js-interrupted-002",
	})
	if !errors.As(err, &terminalErr) || terminalErr.Outcome != LifecycleControlOutcomeTerminalSession {
		t.Fatalf("retry on interrupted = %v, want TERMINAL_SESSION ControlError", err)
	}

	startFailedPartialSession(t, service)
	retry, err := service.RetryDispatch(context.Background(), "dur-sess-js-failed-partial-001", RetryDispatchRequest{
		ControlRequest: ControlRequest{RequestID: "ctrl-retry-fail-001"},
		DispatchID:     "disp-js-fail-002",
	})
	if err != nil {
		t.Fatalf("RetryDispatch failed partial: %v", err)
	}
	if retry.Outcome != LifecycleControlOutcomeAccepted || retry.Status != LifecycleStatusRunning {
		t.Fatalf("retry = %#v, want ACCEPTED/RUNNING", retry)
	}
	if retry.RetryDispatchID != "disp-js-fail-002" {
		t.Fatalf("retryDispatchId = %q", retry.RetryDispatchID)
	}
	retryHash, err := LifecycleControlResultHash(retry)
	if err != nil {
		t.Fatalf("LifecycleControlResultHash retry: %v", err)
	}
	if retryHash != "sha256:ff4b53b67a11b90eeb9a667c68dd206cb2156265067325eb150b31877882852b" {
		t.Fatalf("retry hash = %q", retryHash)
	}

	dispatches, err := service.ListDispatches(context.Background(), "dur-sess-js-failed-partial-001")
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	for _, dispatch := range dispatches.Dispatches {
		if dispatch.ID == "disp-js-fail-002" && dispatch.Status != DispatchStatusQueued {
			t.Fatalf("retried dispatch = %#v, want QUEUED", dispatch)
		}
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlAcceptedInspectionLinks(t *testing.T) {
	service := newContractFakeService(t)
	row := publishedScenarioByPurpose(t, FixturePurposeAsyncRunning)
	startPublishedScenario(t, service, row)

	paused, err := service.Pause(context.Background(), row.SessionID, ControlRequest{})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	want := LifecycleControlLinksForSession(row.SessionID, true)
	if paused.Links != want {
		t.Fatalf("links = %#v, want %#v", paused.Links, want)
	}
	if paused.Session == nil || paused.Session.Status != LifecycleStatusPaused {
		t.Fatalf("session projection = %#v", paused.Session)
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlIdempotentReplayAndConflict(t *testing.T) {
	service := newContractFakeService(t)
	row := publishedScenarioByPurpose(t, FixturePurposeAsyncRunning)
	startPublishedScenario(t, service, row)

	first, err := service.Pause(context.Background(), row.SessionID, ControlRequest{
		RequestID: "ctrl-lifecycle-replay-001",
	})
	if err != nil {
		t.Fatalf("first Pause: %v", err)
	}
	second, err := service.Pause(context.Background(), row.SessionID, ControlRequest{
		RequestID: "ctrl-lifecycle-replay-001",
	})
	if err != nil {
		t.Fatalf("replay Pause: %v", err)
	}
	firstHash, err := LifecycleControlResultHash(first)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	secondHash, err := LifecycleControlResultHash(second)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if firstHash != secondHash {
		t.Fatalf("replay hash drift: %q vs %q", firstHash, secondHash)
	}

	_, err = service.Resume(context.Background(), row.SessionID, ControlRequest{
		RequestID: "ctrl-lifecycle-replay-001",
	})
	var controlErr *ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != LifecycleControlOutcomeConflict {
		t.Fatalf("conflict error = %v, want CONFLICT ControlError", err)
	}
	if controlErr.Status != LifecycleStatusPaused {
		t.Fatalf("conflict status = %q, want PAUSED", controlErr.Status)
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlIsolationAcrossSessions(t *testing.T) {
	service := newContractFakeService(t)
	pausedRow := publishedScenarioByPurpose(t, FixturePurposeLifecycleControl)
	runningRow := publishedScenarioByPurpose(t, FixturePurposeAsyncRunning)
	startPublishedScenario(t, service, pausedRow)
	startPublishedScenario(t, service, runningRow)

	beforePaused, err := service.GetSession(context.Background(), pausedRow.SessionID)
	if err != nil {
		t.Fatalf("GetSession paused before: %v", err)
	}
	if beforePaused.Status != LifecycleStatusPaused {
		t.Fatalf("paused status before = %q", beforePaused.Status)
	}

	if _, err := service.Terminate(context.Background(), runningRow.SessionID, ControlRequest{}); err != nil {
		t.Fatalf("Terminate running: %v", err)
	}

	afterPaused, err := service.GetSession(context.Background(), pausedRow.SessionID)
	if err != nil {
		t.Fatalf("GetSession paused after: %v", err)
	}
	if afterPaused.Status != LifecycleStatusPaused {
		t.Fatalf("paused status after = %q, want PAUSED unchanged", afterPaused.Status)
	}
	pausedHash, err := LifecycleControlResultHash(LifecycleControlResult{
		SessionID: afterPaused.SessionID,
		Operation: LifecycleControlPause,
		Outcome:   LifecycleControlOutcomeNoOp,
		Status:    afterPaused.Status,
		Links:     LifecycleControlLinksForSession(afterPaused.SessionID, true),
	})
	if err != nil {
		t.Fatalf("LifecycleControlResultHash: %v", err)
	}
	if pausedHash != "sha256:dff882b64856e2bf56d03e29643fc65abe7129532ff38615e1033ae39873df7c" {
		t.Fatalf("isolation hash = %q", pausedHash)
	}
}

func TestFakeService_PublishedScenarios_LifecycleControlDeterministicAcrossServiceReload(t *testing.T) {
	row := publishedScenarioByPurpose(t, FixturePurposeLifecycleControl)

	runControl := func(t *testing.T) string {
		t.Helper()
		service := newContractFakeService(t)
		startPublishedScenario(t, service, row)
		resumed, err := service.Resume(context.Background(), row.SessionID, ControlRequest{})
		if err != nil {
			t.Fatalf("Resume: %v", err)
		}
		hash, err := LifecycleControlResultHash(resumed)
		if err != nil {
			t.Fatalf("LifecycleControlResultHash: %v", err)
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
