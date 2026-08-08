package service

import (
	"strings"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestFailureDiagnosticsUseClosedVocabularyAndProviderFallbacks(t *testing.T) {
	primary := &workers.WorkDiagnostics{
		Metadata: map[string]string{
			workers.ProviderResponseMetadataFailureOperation:      "provider_session_ingestion",
			workers.ProviderResponseMetadataFailureClassification: "resource_limit",
			workers.ProviderResponseMetadataFailureStage:          "final_parse",
		},
	}
	if got := safeDiagnosticFailureContext(primary); got != "operation=provider_session_ingestion classification=resource_limit stage=final_parse" {
		t.Fatalf("safeDiagnosticFailureContext(primary) = %q, want all safe fields", got)
	}

	fallback := &workers.WorkDiagnostics{
		Provider: &workers.ProviderDiagnostic{ResponseMetadata: map[string]string{
			workers.ProviderResponseMetadataFailureOperation:      "provider_session_ingestion",
			workers.ProviderResponseMetadataFailureClassification: "resource_limit",
			workers.ProviderResponseMetadataFailureStage:          "final_parse",
		}},
	}
	if got := mergeDiagnosticContexts(&workers.WorkDiagnostics{}, fallback); got != "operation=provider_session_ingestion classification=resource_limit stage=final_parse" {
		t.Fatalf("mergeDiagnosticContexts(empty, fallback) = %q, want provider fields", got)
	}
	if got := safeDetailWithDiagnostics(
		workersessions.FailureCauseAdapterFailure,
		&workers.WorkFailureMetadata{Family: workers.WorkFailureFamilyTerminal, Type: workers.WorkFailureTypeUnknown},
		primary,
	); !strings.Contains(got, "operation=provider_session_ingestion") {
		t.Fatalf("safeDetailWithDiagnostics() = %q, want safe diagnostic context", got)
	}

	if got := diagnosticValue(nil, workers.ProviderResponseMetadataFailureOperation); got != "" {
		t.Fatalf("diagnosticValue(nil) = %q, want empty", got)
	}
	if got := diagnosticValue(&workers.WorkDiagnostics{}, workers.ProviderResponseMetadataFailureOperation); got != "" {
		t.Fatalf("diagnosticValue(empty) = %q, want empty", got)
	}
	if got := safeDiagnosticValue(strings.Repeat("x", 65), knownFailureOperations); got != "" {
		t.Fatalf("safeDiagnosticValue(overlong) = %q, want empty", got)
	}
	if got := safeDiagnosticValue("untrusted-value", knownFailureOperations); got != "" {
		t.Fatalf("safeDiagnosticValue(unrecognized) = %q, want empty", got)
	}
}

func TestBoundedFailureDetailUsesFallbackAndTruncates(t *testing.T) {
	unknownKind := workersessions.FailureCauseKind("unrecognized")
	if got := boundedFailureDetail(unknownKind, ""); got == "" {
		t.Fatal("boundedFailureDetail(unknown, empty) = empty, want fallback")
	}

	long := strings.Repeat("x", workersessions.MaxFailureCauseDetailRunes+10)
	if got := boundedFailureDetail(workersessions.FailureCauseAdapterFailure, long); len([]rune(got)) != workersessions.MaxFailureCauseDetailRunes {
		t.Fatalf("boundedFailureDetail() length = %d, want %d", len([]rune(got)), workersessions.MaxFailureCauseDetailRunes)
	}
}

func TestClassifyTerminal_ContradictorySuccessWithExecutorPanicUsesPanicCause(t *testing.T) {
	terminal := classifyTerminal(
		&workers.WorkerExecutorPanicError{Cause: "panic evidence"},
		workers.WorkstationDispatchResult{
			TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
			Result:          workers.WorkResult{Outcome: workers.OutcomeAccepted},
		},
	)
	if terminal.Cause == nil || terminal.Cause.Kind != workersessions.FailureCauseExecutorPanic {
		t.Fatalf("terminal cause = %#v, want EXECUTOR_PANIC", terminal.Cause)
	}
}

func TestTerminalDraft_FailedRequiresValidTerminalResult(t *testing.T) {
	if _, err := terminalDraft(workersessions.StateFailed, workersessions.TerminalResult{}, "dispatch-1"); err == nil {
		t.Fatal("terminalDraft(FAILED, zero result) error = nil, want validation error")
	}
}

func TestNormalizeCommittedTerminal_RepairsInvalidFailure(t *testing.T) {
	result := normalizeCommittedTerminal(workersessions.StateFailed, workersessions.TerminalResult{
		Outcome: workersessions.TerminalOutcomeCompleted,
	})
	if result.Outcome != workersessions.TerminalOutcomeFailed || result.Cause == nil {
		t.Fatalf("normalizeCommittedTerminal() = %#v, want FAILED with fallback cause", result)
	}
	if strings.TrimSpace(result.Cause.Detail) == "" {
		t.Fatal("normalized failure cause detail is empty")
	}
}
