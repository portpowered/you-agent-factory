package service

import (
	"context"
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/fixtures"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
)

func TestDurableControlPauseAcceptedOnRunningSession(t *testing.T) {
	t.Parallel()

	owner, err := New(newFixtureBackedExecution(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	runningRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeAsyncRunning)
	startPublishedScenario(t, owner, runningRow)

	paused, err := owner.Pause(context.Background(), runningRow.SessionID, factorysessions.ControlRequest{})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.SessionID != runningRow.SessionID {
		t.Fatalf("sessionId = %q, want %q", paused.SessionID, runningRow.SessionID)
	}
	if paused.Outcome != factorysessions.LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", paused.Outcome)
	}
	if paused.Status != factorysessions.LifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", paused.Status)
	}
	if paused.Operation != factorysessions.LifecycleControlPause {
		t.Fatalf("operation = %q, want PAUSE", paused.Operation)
	}
}

func TestDurableControlPauseNoOpWhenAlreadyPaused(t *testing.T) {
	t.Parallel()

	owner, err := New(newFixtureBackedExecution(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	pausedRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeLifecycleControl)
	startPublishedScenario(t, owner, pausedRow)

	pauseNoOp, err := owner.Pause(context.Background(), pausedRow.SessionID, factorysessions.ControlRequest{})
	if err != nil {
		t.Fatalf("Pause on paused session: %v", err)
	}
	if pauseNoOp.Outcome != factorysessions.LifecycleControlOutcomeNoOp {
		t.Fatalf("outcome = %q, want NO_OP", pauseNoOp.Outcome)
	}
	if pauseNoOp.Status != factorysessions.LifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED unchanged", pauseNoOp.Status)
	}
	if pauseNoOp.SessionID != pausedRow.SessionID {
		t.Fatalf("sessionId = %q, want %q", pauseNoOp.SessionID, pausedRow.SessionID)
	}
}

func TestDurableControlAgainstTerminalSessionReturnsTerminalOutcome(t *testing.T) {
	t.Parallel()

	owner, err := New(newFixtureBackedExecution(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	terminalRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeSyncSuccess)
	startPublishedScenario(t, owner, terminalRow)

	_, err = owner.Pause(context.Background(), terminalRow.SessionID, factorysessions.ControlRequest{})
	var rejected *factorysessions.DurableControlError
	if !errors.As(err, &rejected) {
		t.Fatalf("Pause terminal = %v, want *DurableControlError", err)
	}
	if rejected.Outcome != factorysessions.LifecycleControlOutcomeTerminalSession {
		t.Fatalf("outcome = %q, want TERMINAL_SESSION", rejected.Outcome)
	}
	if errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatal("terminal control rejection must stay distinct from ErrDurableSessionNotFound")
	}
}

func TestDurableControlUnknownSessionReturnsNotFound(t *testing.T) {
	t.Parallel()

	owner, err := New(newFixtureBackedExecution(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = owner.Pause(context.Background(), "dur-sess-missing-999", factorysessions.ControlRequest{})
	if !errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatalf("Pause missing = %v, want ErrDurableSessionNotFound", err)
	}
}

func TestDurableControlFailuresStayDistinctFromOtherDurableErrors(t *testing.T) {
	t.Parallel()

	owner, err := New(&controlFailureStub{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	_, err = owner.Pause(ctx, "dur-sess-missing", factorysessions.ControlRequest{})
	if !errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatalf("Pause missing = %v, want ErrDurableSessionNotFound", err)
	}

	_, err = owner.Pause(ctx, "dur-sess-terminal", factorysessions.ControlRequest{})
	var terminal *factorysessions.DurableControlError
	if !errors.As(err, &terminal) || terminal.Outcome != factorysessions.LifecycleControlOutcomeTerminalSession {
		t.Fatalf("Pause terminal = %v, want *DurableControlError TERMINAL_SESSION", err)
	}
	if errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatal("terminal control rejection must stay distinct from ErrDurableSessionNotFound")
	}

	_, err = owner.ResumeInterruptedSession(ctx, "dur-sess-missing-checkpoint", factorysessions.DurableResumeRequest{RequestID: "resume-1"})
	var missingCheckpoint *factorysessions.DurableResumeError
	if !errors.As(err, &missingCheckpoint) || missingCheckpoint.Outcome != factorysessions.DurableResumeOutcomeMissingCheckpoint {
		t.Fatalf("ResumeInterruptedSession = %v, want *DurableResumeError MISSING_CHECKPOINT", err)
	}
	if errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatal("missing checkpoint must stay distinct from ErrDurableSessionNotFound")
	}
}

type controlFailureStub struct {
	durableexecution.Service
}

func (s *controlFailureStub) Pause(_ context.Context, sessionID string, _ factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	switch sessionID {
	case "dur-sess-missing":
		return factorysessions.LifecycleControlResult{}, factorysessions.ErrDurableSessionNotFound
	case "dur-sess-terminal":
		return factorysessions.LifecycleControlResult{}, &factorysessions.DurableControlError{
			Operation: factorysessions.LifecycleControlPause,
			Outcome:   factorysessions.LifecycleControlOutcomeTerminalSession,
			Status:    factorysessions.LifecycleStatusSucceeded,
			Message:   string(factorysessions.LifecycleControlOutcomeTerminalSession),
		}
	}
	return factorysessions.LifecycleControlResult{}, errors.New("unexpected control request")
}

func (s *controlFailureStub) ResumeInterruptedSession(_ context.Context, sessionID string, _ factorysessions.DurableResumeRequest) (factorysessions.AsyncStartResult, error) {
	if sessionID == "dur-sess-missing-checkpoint" {
		return factorysessions.AsyncStartResult{}, &factorysessions.DurableResumeError{
			Outcome:   factorysessions.DurableResumeOutcomeMissingCheckpoint,
			Status:    factorysessions.LifecycleStatusPaused,
			Field:     "checkpointSummary",
			Message:   string(factorysessions.DurableResumeOutcomeMissingCheckpoint),
			SessionID: sessionID,
		}
	}
	return factorysessions.AsyncStartResult{}, errors.New("unexpected resume request")
}

func startPublishedScenario(t *testing.T, owner *Service, row fixtures.PublishedFixtureScenario) {
	t.Helper()
	req := startRequestForPublished(row)
	if row.Purpose == fixtures.FixturePurposeSyncSuccess || row.Purpose == fixtures.FixturePurposeSyncTimeout {
		if _, err := owner.StartSync(context.Background(), factorysessions.DurableStartRequest(req)); err != nil {
			t.Fatalf("StartSync(%s): %v", row.Purpose, err)
		}
		return
	}
	if _, err := owner.StartAsync(context.Background(), factorysessions.DurableStartRequest(req)); err != nil {
		t.Fatalf("StartAsync(%s): %v", row.Purpose, err)
	}
}
