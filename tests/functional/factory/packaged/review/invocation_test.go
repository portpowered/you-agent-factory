package review

import (
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

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
			name:             "model_provider_flags",
			expectedProvider: "gemini",
			expectedModel:    "flag-gemini-model",
			operatorArgs: []string{
				"--default-worker-model-provider", "GEMINI",
				"--default-worker-model", "flag-gemini-model",
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
func TestPackagedReviewRetryExhaustionFails(t *testing.T) {
	submitted := "customer request"
	runner := packagedReviewFailingCommandRunner{}

	response, _, execErr := runPackagedReviewCLIJSONFailureInvocation(t, runner, submitted)
	if execErr == nil {
		t.Fatal("Process.Execute error = nil, want terminal packaged-review provider failure")
	}
	assertPackagedReviewBoundedFailureFailed(t, response)
}
