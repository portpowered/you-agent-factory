package review

import (
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Story 001 characterization map (reconciled against the recovered P022
// inventory):
//
//   - TestPackagedReviewApprovalCompletes,
//     TestPackagedReviewRejectionCarriesFeedback,
//     TestPackagedReviewThreeCleanRejectionsDoNotTripFailureBreaker, and
//     TestPackagedReviewRejectionHonorsMaterializedAndFlaggedProviderSettings
//     -> CLI terminal response/primary result, controlled provider call count
//     and prompt feedback, and provider/model selection. Eligible for
//     packaged-review-public-outcomes; fidelity is local-real root composition
//     and test-owned filesystem with a controlled ProviderCommandRunner. The
//     settings test has three eligible configuration cells.
//   - TestPackagedReviewMalformedDecisionEnvelopeUsesCanonicalFailurePath ->
//     failed CLI response and reviewable-work:failed Work state for non-JSON
//     and unsupported decision output. Retained as isolated-with-reason in
//     packaged-review-envelope-validation because malformed provider output is
//     the edge under test; fidelity is a controlled runner over local-real root
//     composition.
//   - TestPackagedReviewDecisionEnvelopeValidatesRecordedOutputWork -> failed
//     CLI response for an invalid recorded_output_work state. Retained as
//     isolated-with-reason in packaged-review-envelope-validation because
//     envelope validation is the edge under test; fidelity is controlled
//     ProviderCommandRunner output over local-real root composition.
//   - TestPackagedReviewRetryExhaustionFails -> failed CLI response,
//     reviewable-work:failed Work state, and no completed primary result.
//     Retained as isolated-with-reason in packaged-review-command-failure
//     because the command-runner failure is the injected failure witness.
//
// Current baseline identity/resource ownership: each invocation creates its
// own root.BuildProcess, uses the implicit default Factory Session route, and
// supplies a t.TempDir home and working directory. No current default row
// opens or closes an explicit session, selector, or durable runtime identity,
// and no row directly asserts Factory Event or replay history. The eligible
// migration must allocate unique Work/request/selector/home/workspace/
// Factory/runtime identities; retained rows keep their failure fidelity. The
// testing package owns every temporary path, and story 004 owns the direct
// process/session/runtime census. Existing assertions are characterization and
// must remain intact. The functionallong TestCodeReviewLoop is mapped in its
// own file and is excluded from this seven-row default inventory.

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

// TestPackagedReviewRejectionCarriesFeedback proves packaged @you/review
// invocation carries the original request, rejected prior candidate, and
// reviewer feedback into the revised work attempt after a first-pass rejection,
// then completes with the later approved candidate as the primary result.
func TestPackagedReviewRejectionCarriesFeedback(t *testing.T) {
	submitted := "write release notes"
	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("first candidate")},
		platformprocess.CommandResult{Stdout: []byte(`{"decision":"REJECTED","feedback":"add the missing release date"}`)},
		platformprocess.CommandResult{Stdout: []byte("revised candidate")},
		platformprocess.CommandResult{Stdout: []byte(`{"decision":"ACCEPTED","output":"approved revised candidate"}`)},
	)

	response := runPackagedReviewCLIJSONInvocation(t, runner, submitted)
	assertPackagedReviewCompletedWithText(t, response, "approved revised candidate")
	if invocationPrimaryResultText(t, response) == submitted {
		t.Fatal("primaryResult echoed submitted request text")
	}
	if got := runner.CallCount(); got != 4 {
		t.Fatalf("provider invocation count = %d, want work, review, revised work, review", got)
	}

	requests := runner.Requests()
	if len(requests) != 4 {
		t.Fatalf("recorded provider requests = %d, want 4", len(requests))
	}
	secondWorkPrompt := providerCommandPrompt(requests[2])
	if !strings.Contains(secondWorkPrompt, submitted) ||
		!strings.Contains(secondWorkPrompt, "first candidate") ||
		!strings.Contains(secondWorkPrompt, "add the missing release date") {
		t.Fatalf(
			"revised work prompt = %q, want request, rejected candidate, and review feedback",
			secondWorkPrompt,
		)
	}
}

// TestPackagedReviewThreeCleanRejectionsDoNotTripFailureBreaker proves that
// explicit, cleanly exited reviewer decisions remain on the authored
// rejection route even when legacy stop words and a consecutive-failure limit
// are present on the materialized workstation configuration.
func TestPackagedReviewThreeCleanRejectionsDoNotTripFailureBreaker(t *testing.T) {
	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("first candidate")},
		platformprocess.CommandResult{Stdout: []byte(`{"decision":"REJECTED","feedback":"add the release date"}`)},
		platformprocess.CommandResult{Stdout: []byte("second candidate")},
		platformprocess.CommandResult{Stdout: []byte(`{"decision":"REJECTED","feedback":"add the owner"}`)},
		platformprocess.CommandResult{Stdout: []byte("third candidate")},
		platformprocess.CommandResult{Stdout: []byte(`{"decision":"REJECTED","feedback":"add the rollback plan"}`)},
		platformprocess.CommandResult{Stdout: []byte("fourth candidate")},
		platformprocess.CommandResult{Stdout: []byte(`{"decision":"ACCEPTED","output":"approved fourth candidate"}`)},
	)

	response := runPackagedReviewCLIJSONInvocationWithFactorySetup(
		t,
		runner,
		configurePackagedReviewRejectionClassification,
		"write the release notes",
	)
	assertPackagedReviewCompletedWithText(t, response, "approved fourth candidate")
	if got := runner.CallCount(); got != 8 {
		t.Fatalf("provider invocation count = %d, want four work/review round trips", got)
	}

	requests := runner.Requests()
	for index, wantFeedback := range map[int]string{
		2: "add the release date",
		4: "add the owner",
		6: "add the rollback plan",
	} {
		if prompt := providerCommandPrompt(requests[index]); !strings.Contains(prompt, wantFeedback) {
			t.Fatalf("work prompt %d = %q, want reviewer feedback %q", index, prompt, wantFeedback)
		}
	}
}

// TestPackagedReviewRejectionHonorsMaterializedAndFlaggedProviderSettings proves
// packaged @you/review rejection-then-approval through the public CLI honors
// materialized worker model configuration and default worker model provider
// flags when routing work and review dispatches.
func TestPackagedReviewRejectionHonorsMaterializedAndFlaggedProviderSettings(t *testing.T) {
	for _, tc := range []struct {
		name             string
		configure        func(t *testing.T, factoryDir string)
		operatorArgs     []string
		expectedProvider string
		expectedModel    string
	}{
		{name: "defaults"},
		{
			name:             "materialized_configuration",
			expectedProvider: "codex",
			expectedModel:    "configured-codex-model",
			configure: func(t *testing.T, factoryDir string) {
				setPackagedReviewWorkerModel(t, factoryDir, "configured-codex-model")
			},
		},
		{
			name:             "run_provider_model_flags",
			expectedProvider: "codex",
			expectedModel:    "flag-codex-model",
			operatorArgs: []string{
				"--provider", "CODEX",
				"--model", "flag-codex-model",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := support.NewShapedProviderCommandRunner(
				platformprocess.CommandResult{Stdout: []byte("candidate work")},
				platformprocess.CommandResult{Stdout: []byte(`{"decision":"REJECTED","feedback":"add the release date"}`)},
				platformprocess.CommandResult{Stdout: []byte("revised candidate")},
				platformprocess.CommandResult{Stdout: []byte(`{"decision":"ACCEPTED","output":"approved revised candidate"}`)},
			)

			response := runPackagedReviewCLIJSONInvocationWithFactorySetup(
				t,
				runner,
				tc.configure,
				"write the release notes",
				tc.operatorArgs...,
			)
			assertPackagedReviewCompletedWithText(t, response, "approved revised candidate")
			if got := runner.CallCount(); got != 4 {
				t.Fatalf("provider invocation count = %d, want work/review followed by revised work/review", got)
			}
			assertPackagedReviewProviderInvocations(t, runner.Requests(), tc.expectedProvider, tc.expectedModel)
		})
	}
}

// TestPackagedReviewRetryExhaustionFails proves packaged @you/review invocation
// returns a failed public terminal outcome with no completed success primary
// result when the edge-mocked provider cannot satisfy the packaged approval gate,
// for example because work dispatch fails before review can approve output.
//
// Retained-edge fidelity: the controlled command failure is the property under
// test, so this remains an isolated root-built cell with no fallback to a
// successful provider result. Its t.TempDir home and working directory own
// cleanup.
func TestPackagedReviewRetryExhaustionFails(t *testing.T) {
	submitted := "customer request"
	runner := packagedReviewFailingCommandRunner{}

	response, _, execErr := runPackagedReviewCLIJSONFailureInvocation(t, runner, submitted)
	if execErr == nil {
		t.Fatal("Process.Execute error = nil, want terminal packaged-review provider failure")
	}
	assertPackagedReviewBoundedFailureFailed(t, response)
}
