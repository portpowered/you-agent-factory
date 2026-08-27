package review

import (
	"strings"
	"testing"
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
//   - TestPackagedReviewMalformedDecisionEnvelopeUsesCanonicalFailurePath and
//     TestPackagedReviewDecisionEnvelopeValidatesRecordedOutputWork -> failed
//     explicit-session response and reviewable-work:failed Work state for
//     malformed or invalid recorded decision-envelope output. The shared host
//     retains each input edge's exact controlled bytes behind a unique selector
//     and session; no invalid envelope is converted into a normal success.
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

// TestPackagedReviewSharedProcess proves compatible public-outcome scenarios
// share one root-built process while retaining explicit unique Factory Sessions,
// workspace selectors, and public Work/Event/replay evidence for each run.
func TestPackagedReviewSharedProcess(t *testing.T) {
	t.Run("ApprovalCompletes", func(t *testing.T) {
		t.Parallel()
		submitted := "customer request"
		runner := &packagedReviewCommandRunner{acceptedOutput: "approved candidate work"}
		scenario := openPackagedReviewScenario(t, runner, "shared-approval-review", nil)
		response := invokePackagedReviewSession(t, scenario, map[string]any{"input": submitted})
		assertPackagedReviewCompletedWithText(t, response, "approved candidate work")
		if invocationPrimaryResultText(t, response) == submitted {
			t.Fatal("primaryResult echoed submitted request text")
		}
		if got := len(runner.Requests()); got != 2 {
			t.Fatalf("provider invocation count = %d, want work then review", got)
		}
		assertPackagedReviewSharedEvidence(t, scenario, runner, "approved")
	})

	t.Run("RejectionCarriesFeedback", func(t *testing.T) {
		t.Parallel()
		submitted := "write release notes"
		runner := &packagedReviewCommandRunner{
			rejectReviews:      1,
			rejectionFeedbacks: []string{"add the missing release date"},
			acceptedOutput:     "approved revised candidate",
		}
		scenario := openPackagedReviewScenario(t, runner, "shared-feedback-review", nil)
		response := invokePackagedReviewSession(t, scenario, map[string]any{"input": submitted})
		assertPackagedReviewCompletedWithText(t, response, "approved revised candidate")
		if invocationPrimaryResultText(t, response) == submitted {
			t.Fatal("primaryResult echoed submitted request text")
		}
		if got := len(runner.Requests()); got != 4 {
			t.Fatalf("recorded provider requests = %d, want work, review, revised work, review", got)
		}
		secondWorkPrompt := providerCommandPrompt(runner.Requests()[2])
		if !strings.Contains(secondWorkPrompt, submitted) ||
			!strings.Contains(secondWorkPrompt, "first candidate") ||
			!strings.Contains(secondWorkPrompt, "add the missing release date") {
			t.Fatalf(
				"revised work prompt = %q, want request, rejected candidate, and review feedback",
				secondWorkPrompt,
			)
		}
		assertPackagedReviewSharedEvidence(t, scenario, runner, "approved")
	})

	t.Run("ThreeCleanRejectionsDoNotTripFailureBreaker", func(t *testing.T) {
		t.Parallel()
		runner := &packagedReviewCommandRunner{
			rejectReviews: 3,
			rejectionFeedbacks: []string{
				"add the release date",
				"add the owner",
				"add the rollback plan",
			},
			acceptedOutput: "approved fourth candidate",
		}
		scenario := openPackagedReviewScenario(
			t,
			runner,
			"shared-clean-rejections-review",
			configurePackagedReviewRejectionClassification,
		)
		response := invokePackagedReviewSession(t, scenario, map[string]any{"input": "write the release notes"})
		assertPackagedReviewCompletedWithText(t, response, "approved fourth candidate")
		if got := runner.ReviewerCalls(); got != 4 {
			t.Fatalf("reviewer call count = %d, want four clean review decisions", got)
		}
		requests := runner.Requests()
		if got := len(requests); got != 8 {
			t.Fatalf("provider invocation count = %d, want four work/review round trips", got)
		}
		for index, wantFeedback := range map[int]string{
			2: "add the release date",
			4: "add the owner",
			6: "add the rollback plan",
		} {
			if prompt := providerCommandPrompt(requests[index]); !strings.Contains(prompt, wantFeedback) {
				t.Fatalf("work prompt %d = %q, want reviewer feedback %q", index, prompt, wantFeedback)
			}
		}
		assertPackagedReviewSharedEvidence(t, scenario, runner, "approved")
	})

	t.Run("MalformedDecisionEnvelopeUsesCanonicalFailurePath", func(t *testing.T) {
		t.Parallel()
		exercisePackagedReviewMalformedDecisionEnvelope(t)
	})

	t.Run("DecisionEnvelopeValidatesRecordedOutputWork", func(t *testing.T) {
		t.Parallel()
		exercisePackagedReviewRecordedOutputWorkValidation(t)
	})

	t.Run("RejectionHonorsMaterializedAndFlaggedProviderSettings", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name             string
			configure        func(t *testing.T, factoryDir string)
			invocationArgs   map[string]any
			expectedProvider string
			expectedModel    string
		}{
			{
				name:             "defaults",
				expectedProvider: "codex",
				expectedModel:    "operator-configured-model",
			},
			{
				name:             "materialized_configuration",
				expectedProvider: "codex",
				expectedModel:    "configured-codex-model",
				configure: func(t *testing.T, factoryDir string) {
					setPackagedReviewWorkerModel(t, factoryDir, "configured-codex-model")
				},
			},
			{
				name: "run_provider_model_flags",
				invocationArgs: map[string]any{
					"writerProvider":   "CODEX",
					"writerModel":      "flag-codex-model",
					"reviewerProvider": "CODEX",
					"reviewerModel":    "flag-codex-model",
				},
				expectedProvider: "codex",
				expectedModel:    "flag-codex-model",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				runner := &packagedReviewCommandRunner{
					rejectReviews:      1,
					rejectionFeedbacks: []string{"add the missing release date"},
					acceptedOutput:     "approved revised candidate",
				}
				scenario := openPackagedReviewScenario(
					t,
					runner,
					"shared-settings-review-"+strings.ReplaceAll(tc.name, "_", "-"),
					tc.configure,
				)
				args := map[string]any{"input": "write the release notes"}
				for key, value := range tc.invocationArgs {
					args[key] = value
				}
				response := invokePackagedReviewSession(t, scenario, args)
				assertPackagedReviewCompletedWithText(t, response, "approved revised candidate")
				if got := len(runner.Requests()); got != 4 {
					t.Fatalf("provider invocation count = %d, want work/review followed by revised work/review", got)
				}
				assertPackagedReviewProviderInvocations(t, runner.Requests(), tc.expectedProvider, tc.expectedModel)
				assertPackagedReviewSharedEvidence(t, scenario, runner, "approved")
			})
		}
	})
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
