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
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
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
	request := runner.LastRequest()
	if request.Command != "echo" {
		t.Fatalf("script command = %q, want authored command %q", request.Command, "echo")
	}
	assertCommandArgs(t, request, []string{"default-output"})
	if request.WorkDir == "" {
		t.Fatal("script command work directory is empty, want the runtime workspace")
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

// TestStatelessWorkersRootExecutesDetachedScriptAttempt proves a detached
// script attempt submitted to the public Workers root contract runs through
// the injected command edge and returns one correlated accepted result.
func TestStatelessWorkersRootExecutesDetachedScriptAttempt(t *testing.T) {
	runner := support.NewRecordingCommandRunner("detached-script-output")
	service, err := root.BuildStatelessWorkers(t.Context(), serviceedges.Edges{
		ScriptCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("BuildStatelessWorkers() error = %v", err)
	}

	result, err := service.Execute(t.Context(), detachedScriptExecuteRequest())
	if err != nil {
		t.Fatalf("Workers Execute() error = %v", err)
	}
	if result.Outcome != workerexecution.ExecutionOutcomeAccepted {
		t.Fatalf("detached outcome = %q, failure = %#v, want ACCEPTED", result.Outcome, result.Failure)
	}
	if got := executeOutputText(result.Output); got != "detached-script-output" {
		t.Fatalf("detached primary output = %q, want the script stdout", got)
	}
	if result.Correlation.DispatchID != "detached-dispatch" ||
		result.Correlation.AttemptID != "detached-attempt" ||
		result.Correlation.TraceID != "detached-trace" {
		t.Fatalf("result correlation = %#v, want the submitted detached correlation", result.Correlation)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("script command calls = %d, want one canonical Workers attempt", runner.CallCount())
	}
	request := runner.LastRequest()
	if request.Command != "echo" || len(request.Args) != 1 || request.Args[0] != "detached-output" {
		t.Fatalf("script command request = %#v, want the submitted command and args", request)
	}
}

// TestStatelessWorkersRootPreservesDetachedFailureAndPreStartErrors proves the
// public Workers root reports a typed terminal failure for a non-zero script
// exit and a pre-start error for an unknown runner, without sharing attempt
// state between the two calls.
func TestStatelessWorkersRootPreservesDetachedFailureAndPreStartErrors(t *testing.T) {
	t.Run("non-zero exit is a typed terminal failure", func(t *testing.T) {
		service, err := root.BuildStatelessWorkers(t.Context(), serviceedges.Edges{
			ScriptCommandRunner: nonZeroExitCommandRunner{stderr: "detached failure", exitCode: 17},
		})
		if err != nil {
			t.Fatalf("BuildStatelessWorkers() error = %v", err)
		}

		result, err := service.Execute(t.Context(), detachedScriptExecuteRequest())
		if err != nil {
			t.Fatalf("Workers Execute() error = %v", err)
		}
		if result.Outcome != workerexecution.ExecutionOutcomeFailed || result.Failure == nil {
			t.Fatalf("failed result = %#v, want a typed terminal failure", result)
		}
		if result.Failure.Type == "" || result.Failure.Family == "" {
			t.Fatalf("failure classification = %#v, want type and family", result.Failure)
		}
		if result.Correlation.DispatchID != "detached-dispatch" {
			t.Fatalf("failed correlation = %#v, want the submitted detached correlation", result.Correlation)
		}
	})

	t.Run("unknown runner fails before the command edge starts", func(t *testing.T) {
		runner := support.NewRecordingCommandRunner("never-executed")
		service, err := root.BuildStatelessWorkers(t.Context(), serviceedges.Edges{
			ScriptCommandRunner: runner,
		})
		if err != nil {
			t.Fatalf("BuildStatelessWorkers() error = %v", err)
		}

		request := detachedScriptExecuteRequest()
		request.Target.RunnerID = "unknown-functional-runner"
		result, err := service.Execute(t.Context(), request)
		if err == nil {
			t.Fatalf("pre-start result = %#v, want an unknown-runner request error", result)
		}
		if !strings.Contains(err.Error(), "unknown-functional-runner") {
			t.Fatalf("pre-start error = %v, want it to name the rejected runner", err)
		}
		if result.Outcome != "" {
			t.Fatalf("pre-start outcome = %q, want no terminal outcome before start", result.Outcome)
		}
		if runner.CallCount() != 0 {
			t.Fatalf("script command calls = %d, want no external effect before start", runner.CallCount())
		}
	})
}

func detachedScriptExecuteRequest() workerexecution.ExecuteRequest {
	return workerexecution.ExecuteRequest{
		Correlation: workerexecution.ExecutionCorrelation{
			FactorySessionID: "detached-session",
			RuntimeID:        "detached-runtime",
			GenerationID:     "detached-generation",
			DispatchID:       "detached-dispatch",
			AttemptID:        "detached-attempt",
			RequestID:        "detached-request",
			TraceID:          "detached-trace",
		},
		Target: workerexecution.ExecutionTarget{
			WorkerName:      "detached-script-worker",
			WorkerType:      "SCRIPT_WORKER",
			WorkstationName: "detached-workstation",
			RunnerID:        "script",
			Command:         "echo",
			Args:            []string{"detached-output"},
			Environment: workerexecution.EnvironmentPolicy{
				Vars: map[string]string{"DETACHED_MODE": "functional"},
			},
		},
		Input: workerexecution.ExecutionInput{
			Dispatch: work.WorkDispatch{
				DispatchID:  "detached-dispatch",
				WorkerType:  "SCRIPT_WORKER",
				ProjectID:   "detached-project",
				InputTokens: []any{"detached-token"},
				Execution: work.ExecutionMetadata{
					RequestID: "detached-request",
					TraceID:   "detached-trace",
				},
			},
		},
		Attempt: workerexecution.AttemptContext{Number: 1},
	}
}

func executeOutputText(output workerexecution.ProposedOutput) string {
	var builder strings.Builder
	for _, part := range output.Primary {
		builder.WriteString(part.Text)
	}
	return builder.String()
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
