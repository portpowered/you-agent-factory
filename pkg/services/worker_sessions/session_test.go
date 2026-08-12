package workersessions_test

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
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
			Cause:   &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure, Detail: "execution failed"},
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
			Cause:   &workersessions.FailureCause{Kind: workersessions.FailureCauseExecutorPanic, Detail: "executor failed"},
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

func TestProviderSessionAssociation_ValidateAndClone(t *testing.T) {
	valid := workersessions.ProviderSessionAssociation{
		WorkerSessionID: "worker-1",
		TurnID:          "turn-1",
		DispatchID:      "dispatch-1",
		AttemptID:       "dispatch-1",
		Reference: providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "provider-session-1",
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid association Validate() = %v, want nil", err)
	}
	if got := valid.Clone(); got != valid {
		t.Fatalf("Clone() = %#v, want equal detached value %#v", got, valid)
	}

	for _, test := range []struct {
		name        string
		association workersessions.ProviderSessionAssociation
		wantErr     error
	}{
		{name: "missing worker session", association: workersessions.ProviderSessionAssociation{DispatchID: "dispatch-1", AttemptID: "dispatch-1", Reference: valid.Reference}, wantErr: workersessions.ErrInvalidSessionID},
		{name: "missing dispatch", association: workersessions.ProviderSessionAssociation{WorkerSessionID: "worker-1", AttemptID: "dispatch-1", Reference: valid.Reference}, wantErr: workersessions.ErrInvalidProviderSessionAssociation},
		{name: "mismatched attempt", association: workersessions.ProviderSessionAssociation{WorkerSessionID: "worker-1", DispatchID: "dispatch-1", AttemptID: "dispatch-2", Reference: valid.Reference}, wantErr: workersessions.ErrInvalidProviderSessionAssociation},
		{name: "invalid provider reference", association: workersessions.ProviderSessionAssociation{WorkerSessionID: "worker-1", DispatchID: "dispatch-1", AttemptID: "dispatch-1", Reference: providers.SessionRef{Kind: providers.SessionIDKind, ID: "provider-session-1"}}, wantErr: providers.ErrInvalidID},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.association.Validate(); !errors.Is(err, test.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestProviderSessionAssociationRequest_Validate(t *testing.T) {
	valid := workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "worker-1",
		DispatchID:      "dispatch-1",
		Reference: providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "provider-session-1",
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid association request Validate() = %v, want nil", err)
	}
	for _, test := range []struct {
		name    string
		req     workersessions.ProviderSessionAssociationRequest
		wantErr error
	}{
		{name: "missing worker session", req: workersessions.ProviderSessionAssociationRequest{DispatchID: valid.DispatchID, Reference: valid.Reference}, wantErr: workersessions.ErrInvalidSessionID},
		{name: "blank dispatch", req: workersessions.ProviderSessionAssociationRequest{WorkerSessionID: valid.WorkerSessionID, DispatchID: " ", Reference: valid.Reference}, wantErr: workersessions.ErrInvalidProviderSessionAssociation},
		{name: "invalid provider reference", req: workersessions.ProviderSessionAssociationRequest{WorkerSessionID: valid.WorkerSessionID, DispatchID: valid.DispatchID, Reference: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind}}, wantErr: providers.ErrInvalidSessionRef},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.req.Validate(); !errors.Is(err, test.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestControlRequest_Validate(t *testing.T) {
	if err := (workersessions.ControlRequest{ID: "worker-1"}).Validate(); err != nil {
		t.Fatalf("valid control request Validate() = %v, want nil", err)
	}
	if err := (workersessions.ControlRequest{ID: " \t"}).Validate(); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Fatalf("blank control request Validate() = %v, want ErrInvalidSessionID", err)
	}
}

func TestContinueRequest_ValidateAndNormalize(t *testing.T) {
	valid := workersessions.ContinueRequest{
		RequestID:                " request-1 ",
		SourceWorkerSessionID:    " source-1 ",
		SuccessorWorkerSessionID: " successor-1 ",
		FollowUpInput:            "  follow-up  ",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid ContinueRequest.Validate() = %v, want nil", err)
	}
	normalized := valid.Normalize()
	if normalized.RequestID != "request-1" || normalized.SourceWorkerSessionID != "source-1" || normalized.SuccessorWorkerSessionID != "successor-1" {
		t.Fatalf("Normalize() identities = %#v, want trimmed identities", normalized)
	}
	if normalized.FollowUpInput != valid.FollowUpInput {
		t.Fatalf("Normalize() changed follow-up input from %q to %q", valid.FollowUpInput, normalized.FollowUpInput)
	}

	for _, test := range []struct {
		name string
		req  workersessions.ContinueRequest
		want error
	}{
		{name: "missing request ID", req: workersessions.ContinueRequest{SourceWorkerSessionID: "source", SuccessorWorkerSessionID: "successor", FollowUpInput: "input"}, want: workersessions.ErrInvalidContinuationRequestID},
		{name: "same lineage identity", req: workersessions.ContinueRequest{RequestID: "request", SourceWorkerSessionID: "same", SuccessorWorkerSessionID: "same", FollowUpInput: "input"}, want: workersessions.ErrInvalidContinuationLineage},
		{name: "missing input", req: workersessions.ContinueRequest{RequestID: "request", SourceWorkerSessionID: "source", SuccessorWorkerSessionID: "successor"}, want: workersessions.ErrInvalidContinuationInput},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.req.Validate(); !errors.Is(err, test.want) {
				t.Fatalf("Validate() = %v, want %v", err, test.want)
			}
		})
	}
}

func TestSession_Validate_RequiresAssociationToBelongToSession(t *testing.T) {
	association := &workersessions.ProviderSessionAssociation{
		WorkerSessionID: "worker-1",
		DispatchID:      "dispatch-1",
		AttemptID:       "dispatch-1",
		Reference: providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "provider-session-1",
		},
	}
	valid := workersessions.Session{ID: "worker-1", State: workersessions.StateRunning, ProviderSessionAssociation: association}
	if err := valid.Validate(); err != nil {
		t.Fatalf("associated session Validate() = %v, want nil", err)
	}

	mismatched := valid
	associationCopy := association.Clone()
	mismatched.ProviderSessionAssociation = &associationCopy
	mismatched.ProviderSessionAssociation.WorkerSessionID = "worker-2"
	if err := mismatched.Validate(); !errors.Is(err, workersessions.ErrInvalidProviderSessionAssociation) {
		t.Fatalf("mismatched association Validate() = %v, want ErrInvalidProviderSessionAssociation", err)
	}

	malformed := valid
	malformedAssociation := association.Clone()
	malformedAssociation.Reference.ID = ""
	malformed.ProviderSessionAssociation = &malformedAssociation
	if err := malformed.Validate(); !errors.Is(err, providers.ErrInvalidSessionRef) {
		t.Fatalf("malformed association Validate() = %v, want Providers ErrInvalidSessionRef", err)
	}
}
