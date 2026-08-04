package workersessions_test

import (
	"errors"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestSession_Validate_AcceptsNonEmptyIDAndAcceptedState(t *testing.T) {
	session := workersessions.Session{ID: "worker-1", State: workersessions.StateRunning}
	if err := session.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestSession_Validate_RejectsEmptyAndWhitespaceIdentity(t *testing.T) {
	for _, id := range []string{"", "   ", "\t"} {
		session := workersessions.Session{ID: id, State: workersessions.StateRunning}
		if err := session.Validate(); !errors.Is(err, workersessions.ErrInvalidSessionID) {
			t.Errorf("Validate() with ID %q = %v, want ErrInvalidSessionID", id, err)
		}
	}
}

func TestSession_Validate_RejectsUnknownAndInterruptedState(t *testing.T) {
	for _, state := range []workersessions.State{"", "INTERRUPTED", "unknown"} {
		session := workersessions.Session{ID: "worker-1", State: state}
		if err := session.Validate(); !errors.Is(err, workersessions.ErrInvalidState) {
			t.Errorf("Validate() with state %q = %v, want ErrInvalidState", state, err)
		}
	}
}

func TestSession_Validate_IsDeterministicAndDoesNotMutate(t *testing.T) {
	session := workersessions.Session{ID: "worker-1", State: workersessions.StateRunning}
	original := session

	firstErr := session.Validate()
	secondErr := session.Validate()

	if firstErr != nil || secondErr != nil {
		t.Fatalf("Validate() = (%v, %v), want (nil, nil)", firstErr, secondErr)
	}
	if session != original {
		t.Fatalf("Validate() mutated the session: got %+v, want %+v", session, original)
	}
}

func TestSession_Validate_CompletedRequiresMatchingCompletedResult(t *testing.T) {
	session := workersessions.Session{
		ID:     "worker-1",
		State:  workersessions.StateCompleted,
		Result: &workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted},
	}
	if err := session.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}

	missing := workersessions.Session{ID: "worker-1", State: workersessions.StateCompleted}
	if err := missing.Validate(); !errors.Is(err, workersessions.ErrInvalidTerminalResult) {
		t.Errorf("Validate() with missing Result = %v, want ErrInvalidTerminalResult", err)
	}

	mismatched := workersessions.Session{
		ID:    "worker-1",
		State: workersessions.StateCompleted,
		Result: &workersessions.TerminalResult{
			Outcome: workersessions.TerminalOutcomeFailed,
			Cause:   &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure},
		},
	}
	if err := mismatched.Validate(); !errors.Is(err, workersessions.ErrInvalidTerminalResult) {
		t.Errorf("Validate() with mismatched Result = %v, want ErrInvalidTerminalResult", err)
	}
}

func TestSession_Validate_FailedRequiresMatchingFailedResultWithCause(t *testing.T) {
	session := workersessions.Session{
		ID:    "worker-1",
		State: workersessions.StateFailed,
		Result: &workersessions.TerminalResult{
			Outcome: workersessions.TerminalOutcomeFailed,
			Cause:   &workersessions.FailureCause{Kind: workersessions.FailureCauseExecutorPanic},
		},
	}
	if err := session.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}

	noCause := workersessions.Session{
		ID:     "worker-1",
		State:  workersessions.StateFailed,
		Result: &workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeFailed},
	}
	if err := noCause.Validate(); !errors.Is(err, workersessions.ErrInvalidTerminalResult) {
		t.Errorf("Validate() with FAILED and nil Cause = %v, want ErrInvalidTerminalResult", err)
	}
}

func TestSession_Validate_NonTerminalStateRejectsTerminalResult(t *testing.T) {
	for _, state := range []workersessions.State{
		workersessions.StateReserved, workersessions.StateStarting, workersessions.StateRunning, workersessions.StatePaused,
	} {
		session := workersessions.Session{
			ID:     "worker-1",
			State:  state,
			Result: &workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted},
		}
		if err := session.Validate(); !errors.Is(err, workersessions.ErrInvalidTerminalResult) {
			t.Errorf("Validate() with state %q and a Result = %v, want ErrInvalidTerminalResult", state, err)
		}
	}
}

func TestSession_Terminal_MatchesStateTerminal(t *testing.T) {
	for _, state := range []workersessions.State{
		workersessions.StateReserved, workersessions.StateStarting, workersessions.StateRunning, workersessions.StatePaused,
		workersessions.StateCompleted, workersessions.StateFailed, workersessions.StateCanceled, workersessions.StateTerminated,
	} {
		session := workersessions.Session{ID: "worker-1", State: state}
		if got, want := session.Terminal(), state.Terminal(); got != want {
			t.Errorf("Session.Terminal() with state %q = %v, want %v", state, got, want)
		}
	}
}

func TestReserveRequest_Validate_RejectsEmptyIdentity(t *testing.T) {
	if err := (workersessions.ReserveRequest{ID: ""}).Validate(); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Errorf("Validate() = %v, want ErrInvalidSessionID", err)
	}
	if err := (workersessions.ReserveRequest{ID: "worker-1"}).Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestGetRequest_Validate_RejectsEmptyIdentity(t *testing.T) {
	if err := (workersessions.GetRequest{ID: ""}).Validate(); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Errorf("Validate() = %v, want ErrInvalidSessionID", err)
	}
	if err := (workersessions.GetRequest{ID: "worker-1"}).Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestFilter_Validate_RejectsUnknownState(t *testing.T) {
	filter := workersessions.Filter{States: []workersessions.State{workersessions.StateRunning, "INTERRUPTED"}}
	if err := filter.Validate(); !errors.Is(err, workersessions.ErrInvalidState) {
		t.Errorf("Validate() = %v, want ErrInvalidState", err)
	}
}

func TestFilter_Validate_AcceptsEmptyAndAllValidStates(t *testing.T) {
	if err := (workersessions.Filter{}).Validate(); err != nil {
		t.Errorf("Validate() on empty filter = %v, want nil", err)
	}
	filter := workersessions.Filter{States: []workersessions.State{workersessions.StateRunning, workersessions.StateCompleted}}
	if err := filter.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

// TestListRequest_Validate_DelegatesToFilter proves ListRequest.Validate is
// exactly req.Filter.Validate(): it accepts a well-formed Filter and rejects
// the same malformed Filter Filter.Validate itself rejects.
func TestListRequest_Validate_DelegatesToFilter(t *testing.T) {
	if err := (workersessions.ListRequest{Filter: workersessions.Filter{States: []workersessions.State{workersessions.StateRunning}}}).Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
	req := workersessions.ListRequest{Filter: workersessions.Filter{States: []workersessions.State{"INTERRUPTED"}}}
	if err := req.Validate(); !errors.Is(err, workersessions.ErrInvalidState) {
		t.Errorf("Validate() = %v, want ErrInvalidState", err)
	}
}
