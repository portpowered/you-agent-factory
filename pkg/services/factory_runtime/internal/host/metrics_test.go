package host_test

import (
	"testing"
	"time"

	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestScriptMetricHelpers_PreferFailureMetadataAndDiagnostics(t *testing.T) {
	t.Parallel()

	timeoutResult := workerexecution.WorkResult{
		FailureMetadata: &workerexecution.WorkFailureMetadata{Type: workerexecution.WorkFailureTypeTimeout},
	}
	if !factoryhost.ScriptMetricTimedOut(timeoutResult) {
		t.Fatal("expected timeout result to report timed out")
	}
	if got := factoryhost.ScriptMetricFailureReason(timeoutResult); got != string(workerexecution.WorkFailureTypeTimeout) {
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
	if got := factoryhost.ScriptMetricFailureReason(commandResult); got != "exit_code" {
		t.Fatalf("failure reason = %q, want exit_code", got)
	}
	if duration, ok := factoryhost.ScriptMetricDurationMilliseconds(commandResult); !ok || duration != 250 {
		t.Fatalf("command duration = %v, %v want 250, true", duration, ok)
	}

	outcomeResult := workerexecution.WorkResult{
		Outcome: workerexecution.OutcomeContinue,
		Metrics: workerexecution.WorkMetrics{Duration: 125 * time.Millisecond},
	}
	if duration, ok := factoryhost.ScriptMetricDurationMilliseconds(outcomeResult); !ok || duration != 125 {
		t.Fatalf("metrics duration = %v, %v want 125, true", duration, ok)
	}
	if got := factoryhost.ScriptMetricFailureReason(outcomeResult); got != string(workerexecution.OutcomeContinue) {
		t.Fatalf("fallback failure reason = %q, want %q", got, workerexecution.OutcomeContinue)
	}
}
