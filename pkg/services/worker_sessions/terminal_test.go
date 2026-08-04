package workersessions_test

import (
	"errors"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestTerminalOutcome_Valid(t *testing.T) {
	for _, outcome := range []workersessions.TerminalOutcome{workersessions.TerminalOutcomeCompleted, workersessions.TerminalOutcomeFailed} {
		if !outcome.Valid() {
			t.Errorf("TerminalOutcome(%q).Valid() = false, want true", outcome)
		}
	}
	for _, outcome := range []workersessions.TerminalOutcome{"", "CANCELED", "unknown"} {
		if outcome.Valid() {
			t.Errorf("TerminalOutcome(%q).Valid() = true, want false", outcome)
		}
	}
}

func TestFailureCauseKind_Valid(t *testing.T) {
	valid := []workersessions.FailureCauseKind{
		workersessions.FailureCauseStartFailure,
		workersessions.FailureCauseWorkersExecutionFailure,
		workersessions.FailureCauseAdapterFailure,
		workersessions.FailureCauseExecutorPanic,
		workersessions.FailureCauseEventPublicationFailure,
	}
	for _, kind := range valid {
		if !kind.Valid() {
			t.Errorf("FailureCauseKind(%q).Valid() = false, want true", kind)
		}
	}
	for _, kind := range []workersessions.FailureCauseKind{"", "UNKNOWN_FAILURE"} {
		if kind.Valid() {
			t.Errorf("FailureCauseKind(%q).Valid() = true, want false", kind)
		}
	}
}

func TestFailureCause_Validate_RejectsUnknownKind(t *testing.T) {
	if err := (workersessions.FailureCause{Kind: "UNKNOWN"}).Validate(); !errors.Is(err, workersessions.ErrInvalidFailureCause) {
		t.Errorf("Validate() = %v, want ErrInvalidFailureCause", err)
	}
	if err := (workersessions.FailureCause{Kind: workersessions.FailureCauseExecutorPanic}).Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestTerminalResult_Validate_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		result  workersessions.TerminalResult
		wantErr error
	}{
		{
			name:   "completed with no cause is valid",
			result: workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted},
		},
		{
			name: "failed with a valid cause is valid",
			result: workersessions.TerminalResult{
				Outcome: workersessions.TerminalOutcomeFailed,
				Cause:   &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure},
			},
		},
		{
			name:    "invalid outcome is rejected",
			result:  workersessions.TerminalResult{Outcome: "UNKNOWN"},
			wantErr: workersessions.ErrInvalidTerminalResult,
		},
		{
			name:    "failed without a cause is rejected",
			result:  workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeFailed},
			wantErr: workersessions.ErrInvalidTerminalResult,
		},
		{
			name: "failed with an invalid cause kind is rejected",
			result: workersessions.TerminalResult{
				Outcome: workersessions.TerminalOutcomeFailed,
				Cause:   &workersessions.FailureCause{Kind: "UNKNOWN"},
			},
			wantErr: workersessions.ErrInvalidFailureCause,
		},
		{
			name: "completed with a cause is rejected",
			result: workersessions.TerminalResult{
				Outcome: workersessions.TerminalOutcomeCompleted,
				Cause:   &workersessions.FailureCause{Kind: workersessions.FailureCauseExecutorPanic},
			},
			wantErr: workersessions.ErrInvalidTerminalResult,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.result.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
