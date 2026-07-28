package review

import (
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPackagedReviewApprovalCompletes proves packaged @you/review invocation
// completes through the public CLI under edge-mocked Codex providers when the
// independent reviewer accepts on the first pass, dispatches work then review,
// and returns the approved candidate output as the primary result.
func TestPackagedReviewApprovalCompletes(t *testing.T) {
	submitted := "customer request"
	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("candidate work")},
		platformprocess.CommandResult{Stdout: []byte(`{"decision":"ACCEPTED","output":"approved candidate work"}`)},
	)

	response := runPackagedReviewCLIJSONInvocation(t, runner, submitted)
	assertPackagedReviewCompletedWithText(t, response, "approved candidate work")
	if invocationPrimaryResultText(t, response) == submitted {
		t.Fatal("primaryResult echoed submitted request text")
	}
	if got := runner.CallCount(); got != 2 {
		t.Fatalf("provider invocation count = %d, want work then review", got)
	}
}
