package script_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const scriptPrimaryResultOutput = "script-primary-result-output"

// TestScriptWorkerCompletesWithPublicPrimaryResult proves a root-built script
// worker that exits successfully completes Work on the customer-visible surface
// and exposes the script stdout as the public dispatch primary result.
func TestScriptWorkerCompletesWithPublicPrimaryResult(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("success-input-payload"))

	runner := support.NewRecordingCommandRunner(scriptPrimaryResultOutput)
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ScriptCommandRunner: runner},
		10*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work tokens = %d, want 1 successful script dispatch", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work tokens = %d, want 0", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:init"); got != 0 {
		t.Fatalf("pending work tokens = %d, want 0 after successful completion", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("script command calls = %d, want exactly one external command effect", runner.CallCount())
	}

	assertDispatchOutput(t, events, scriptPrimaryResultOutput)
}

const scriptNonZeroExitMessage = "script-non-zero-exit-output"

// TestScriptWorkerNonZeroExitMapsToFailedOutcome proves a root-built script
// worker whose command exits non-zero routes Work to the failed customer state
// and reports a customer-readable dispatch failure instead of success.
func TestScriptWorkerNonZeroExitMapsToFailedOutcome(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("failure-input-payload"))

	runner := nonZeroExitCommandRunner{stderr: scriptNonZeroExitMessage, exitCode: 1}
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ScriptCommandRunner: runner},
		10*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work tokens = %d, want 1 non-zero-exit script dispatch", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed work tokens = %d, want 0 after script failure", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:init"); got != 0 {
		t.Fatalf("pending work tokens = %d, want 0 after script failure", got)
	}

	assertScriptNonZeroExitDispatchFailure(t, events, scriptNonZeroExitMessage)
}

// TestScriptWorkerCancellationTerminatesChildProcess proves cancelling a
// long-running root-built script worker terminates the external command edge
// and reports a non-success outcome on the public Work / Factory Event surface.
func TestScriptWorkerCancellationTerminatesChildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("cancellation-input-payload"))

	workstationAgentsPath := filepath.Join(dir, "workstations", "run-script", "AGENTS.md")
	agentsMD := "---\ntype: MODEL_WORKSTATION\nlimits:\n  maxExecutionTime: 10ms\n---\nExecute the script.\n"
	if err := os.WriteFile(workstationAgentsPath, []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		cfg["workstations"] = append(cfg["workstations"].([]any), map[string]any{
			"name":     "cancellation-loop-breaker",
			"behavior": "STANDARD",
			"type":     "LOGICAL_MOVE",
			"inputs":   []map[string]any{{"workType": "task", "state": "init"}},
			"outputs":  []map[string]any{{"workType": "task", "state": "failed"}},
			"guards": []map[string]any{{
				"type":        "VISIT_COUNT",
				"workstation": "run-script",
				"maxVisits":   float64(1),
			}},
		})
	})

	runner := &blockingCancellationCommandRunner{}
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ScriptCommandRunner: runner},
		10*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work tokens = %d, want 1 cancelled script dispatch", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed work tokens = %d, want 0 after cancellation", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:init"); got != 0 {
		t.Fatalf("pending work tokens = %d, want 0 after cancellation", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("script command calls = %d, want exactly one external command effect", runner.CallCount())
	}
	if !runner.Started() {
		t.Fatal("script command edge never entered a cancellable long-running execution")
	}
	if !runner.ContextCanceled() {
		t.Fatal("script command edge context was not canceled before termination")
	}
	if !runner.Terminated() {
		t.Fatal("script command edge did not terminate after cancellation")
	}
	assertScriptCancellationDispatchFailure(t, events)
}

// TestInferenceEvents_ScriptWorkersDoNotEmitInferenceEvents proves a root-built
// script worker completes through dispatch lifecycle Factory Events without
// emitting inference request or response events.
func TestInferenceEvents_ScriptWorkersDoNotEmitInferenceEvents(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "work-script-no-inference",
		WorkTypeID: "task",
		TraceID:    "trace-script-no-inference",
		Payload:    []byte("script input"),
	})

	runner := support.NewStaticSuccessCommandRunner("script-output-ok")
	_, _, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ScriptCommandRunner: runner},
		10*time.Second,
	)

	if !hasFactoryEventType(events, factoryapi.FactoryEventTypeDispatchRequest) ||
		!hasFactoryEventType(events, factoryapi.FactoryEventTypeDispatchResponse) {
		t.Fatalf(
			"script worker canonical events = %v, want dispatch lifecycle events",
			factoryEventTypes(events),
		)
	}
	if hasFactoryEventType(events, factoryapi.FactoryEventTypeInferenceRequest) ||
		hasFactoryEventType(events, factoryapi.FactoryEventTypeInferenceResponse) {
		t.Fatalf("script worker emitted inference events: %v", factoryEventTypes(events))
	}
}

// TestServiceConfigOverrideAlignment_FunctionalHTTPServerScriptCommandRunner
// proves a root-built script worker routes through the replaced
// ScriptCommandRunner edge and completes one terminal dispatch.
func TestServiceConfigOverrideAlignment_FunctionalHTTPServerScriptCommandRunner(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("script server alignment"))

	runner := support.NewRecordingCommandRunner("script alignment output")
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ScriptCommandRunner: runner},
		10*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("terminal token count = %d, want 1", got)
	}
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("script command runner calls = %d, want 1", got)
	}
}

type blockingCancellationCommandRunner struct {
	mu              sync.Mutex
	calls           int
	started         bool
	contextCanceled bool
	terminated      bool
}

func (r *blockingCancellationCommandRunner) Run(
	ctx context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	r.started = true
	r.mu.Unlock()

	<-ctx.Done()

	r.mu.Lock()
	r.contextCanceled = true
	r.terminated = true
	r.mu.Unlock()

	return platformprocess.CommandResult{}, ctx.Err()
}

func (r *blockingCancellationCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *blockingCancellationCommandRunner) Started() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.started
}

func (r *blockingCancellationCommandRunner) ContextCanceled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.contextCanceled
}

func (r *blockingCancellationCommandRunner) Terminated() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.terminated
}

func assertScriptCancellationDispatchFailure(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) == 0 {
		t.Fatal("factory events missing dispatch observations")
	}

	for _, want := range []string{
		"execution cancelled: context canceled",
		"execution timeout",
	} {
		for _, dispatch := range dispatches {
			response := dispatch.Response
			if response == nil || response.Outcome != factoryapi.WorkOutcomeFailed {
				continue
			}
			if response.Output != nil {
				continue
			}
			if response.Error != nil && strings.Contains(*response.Error, want) {
				return
			}
		}
	}

	t.Fatalf(
		"factory events missing failed cancellation dispatch: %#v",
		dispatches,
	)
}

type nonZeroExitCommandRunner struct {
	stderr   string
	exitCode int
}

func (r nonZeroExitCommandRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{
		Stderr:   []byte(r.stderr),
		ExitCode: r.exitCode,
	}, nil
}

func assertScriptNonZeroExitDispatchFailure(t *testing.T, events []factoryapi.FactoryEvent, wantMessage string) {
	t.Helper()

	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) == 0 {
		t.Fatal("factory events missing dispatch observations")
	}
	response := dispatches[len(dispatches)-1].Response
	if response == nil {
		t.Fatal("dispatch response missing for failed script execution")
	}
	if response.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("dispatch outcome = %s, want FAILED", response.Outcome)
	}
	if response.Output != nil {
		t.Fatalf("dispatch output = %#v, want no primary result on script failure", response.Output)
	}
	assertDispatchErrorContains(t, events, wantMessage)
}
