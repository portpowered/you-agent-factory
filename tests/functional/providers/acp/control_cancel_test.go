package acp_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

// TestControlAttemptCancelsInFlightACPAttemptThroughRealSubprocess proves the
// Providers root's ControlAttempt(cancel) seam against a real ACP daemon
// subprocess (not a fake acp.Service): the exact in-flight attempt accepts a
// real session/cancel notification sent over OS pipes, the daemon's
// beginCancelable/endCancelable window and StopReasonCancelled mapping
// observe it, and Execute reports the established canceled failure.
func TestControlAttemptCancelsInFlightACPAttemptThroughRealSubprocess(t *testing.T) {
	t.Setenv(acpHelperEnvironment, "cancelable")
	signal := filepath.Join(t.TempDir(), "prompt-started")
	t.Setenv("YOU_TEST_ACP_PROMPT_SIGNAL", signal)

	var starts atomic.Int32
	service, err := providerswire.NewService(
		providerswire.WithCommandFactory(acpHelperCommandFactory(&starts)),
		providerswire.WithExecutableLocator(availableExecutableLocator{}),
	)
	if err != nil {
		t.Fatalf("construct Providers service: %v", err)
	}

	const provider = providers.ID("cursor-acp")
	const attemptID = "acp-control-cancel-attempt"
	workingDirectory := t.TempDir()
	type executeOutcome struct {
		result providers.ExecuteResult
		err    error
	}
	executed := make(chan executeOutcome, 1)
	go func() {
		result, err := service.Execute(context.Background(), providers.ExecuteRequest{
			Provider:         provider,
			AttemptID:        attemptID,
			UserMessage:      "cancel this ACP attempt",
			WorkingDirectory: workingDirectory,
		})
		executed <- executeOutcome{result: result, err: err}
	}()

	waitForACPTestFile(t, signal)

	controlResult, err := service.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  provider,
		AttemptID: attemptID,
		Action:    providers.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("ControlAttempt(cancel) error = %v", err)
	}
	if controlResult.Outcome != providers.ControlOutcomeCompleted {
		t.Fatalf("ControlAttempt(cancel) outcome = %q, want %q", controlResult.Outcome, providers.ControlOutcomeCompleted)
	}

	select {
	case outcome := <-executed:
		var failure providers.ExecuteFailure
		if !errors.As(outcome.err, &failure) || failure.Kind != providers.ExecuteFailureKindCanceled {
			t.Fatalf("Execute() error = %v, want ExecuteFailureKindCanceled", outcome.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the canceled Execute() to return")
	}

	if got := starts.Load(); got != 1 {
		t.Fatalf("ACP process starts = %d, want 1", got)
	}
}
