package workersessions_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
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
	if err := (workersessions.FailureCause{Kind: workersessions.FailureCauseExecutorPanic, Detail: "executor failed"}).Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestFailureCause_Validate_AdmitsOnlySafeProviderContinuationClassifications(t *testing.T) {
	valid := workersessions.FailureCause{
		Kind:                            workersessions.FailureCauseWorkersExecutionFailure,
		Detail:                          "provider execution failed",
		ProviderFailureKind:             providers.ExecuteFailureKindDependency,
		ProviderContinuationFailureKind: providers.ContinuationFailureKindStale,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid continuation classification Validate() = %v", err)
	}
	unsupported := workersessions.FailureCause{
		Kind:                        workersessions.FailureCauseWorkersExecutionFailure,
		Detail:                      "continuation was unsupported",
		ProviderContinuationOutcome: providers.ContinuationOutcomeUnsupported,
	}
	if err := unsupported.Validate(); err != nil {
		t.Fatalf("unsupported continuation classification Validate() = %v", err)
	}
	invalid := workersessions.FailureCause{
		Kind:                            workersessions.FailureCauseWorkersExecutionFailure,
		Detail:                          "continuation classification conflicted",
		ProviderContinuationFailureKind: providers.ContinuationFailureKindStale,
		ProviderContinuationOutcome:     providers.ContinuationOutcomeUnsupported,
	}
	if err := invalid.Validate(); !errors.Is(err, workersessions.ErrInvalidFailureCause) {
		t.Fatalf("simultaneous continuation failure/outcome Validate() = %v, want ErrInvalidFailureCause", err)
	}
	providerKind, continuationKind, outcome := workersessions.SanitizeProviderFailureClassification(
		providers.ExecuteFailureKind("untrusted"),
		providers.ContinuationFailureKind("untrusted"),
		providers.ContinuationOutcome("untrusted"),
	)
	if providerKind != "" || continuationKind != "" || outcome != "" {
		t.Fatalf("SanitizeProviderFailureClassification() = %q/%q/%q, want empty safe values", providerKind, continuationKind, outcome)
	}
}

func TestFailureCause_Validate_RejectsEmptyAndUnboundedDetail(t *testing.T) {
	tests := []struct {
		name   string
		detail string
	}{
		{name: "empty", detail: ""},
		{name: "whitespace", detail: " \t\n"},
		{name: "untrimmed", detail: " failure "},
		{name: "too long", detail: strings.Repeat("x", workersessions.MaxFailureCauseDetailRunes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := (workersessions.FailureCause{
				Kind:   workersessions.FailureCauseWorkersExecutionFailure,
				Detail: test.detail,
			}).Validate()
			if !errors.Is(err, workersessions.ErrInvalidFailureCause) {
				t.Fatalf("Validate() = %v, want ErrInvalidFailureCause", err)
			}
		})
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
				Cause:   &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure, Detail: "execution failed"},
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
			name: "failed with an unknown continuation classification is rejected",
			result: workersessions.TerminalResult{
				Outcome: workersessions.TerminalOutcomeFailed,
				Cause: &workersessions.FailureCause{
					Kind:                            workersessions.FailureCauseWorkersExecutionFailure,
					Detail:                          "continuation classification was unknown",
					ProviderContinuationFailureKind: providers.ContinuationFailureKind("UNKNOWN"),
				},
			},
			wantErr: workersessions.ErrInvalidFailureCause,
		},
		{
			name: "completed with a cause is rejected",
			result: workersessions.TerminalResult{
				Outcome: workersessions.TerminalOutcomeCompleted,
				Cause:   &workersessions.FailureCause{Kind: workersessions.FailureCauseExecutorPanic, Detail: "executor failed"},
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
