package service_test

import (
	"testing"
	"time"

	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

func TestScriptMetricHelpers_PreferFailureMetadataAndDiagnostics(t *testing.T) {
	t.Parallel()

	timeoutResult := workerexecution.WorkResult{
		FailureMetadata: &workerexecution.WorkFailureMetadata{Type: workerexecution.WorkFailureTypeTimeout},
	}
	if !factoryservice.ScriptMetricTimedOut(timeoutResult) {
		t.Fatal("expected timeout result to report timed out")
	}
	if got := factoryservice.ScriptMetricFailureReason(timeoutResult); got != string(workerexecution.WorkFailureTypeTimeout) {
		t.Fatalf("failure reason = %q, want %q", got, workerexecution.WorkFailureTypeTimeout)
	}

	commandResult := workerexecution.WorkResult{
		Outcome: workerexecution.OutcomeRejected,
		Diagnostics: &workerexecution.WorkDiagnostics{
			Command: &workerexecution.CommandDiagnostic{
				ExitCode: 7,
				Duration: 250 * time.Millisecond,
			},
		},
	}
	if got := factoryservice.ScriptMetricFailureReason(commandResult); got != "exit_code" {
		t.Fatalf("failure reason = %q, want exit_code", got)
	}
	if duration, ok := factoryservice.ScriptMetricDurationMilliseconds(commandResult); !ok || duration != 250 {
		t.Fatalf("command duration = %v, %v want 250, true", duration, ok)
	}

	outcomeResult := workerexecution.WorkResult{
		Outcome: workerexecution.OutcomeContinue,
		Metrics: workerexecution.WorkMetrics{Duration: 125 * time.Millisecond},
	}
	if duration, ok := factoryservice.ScriptMetricDurationMilliseconds(outcomeResult); !ok || duration != 125 {
		t.Fatalf("metrics duration = %v, %v want 125, true", duration, ok)
	}
	if got := factoryservice.ScriptMetricFailureReason(outcomeResult); got != string(workerexecution.OutcomeContinue) {
		t.Fatalf("fallback failure reason = %q, want %q", got, workerexecution.OutcomeContinue)
	}
}
