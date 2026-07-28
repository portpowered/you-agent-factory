package dashboard

import (
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestFormatDashboardWorkTypeCounts(t *testing.T) {
	t.Parallel()

	if got := formatDashboardWorkTypeCounts(nil); got != "" {
		t.Fatalf("empty counts = %q, want empty string", got)
	}
	if got := formatDashboardWorkTypeCounts(map[string]int{"beta": 2, "alpha": 1}); got != "  (alpha=1, beta=2)" {
		t.Fatalf("sorted counts = %q, want ordered breakdown", got)
	}
}

func TestDisplayCompletedDispatchStatus(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		string(workerexecution.OutcomeAccepted):  "Success",
		string(workerexecution.OutcomeContinue):  "Continue",
		string(workerexecution.OutcomeRejected):  "Rejected",
		string(workerexecution.OutcomeFailed):    "Failed",
		"mystery":                                "Unknown",
	}
	for outcome, want := range cases {
		if got := displayCompletedDispatchStatus(outcome); got != want {
			t.Fatalf("displayCompletedDispatchStatus(%q) = %q, want %q", outcome, got, want)
		}
	}
}
