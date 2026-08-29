package quorum

import (
	"testing"
	"time"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestPackagedQuorum(t *testing.T) {
	fixture := newPackagedQuorumSharedFixture(t)
	t.Run("TestPackagedQuorumRequiredInputCompletes", func(t *testing.T) {
		testPackagedQuorumRequiredInputCompletes(t, fixture)
	})
	t.Run("TestPackagedQuorumOptionalMemberSettingsReachWorkers", func(t *testing.T) {
		testPackagedQuorumOptionalMemberSettingsReachWorkers(t, fixture)
	})
	t.Run("TestPackagedQuorumGatesMergeUntilBothBranchesComplete", func(t *testing.T) {
		testPackagedQuorumGatesMergeUntilBothBranchesComplete(t, fixture)
	})
	t.Run("TestPackagedQuorumInsufficientSuccessfulMembersFails", func(t *testing.T) {
		testPackagedQuorumInsufficientSuccessfulMembersFails(t, fixture)
	})
	t.Run("reuse after insufficient quorum", func(t *testing.T) {
		testPackagedQuorumReusesProcessAfterInsufficientMembers(t, fixture)
	})
}

// testPackagedQuorumRequiredInputCompletes proves that invoking the packaged
// @you/quorum Factory with only the required text request completes through the
// public Factory Session API under edge-mocked Codex providers, dispatches both independent
// branch members, and returns one merged primary result reflecting the
// submitted request.
func testPackagedQuorumRequiredInputCompletes(
	t *testing.T,
	fixture *packagedQuorumSharedFixture,
) {
	requestText := packagedQuorumRequestText(t)
	runner := newPackagedQuorumCommandRunner()
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)

	response := runPackagedQuorumInvocation(t, scenario, map[string]any{"input": requestText})
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

// testPackagedQuorumOptionalMemberSettingsReachWorkers proves that optional
// branch and merge provider/model settings supplied through the public Factory Session API
// reach the quorum branch and merge workers under edge-mocked Codex providers
// and complete with a merged primary result reflecting the submitted request.
func testPackagedQuorumOptionalMemberSettingsReachWorkers(
	t *testing.T,
	fixture *packagedQuorumSharedFixture,
) {
	requestText := "configured quorum optional member settings request"
	runner := newPackagedQuorumCommandRunner()
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)

	response := runPackagedQuorumInvocation(
		t,
		scenario,
		map[string]any{
			"input":          requestText,
			"branchProvider": "CODEX",
			"branchModel":    "gpt-5.1",
			"mergeProvider":  "CODEX",
			"mergeModel":     "gpt-5.2",
		},
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

// testPackagedQuorumGatesMergeUntilBothBranchesComplete proves packaged
// @you/quorum invocation does not dispatch merge until both branch members
// complete under edge-mocked Codex providers and then returns one merged
// primary result reflecting the submitted request.
func testPackagedQuorumGatesMergeUntilBothBranchesComplete(
	t *testing.T,
	fixture *packagedQuorumSharedFixture,
) {
	runner := newPackagedQuorumGatedCommandRunner()
	requestText := packagedQuorumRequestText(t)
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)

	responseCh := make(chan factoryapi.InvocationResponse, 1)
	go func() {
		responseCh <- runPackagedQuorumInvocation(t, scenario, map[string]any{"input": requestText})
	}()

	runner.waitForBranchStarts(t)
	if got := runner.callCount(packagedQuorumMergeWorkstation); got != 0 {
		t.Fatalf("merge call count before branch B release = %d, want 0", got)
	}
	close(runner.releaseBranchB)

	select {
	case response := <-responseCh:
		if response.Status != factoryapi.InvocationTerminalStatusCompleted {
			t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
		}
		assertMergedQuorumPrimaryResult(t, invocationPrimaryResultText(t, response), requestText)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for gated quorum invocation to complete")
	}
	if got := runner.callCount(packagedQuorumMergeWorkstation); got != 1 {
		t.Fatalf("merge call count = %d, want exactly 1", got)
	}
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

// testPackagedQuorumInsufficientSuccessfulMembersFails proves that packaged
// @you/quorum invocation returns a failed public terminal outcome when fewer
// than the required branch members succeed, without emitting a completed success
// primary result for the failing run.
func testPackagedQuorumInsufficientSuccessfulMembersFails(
	t *testing.T,
	fixture *packagedQuorumSharedFixture,
) {
	requestText := "functional packaged quorum insufficient successful members request"
	runner := newPackagedQuorumBranchBFailingCommandRunner()
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)

	response := runPackagedQuorumInvocation(t, scenario, map[string]any{"input": requestText})
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

func testPackagedQuorumReusesProcessAfterInsufficientMembers(
	t *testing.T,
	fixture *packagedQuorumSharedFixture,
) {
	requestText := "reuse the quorum process after an insufficient-member failure"
	runner := newPackagedQuorumCommandRunner()
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)
	response := runPackagedQuorumInvocation(t, scenario, map[string]any{"input": requestText})
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("response = %#v, want completed invocation after prior quorum failure", response)
	}
	assertMergedQuorumPrimaryResult(t, invocationPrimaryResultText(t, response), requestText)
	for _, workstation := range []string{
		packagedQuorumBranchAWorkstation,
		packagedQuorumBranchBWorkstation,
		packagedQuorumMergeWorkstation,
	} {
		if got := runner.callCount(workstation); got != 1 {
			t.Fatalf("%s provider call count = %d, want one dispatch after reuse", workstation, got)
		}
	}
}
