package quorum

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// TestPackagedQuorumRequiredInputCompletes proves that invoking the packaged
// @you/quorum Factory with only the required text request completes through the
// public CLI under edge-mocked Codex providers, dispatches both independent
// branch members, and returns one merged primary result reflecting the
// submitted request.
func TestPackagedQuorumRequiredInputCompletes(t *testing.T) {
	requestText := packagedQuorumRequestText(t)
	runner := newPackagedQuorumCommandRunner()

	response := runPackagedQuorumCLIJSONInvocation(t, runner, requestText)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}

	for _, workstation := range []string{
		packagedQuorumBranchAWorkstation,
		packagedQuorumBranchBWorkstation,
		packagedQuorumMergeWorkstation,
	} {
		if got := runner.callCount(workstation); got != 1 {
			t.Fatalf("%s provider call count = %d, want one quorum dispatch", workstation, got)
		}
	}

	assertMergedQuorumPrimaryResult(t, invocationPrimaryResultText(t, response), requestText)
	assertPromptIncludes(
		t,
		runner.capturedMergePrompt(),
		"Original request:\n",
		requestText,
		"Branch A output:\n",
		"branch A",
		"Branch B output:\n",
		"branch B",
	)
}
