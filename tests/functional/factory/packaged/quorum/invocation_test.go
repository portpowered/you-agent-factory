package quorum

import (
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
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

// TestPackagedQuorumOptionalMemberSettingsReachWorkers proves that optional
// branch and merge provider/model settings supplied through the public CLI
// reach the quorum branch and merge workers under edge-mocked Codex providers
// and complete with a merged primary result reflecting the submitted request.
func TestPackagedQuorumOptionalMemberSettingsReachWorkers(t *testing.T) {
	requestText := "configured quorum optional member settings request"
	runner := newPackagedQuorumCommandRunner()

	response := runPackagedQuorumCLIJSONInvocation(
		t,
		runner,
		requestText,
		"--branch-provider", "CODEX",
		"--branch-model", "gpt-5.1",
		"--merge-provider", "CODEX",
		"--merge-model", "gpt-5.2",
	)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}

	for _, workstation := range []string{
		packagedQuorumBranchAWorkstation,
		packagedQuorumBranchBWorkstation,
		packagedQuorumMergeWorkstation,
	} {
		if got := runner.callCount(workstation); got != 1 {
			t.Fatalf("%s provider call count = %d, want one configured quorum dispatch", workstation, got)
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

	codex := string(modelprovider.ProviderCodex)
	runner.assertProviderModel(t, packagedQuorumBranchAWorkstation, codex, "gpt-5.1")
	runner.assertProviderModel(t, packagedQuorumBranchBWorkstation, codex, "gpt-5.1")
	runner.assertProviderModel(t, packagedQuorumMergeWorkstation, codex, "gpt-5.2")
}

// TestPackagedQuorumInsufficientSuccessfulMembersFails proves that packaged
// @you/quorum invocation returns a failed public terminal outcome when fewer
// than the required branch members succeed, without emitting a completed success
// primary result for the failing run.
func TestPackagedQuorumInsufficientSuccessfulMembersFails(t *testing.T) {
	requestText := "functional packaged quorum insufficient successful members request"
	runner := newPackagedQuorumBranchBFailingCommandRunner()

	response, _, execErr := runPackagedQuorumCLIJSONFailureInvocation(t, runner, requestText)
	if execErr == nil {
		t.Fatal("Process.Execute error = nil, want terminal packaged-quorum branch failure")
	}
	assertPackagedQuorumInsufficientSuccessfulMembersFailed(t, response)

	if got := runner.callCount(packagedQuorumBranchAWorkstation); got != 1 {
		t.Fatalf("%s provider call count = %d, want one successful branch dispatch", packagedQuorumBranchAWorkstation, got)
	}
	if got := runner.callCount(packagedQuorumBranchBWorkstation); got < 1 {
		t.Fatalf("%s provider call count = %d, want at least one failing branch dispatch", packagedQuorumBranchBWorkstation, got)
	}
	if got := runner.callCount(packagedQuorumMergeWorkstation); got != 0 {
		t.Fatalf("%s provider call count = %d, want no merge dispatch before both branches succeed", packagedQuorumMergeWorkstation, got)
	}
}
