package dispatch

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPetriExecutorPanicRoutesToFailedTerminal proves a panicking
// WorkerExecutor still reaches the documented failed terminal with the
// established "executor panic: <cause>" compatibility text, split into its
// own focused top-level test (out of
// TestPetriWorkerErrorReturnsFailedTerminalOutcome in simple_run_test.go) so
// this scenario does not deepen that file's/function's existing backend-size
// debt.
func TestPetriExecutorPanicRoutesToFailedTerminal(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	traceID := "trace-executor-panic"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte(`{"title":"will panic at executor"}`),
	})

	// ProviderOverride (not ProviderCommandRunner) is required here; see the
	// documented in-scope exception on panicInferenceProvider below.
	provider := panicInferenceProvider{message: "simulated executor catastrophic panic"}
	provider.ProviderServiceAdapter.InferFunc = provider.Infer
	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)

	failedTerminal := support.WorkCustomerLocation("task", "failed")
	successTerminal := support.WorkCustomerLocation("task", "done")
	assertWorkAtCustomerStates(t, listed, map[string]int{
		failedTerminal:  1,
		successTerminal: 0,
		support.WorkCustomerLocation("task", "init"): 0,
	})
	assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{traceID})
	assertTraceAbsentAtCustomerState(t, listed, successTerminal, traceID)
	assertQuiescentSession(t, session, 0, 1)

	failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, traceID)
	if !ok {
		t.Fatalf("missing failed Work for trace %q at %s", traceID, failedTerminal)
	}
	dispatches := support.ObserveDispatchEvents(t, events)
	assertFailedDispatchForWork(t, dispatches, failedWorkID)
	assertFailedDispatchResponseErrorForWork(t, dispatches, failedWorkID)
	assertDispatchResponseErrorContains(
		t,
		dispatches,
		failedWorkID,
		"executor panic: simulated executor catastrophic panic",
	)
}

// panicInferenceProvider panics from Infer to exercise the Workers execution
// boundary's WorkerExecutor panic recovery through the customer process
// boundary rather than any internal Petri or Workers seam.
//
// backend-review construction-path exception: general-backend-standards.md §7
// rule 3 / code-review-standards.md §9 prefer ProviderCommandRunner and other
// command-runner edge mocks over custom in-process provider fakes. This cell
// uses ProviderOverride instead because there is no CommandResult/exit-code
// shape that can reach the WorkerExecutor recover() boundary this test
// proves: ScriptWrapProvider.Execute converts CommandRunner.Run's
// (CommandResult, error) return into an ordinary *ProviderError branch on
// err != nil or ExitCode != 0
// (pkg/services/workers/internal/providercompat/inference_provider.go:179-217),
// and everything downstream of that (invocation.Executor.Execute at
// pkg/services/workers/internal/services/workstations/invocation/executor.go:51)
// only calls deterministic, non-panicking string/format helpers on the
// result. A subprocess boundary cannot unwind the calling goroutine's stack,
// so ProviderCommandRunner can only ever produce the ordinary failed-executor
// path already covered by the "provider_command_exit_routes_to_failed_terminal"
// subtest in simple_run_test.go, never a Go-level panic. An in-process
// Provider.Infer fake is therefore the only way to exercise the recover() in
// workerExecutorRequestAdapter.Execute
// (pkg/services/workers/workstation_pool_boundary_impl.go).
type panicInferenceProvider struct {
	testutil.ProviderServiceAdapter
	message string
}

func (p panicInferenceProvider) Infer(
	_ context.Context,
	_ workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	panic(p.message)
}

func assertDispatchResponseErrorContains(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	workID string,
	want string,
) {
	t.Helper()

	for _, dispatch := range dispatches {
		if !support.DispatchObservationIncludesWork(dispatch, workID) {
			continue
		}
		if dispatch.Response == nil || dispatch.Response.Error == nil {
			continue
		}
		if strings.Contains(*dispatch.Response.Error, want) {
			return
		}
	}
	t.Fatalf("no dispatch response error for work %q containing %q", workID, want)
}
