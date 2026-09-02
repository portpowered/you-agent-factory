package review

import (
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
// Current migration identity/resource ownership: eligible rows share one
// root-built continuous host, but each scenario owns a copied Factory,
// explicit Factory Session, selector, request, and temporary home/workspace.
// The eligible API invocation is a narrowly scoped CLI-plus-API parity
// exception: the packaged Petri Factory must run in its already-open session
// so the tests can inspect canonical Work/Event/dispatch/replay history, while
// public run has no session-target flag and its remote durable source only
// resolves JavaScript workflow factories. The shared-process
// CLIResponseMatchesExplicitSession cell still executes the same success via
// the root-built customer CLI and compares terminal response and primary
// result. Retained command-failure and separate functionallong rows keep their
// original isolated resource ownership. Existing assertions are
// customer-observable assertions remain intact.

// TestPackagedReviewSharedProcess proves compatible public-outcome scenarios
// share one root-built process while retaining explicit unique Factory Sessions,
// workspace selectors, and public Work/Event/replay evidence for each run.
func TestPackagedReviewSharedProcess(t *testing.T) {
	t.Parallel()
	sharedPackagedReviewFixture(t)
	t.Run("ApprovalCompletes", testPackagedReviewApprovalCompletes)
	t.Run("RejectionCarriesFeedback", testPackagedReviewRejectionCarriesFeedback)
	t.Run("ThreeCleanRejectionsDoNotTripFailureBreaker", testPackagedReviewThreeCleanRejections)
	t.Run("MalformedDecisionEnvelopeUsesCanonicalFailurePath", testPackagedReviewMalformedDecisionEnvelope)
	t.Run("DecisionEnvelopeValidatesRecordedOutputWork", testPackagedReviewRecordedOutputWork)
	t.Run("RejectionHonorsMaterializedAndFlaggedProviderSettings", testPackagedReviewProviderSettings)
	t.Run("CLIResponseMatchesExplicitSession", testPackagedReviewCLIResponseParity)
}

func testPackagedReviewApprovalCompletes(t *testing.T) {
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
}

func testPackagedReviewRejectionCarriesFeedback(t *testing.T) {
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
}

func testPackagedReviewThreeCleanRejections(t *testing.T) {
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
}

func testPackagedReviewMalformedDecisionEnvelope(t *testing.T) {
	t.Parallel()
	exercisePackagedReviewMalformedDecisionEnvelope(t)
}

func testPackagedReviewRecordedOutputWork(t *testing.T) {
	t.Parallel()
	exercisePackagedReviewRecordedOutputWorkValidation(t)
}

func testPackagedReviewProviderSettings(t *testing.T) {
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
}

func testPackagedReviewCLIResponseParity(t *testing.T) {
	t.Parallel()
	request := "prove the packaged Review CLI response"

	apiRunner := &packagedReviewCommandRunner{acceptedOutput: "approved candidate work"}
	apiScenario := openPackagedReviewScenario(t, apiRunner, "shared-cli-api-parity-review", nil)
	apiResponse := invokePackagedReviewSession(t, apiScenario, map[string]any{
		"input": request,
	})
	if apiResponse.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("explicit-session response status = %q, want COMPLETED: %#v", apiResponse.Status, apiResponse)
	}
	assertPackagedReviewSharedEvidence(t, apiScenario, apiRunner, "approved")

	cliRunner := &packagedReviewCommandRunner{acceptedOutput: "approved candidate work"}
	cliResponse, stderr, err := runPackagedReviewCLIInvocation(
		t,
		cliRunner,
		request,
		"--writer-provider", "CODEX",
		"--writer-model", "operator-configured-model",
		"--reviewer-provider", "CODEX",
		"--reviewer-model", "operator-configured-model",
	)
	if err != nil {
		t.Fatalf("root-built customer CLI error = %v\nresponse = %#v\nstderr = %q", err, cliResponse, stderr)
	}
	if stderr != "" {
		t.Fatalf("root-built customer CLI stderr = %q, want empty successful-run stderr", stderr)
	}
	if cliResponse.Status != apiResponse.Status {
		t.Fatalf("customer CLI status = %q, explicit-session status = %q", cliResponse.Status, apiResponse.Status)
	}
	if got, want := invocationPrimaryResultText(t, cliResponse), invocationPrimaryResultText(t, apiResponse); got != want {
		t.Fatalf("customer CLI primary result = %q, explicit-session primary result = %q", got, want)
	}
	if got := strings.Join(cliRunner.Roles(), ","); got != "writer,reviewer" {
		t.Fatalf("customer CLI stage order = %q, want writer,reviewer", got)
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
	t.Parallel()
	submitted := "customer request"
	runner := packagedReviewFailingCommandRunner{}

	response, _, execErr := runPackagedReviewCLIInvocation(t, runner, submitted)
	if execErr == nil {
		t.Fatal("Process.Execute error = nil, want terminal packaged-review provider failure")
	}
	assertPackagedReviewBoundedFailureFailed(t, response)
}
