package review

import "testing"

// exercisePackagedReviewMalformedDecisionEnvelope proves the
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
// Retained-edge fidelity: malformed and unsupported reviewer output remains the
// injected input edge. The shared host does not replace that edge: each cell
// has its own selector, copied Factory, explicit Factory Session, and exact
// controlled ProviderCommandRunner bytes.
func exercisePackagedReviewMalformedDecisionEnvelope(t *testing.T) {
	t.Helper()
	tests := []struct {
		name     string
		envelope string
	}{
		{name: "not json", envelope: "the candidate looks fine to me"},
		{name: "unsupported decision", envelope: `{"decision":"MAYBE","feedback":"reviewer note"}`},
	}
	for _, test := range tests {
		runner := &packagedReviewCommandRunner{reviewOutputs: []string{test.envelope}}
		scenario := openPackagedReviewScenario(t, runner, "shared-malformed-review-"+test.name, nil)
		response := invokePackagedReviewSession(t, scenario, map[string]any{"input": "customer request"})
		assertPackagedReviewBoundedFailureFailed(t, response)
		assertPackagedReviewSharedEvidence(t, scenario, runner, "failed")
	}
}
