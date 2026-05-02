package functional_test

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// TestExecutorFailure_NoFailureArcs verifies that when a provider returns a Go
// error and the transition has no FailureArcs configured, the input token is
// routed to the work type's FAILED state place rather than being silently lost.
func TestExecutorFailure_NoFailureArcs(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "executor_failure_no_arcs"))
	testutil.WriteSeedFile(t, dir, "task", []byte("work payload"))

	// Provider returns a Go error, exercising the full error propagation path:
	// Provider error → AgentExecutor → OutcomeFailed → routing to FAILED place.
	provider := testutil.NewMockProviderWithErrors(
		[]interfaces.InferenceResponse{{}},
		[]error{errors.New("executor crashed")},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 5*time.Second)

	// Token should be in task:failed (fallback routing).
	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing")

	// Verify the failure record is set on the token.
	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.PlaceID == "task:failed" {
			if tok.History.LastError == "" {
				t.Error("expected LastError to be set on failed token")
			}
			break
		}
	}
}

// TestExecutorFailure_OutcomeFailed_NoFailureArcs verifies that when an executor
// returns OutcomeFailed (not a Go error) and no FailureArcs are configured,
// the token is routed to the work type's FAILED state place.
func TestExecutorFailure_OutcomeFailed_NoFailureArcs(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "executor_failure_no_arcs"))
	testutil.WriteSeedFile(t, dir, "task", []byte("work"))
	runner := testutil.NewProviderCommandRunner(workers.CommandResult{
		Stderr:   []byte("provider unavailable"),
		ExitCode: 1,
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing")
}

// TestExecutorFailure_WithFailureArcs verifies that when an executor fails and
// FailureArcs ARE configured, tokens are routed via those arcs (existing behavior).
func TestExecutorFailure_WithFailureArcs(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "executor_failure_with_arcs"))
	testutil.WriteSeedFile(t, dir, "task", []byte("work"))
	runner := testutil.NewProviderCommandRunner(workers.CommandResult{
		Stderr:   []byte("intentional failure"),
		ExitCode: 1,
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:done")
}

// TestExecutorSuccess_TokenAtOutputPlace verifies the success path:
// token is consumed, executor succeeds, token appears at the output place.
func TestExecutorSuccess_TokenAtOutputPlace(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte("work"))

	// MockProvider returns content containing the stop token "COMPLETE"
	// so the worker ACCEPTS via the real AgentExecutor pipeline.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Task finished. COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init")
}
