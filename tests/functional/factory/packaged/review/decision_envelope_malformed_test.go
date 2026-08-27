package review

import (
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPackagedReviewMalformedDecisionEnvelopeUsesCanonicalFailurePath proves the
// AGENT_RUN detached dispatch routes a malformed reviewer envelope through the
// Factory Definitions owner of decision-envelope interpretation rather than a
// Workers-local parser.
//
// Both reviewer responses below are unreadable as a decision envelope: the
// first is not JSON at all, the second is well-formed JSON carrying a decision
// outside the authored vocabulary. Only the canonical
// FailedWorkResultFromDecisionEnvelopeError path handles both, so this test is
// the functional coverage witness for that path and for the
// completion-validation diagnostics it attaches. The exact failure
// classification is pinned at the seam by
// TestExecuteKeepsCanonicalMalformedDecisionEnvelopeFailure in
// pkg/services/workers/internal/services/runners/internal/services/agent/internal/service.
//
// Retained-edge fidelity: malformed and unsupported reviewer output is the
// injected input edge, so these cells remain isolated and use the controlled
// ProviderCommandRunner only to deliver the exact invalid bytes. Each cell's
// t.TempDir home/working directory owns cleanup.
func TestPackagedReviewMalformedDecisionEnvelopeUsesCanonicalFailurePath(t *testing.T) {
	tests := []struct {
		name     string
		envelope string
	}{
		{name: "not json", envelope: "the candidate looks fine to me"},
		{name: "unsupported decision", envelope: `{"decision":"MAYBE","feedback":"reviewer note"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := support.NewShapedProviderCommandRunner(
				platformprocess.CommandResult{Stdout: []byte("candidate work")},
				platformprocess.CommandResult{Stdout: []byte(test.envelope)},
			)

			response, _, execErr := runPackagedReviewCLIJSONFailureInvocation(t, runner, "customer request")
			if execErr == nil {
				t.Fatalf(
					"Process.Execute error = nil, want failure for an unreadable decision envelope; status = %q",
					response.Status,
				)
			}
			assertPackagedReviewBoundedFailureFailed(t, response)
		})
	}
}
