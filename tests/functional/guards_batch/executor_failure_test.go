package guards_batch

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestExecutorFailure_NoFailureArcs(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_failure_no_arcs"))
	testutil.WriteSeedFile(t, dir, "task", []byte("work payload"))

	provider := testutil.NewMockProviderWithErrors(
		[]interfaces.InferenceResponse{{}},
		[]error{errors.New("executor crashed")},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing")

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

func TestExecutorFailure_OutcomeFailed_NoFailureArcs(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_failure_no_arcs"))
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

func TestExecutorFailure_WithFailureArcs(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_failure_with_arcs"))
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

func TestExecutorSuccess_TokenAtOutputPlace(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte("work"))

	provider := testutil.NewMockProvider(
		support.AcceptedProviderResponse(),
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
