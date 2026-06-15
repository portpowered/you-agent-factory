package fixtures_test

import (
	"context"
	"errors"
	"testing"
	"time"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

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
