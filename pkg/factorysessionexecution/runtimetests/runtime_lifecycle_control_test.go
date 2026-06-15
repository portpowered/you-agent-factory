package factorysessionexecution_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestRuntimeService_CancelRunningSessionReturnsCanceling(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")

	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-cancel-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	canceled, err := service.Cancel(context.Background(), started.SessionID, factorysessionexecution.ControlRequest{})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if canceled.Outcome != factorysessionexecution.LifecycleControlOutcomeAccepted ||
		canceled.Status != factorysessionexecution.LifecycleStatusCanceling {
		t.Fatalf("cancel = %#v, want ACCEPTED/CANCELING", canceled)
	}
	if canceled.Operation != factorysessionexecution.LifecycleControlCancel {
		t.Fatalf("operation = %q, want CANCEL", canceled.Operation)
	}
	if canceled.Links.Results == "" || canceled.Links.Session == "" {
		t.Fatalf("links = %#v, want inspection links", canceled.Links)
	}
}

func TestRuntimeService_TerminateRunningSessionReturnsTerminated(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")

	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-terminate-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	terminated, err := service.Terminate(context.Background(), started.SessionID, factorysessionexecution.ControlRequest{})
	if err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if terminated.Outcome != factorysessionexecution.LifecycleControlOutcomeAccepted ||
		terminated.Status != factorysessionexecution.LifecycleStatusTerminated {
		t.Fatalf("terminate = %#v, want ACCEPTED/TERMINATED", terminated)
	}
}

func TestRuntimeService_CancelTerminalSessionReturnsTypedControlError(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")

	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-cancel-terminal-001",
		Source: factorysessionexecution.Source{
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
		if read.Status == factorysessionexecution.LifecycleStatusSucceeded {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	_, err = service.Cancel(context.Background(), started.SessionID, factorysessionexecution.ControlRequest{})
	var controlErr *factorysessionexecution.ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != factorysessionexecution.LifecycleControlOutcomeTerminalSession {
		t.Fatalf("cancel on terminal = %v, want TERMINAL_SESSION ControlError", err)
	}
}

func TestRuntimeService_CancelMissingSessionReturnsNotFound(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")
	_, err := service.Cancel(context.Background(), "dur-sess-missing-999", factorysessionexecution.ControlRequest{})
	if !errors.Is(err, factorysessionexecution.ErrSessionNotFound) {
		t.Fatalf("cancel missing = %v, want ErrSessionNotFound", err)
	}
}

func TestRuntimeService_PauseRunningSessionReturnsPaused(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")

	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-pause-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	paused, err := service.Pause(context.Background(), started.SessionID, factorysessionexecution.ControlRequest{})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.Outcome != factorysessionexecution.LifecycleControlOutcomeAccepted ||
		paused.Status != factorysessionexecution.LifecycleStatusPaused {
		t.Fatalf("pause = %#v, want ACCEPTED/PAUSED", paused)
	}
	if paused.Operation != factorysessionexecution.LifecycleControlPause {
		t.Fatalf("operation = %q, want PAUSE", paused.Operation)
	}
}

func TestRuntimeService_ResumePausedSessionReturnsRunning(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")

	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-resume-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	if _, err := service.Pause(context.Background(), started.SessionID, factorysessionexecution.ControlRequest{}); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	resumed, err := service.Resume(context.Background(), started.SessionID, factorysessionexecution.ControlRequest{})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if resumed.Outcome != factorysessionexecution.LifecycleControlOutcomeAccepted ||
		resumed.Status != factorysessionexecution.LifecycleStatusRunning {
		t.Fatalf("resume = %#v, want ACCEPTED/RUNNING", resumed)
	}
}

func TestRuntimeService_PauseTerminalSessionReturnsTypedControlError(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")

	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-pause-terminal-001",
		Source: factorysessionexecution.Source{
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
		if read.Status == factorysessionexecution.LifecycleStatusSucceeded {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	_, err = service.Pause(context.Background(), started.SessionID, factorysessionexecution.ControlRequest{})
	var controlErr *factorysessionexecution.ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != factorysessionexecution.LifecycleControlOutcomeTerminalSession {
		t.Fatalf("pause on terminal = %v, want TERMINAL_SESSION ControlError", err)
	}
}

func TestRuntimeService_ApproveRunningSessionReturnsTypedControlError(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")

	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-approve-invalid-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	_, err = service.Approve(context.Background(), started.SessionID, factorysessionexecution.ApproveRequest{})
	var controlErr *factorysessionexecution.ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != factorysessionexecution.LifecycleControlOutcomeInvalidState {
		t.Fatalf("approve on running = %v, want INVALID_STATE ControlError", err)
	}
}

func TestRuntimeService_RetryDispatchMissingDispatchReturnsNotFound(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")

	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-retry-missing-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	_, err = service.RetryDispatch(context.Background(), started.SessionID, factorysessionexecution.RetryDispatchRequest{
		DispatchID: "disp-missing-001",
	})
	if !errors.Is(err, factorysessionexecution.ErrDispatchNotFound) {
		t.Fatalf("retry missing dispatch = %v, want ErrDispatchNotFound", err)
	}
}

func TestRuntimeService_ControlIdempotentReplayAndConflict(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")

	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-control-replay-start-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	first, err := service.Pause(context.Background(), started.SessionID, factorysessionexecution.ControlRequest{
		RequestID: "ctrl-runtime-replay-001",
	})
	if err != nil {
		t.Fatalf("first Pause: %v", err)
	}
	second, err := service.Pause(context.Background(), started.SessionID, factorysessionexecution.ControlRequest{
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
	if read.Status != factorysessionexecution.LifecycleStatusPaused {
		t.Fatalf("status after replay = %q, want PAUSED", read.Status)
	}

	_, err = service.Resume(context.Background(), started.SessionID, factorysessionexecution.ControlRequest{
		RequestID: "ctrl-runtime-replay-001",
	})
	var controlErr *factorysessionexecution.ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != factorysessionexecution.LifecycleControlOutcomeConflict {
		t.Fatalf("conflict = %v, want CONFLICT ControlError", err)
	}
	if controlErr.Status != factorysessionexecution.LifecycleStatusPaused {
		t.Fatalf("conflict status = %q, want PAUSED unchanged", controlErr.Status)
	}
}
