package service_test

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
)

func TestScriptMetricHelpers_PreferFailureMetadataAndDiagnostics(t *testing.T) {
	t.Parallel()

	timeoutResult := interfaces.WorkResult{
		FailureMetadata: &interfaces.WorkFailureMetadata{Type: interfaces.WorkFailureTypeTimeout},
	}
	if !factoryservice.ScriptMetricTimedOut(timeoutResult) {
		t.Fatal("expected timeout result to report timed out")
	}
	if got := factoryservice.ScriptMetricFailureReason(timeoutResult); got != string(interfaces.WorkFailureTypeTimeout) {
		t.Fatalf("failure reason = %q, want %q", got, interfaces.WorkFailureTypeTimeout)
	}

	commandResult := interfaces.WorkResult{
		Outcome: interfaces.OutcomeRejected,
		Diagnostics: &interfaces.WorkDiagnostics{
			Command: &interfaces.CommandDiagnostic{
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

	outcomeResult := interfaces.WorkResult{
		Outcome: interfaces.OutcomeContinue,
		Metrics: interfaces.WorkMetrics{Duration: 125 * time.Millisecond},
	}
	if duration, ok := factoryservice.ScriptMetricDurationMilliseconds(outcomeResult); !ok || duration != 125 {
		t.Fatalf("metrics duration = %v, %v want 125, true", duration, ok)
	}
	if got := factoryservice.ScriptMetricFailureReason(outcomeResult); got != string(interfaces.OutcomeContinue) {
		t.Fatalf("fallback failure reason = %q, want %q", got, interfaces.OutcomeContinue)
	}
}
