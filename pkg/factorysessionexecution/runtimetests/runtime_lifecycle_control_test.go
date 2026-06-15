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
