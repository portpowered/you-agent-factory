package review

import (
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPackagedReviewDecisionEnvelopeValidatesRecordedOutputWork proves an
// AGENT_RUN decision-envelope reviewer still has its recorded_output_work
// parsed and validated. The envelope below accepts the candidate while
// recording a work item in "complete", a state the packaged @you/review factory
// never authors (its reviewable-work states are init, in-review, approved, and
// failed). Runtime must reject that recorded work instead of silently
// completing the invocation as if the reviewer had recorded nothing.
//
// Retained-edge fidelity: invalid recorded_output_work is a malformed decision
// envelope witness, not a normal shared outcome. Keep the exact controlled
// ProviderCommandRunner bytes and the isolated t.TempDir cleanup boundary.
func TestPackagedReviewDecisionEnvelopeValidatesRecordedOutputWork(t *testing.T) {
	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("candidate work")},
		platformprocess.CommandResult{Stdout: []byte(
			`{"decision":"ACCEPTED","output":"approved candidate work",` +
				`"recorded_output_work":[{"workTypeId":"reviewable-work","state":"complete",` +
				`"content":[{"type":"text","text":"recorded artifact"}]}]}`,
		)},
	)

	response, _, execErr := runPackagedReviewCLIJSONFailureInvocation(t, runner, "customer request")
	if execErr == nil {
		t.Fatalf(
			"Process.Execute error = nil, want failure for recorded_output_work in an unauthored state; status = %q",
			response.Status,
		)
	}
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED", response.Status)
	}
}
