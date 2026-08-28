package review

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// exercisePackagedReviewRecordedOutputWorkValidation proves an
// AGENT_RUN decision-envelope reviewer still has its recorded_output_work
// parsed and validated. The envelope below accepts the candidate while
// recording a work item in "complete", a state the packaged @you/review factory
// never authors (its reviewable-work states are init, in-review, approved, and
// failed). Runtime must reject that recorded work instead of silently
// completing the invocation as if the reviewer had recorded nothing.
//

// Retained-edge fidelity: invalid recorded_output_work remains a malformed
// decision-envelope witness, not a normal shared outcome. Keep the exact
// controlled ProviderCommandRunner bytes and the per-scenario selector,
// explicit Factory Session, and t.TempDir cleanup boundary.
func exercisePackagedReviewRecordedOutputWorkValidation(t *testing.T) {
	t.Helper()
	runner := &packagedReviewCommandRunner{reviewOutputs: []string{
		`{"decision":"ACCEPTED","output":"approved candidate work",` +
			`"recorded_output_work":[{"workTypeId":"reviewable-work","state":"complete",` +
			`"content":[{"type":"text","text":"recorded artifact"}]}]}`,
	}}
	scenario := openPackagedReviewScenario(t, runner, "shared-recorded-output-work-review", nil)
	response := invokePackagedReviewSession(t, scenario, map[string]any{"input": "customer request"})
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED", response.Status)
	}
	assertPackagedReviewSharedEvidence(t, scenario, runner, "failed")
}
