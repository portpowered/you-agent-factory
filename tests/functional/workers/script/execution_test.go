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
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerwire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
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

// TestStatelessInvocationAdapterExecutesThroughCanonicalWorkers proves the
// compatibility invocation boundary uses a real root-built Workers service,
// rather than constructing the retired provider-backed execution graph.
func TestStatelessInvocationAdapterExecutesThroughCanonicalWorkers(t *testing.T) {
	runner := support.NewRecordingCommandRunner("adapter-script-output")
	service, err := root.BuildStatelessWorkers(t.Context(), serviceedges.Edges{
		ScriptCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("BuildStatelessWorkers() error = %v", err)
	}
	executor := workerwire.NewInvocationWithProgressFromService(service, nil)

	result, err := executor.Execute(context.Background(), workerexecution.InvocationInput{
		Request: workerexecution.ProviderInferenceRequest{
			Dispatch: work.WorkDispatch{
				DispatchID:  "adapter-dispatch",
				WorkerType:  "SCRIPT_WORKER",
				ProjectID:   "adapter-project",
				InputTokens: []any{"adapter-token"},
				Execution: work.ExecutionMetadata{
					RequestID: "adapter-request",
					TraceID:   "adapter-trace",
				},
			},
			Correlation: workerexecution.ExecutionCorrelation{
				FactorySessionID: "adapter-session",
				RuntimeID:        "adapter-runtime",
				GenerationID:     "adapter-generation",
				DispatchID:       "adapter-dispatch",
				AttemptID:        "adapter-attempt",
				RequestID:        "adapter-request",
				TraceID:          "adapter-trace",
			},
			WorkerName:       "adapter-script-worker",
			WorkerType:       "SCRIPT_WORKER",
			WorkstationType:  "adapter-workstation",
			RunnerID:         "script",
			ExecutorProvider: workerexecution.ExecutorProviderACP,
			Model:            "adapter-model",
			ModelProvider:    "adapter-provider",
			ModelOperation:   "invoke",
			UserMessage:      "run the adapter script",
			EnvVars:          map[string]string{"ADAPTER_MODE": "functional"},
			Command:          "echo",
			Args:             []string{"adapter-output"},
		},
	})
	if err != nil {
		t.Fatalf("invocation Execute() error = %v", err)
	}
	if result.Attempt != 1 || result.Response.Outcome != workerexecution.OutcomeAccepted ||
		result.Response.Content != "adapter-script-output" {
		t.Fatalf(
			"invocation result = %#v, failure metadata = %#v, failure detail = %#v, want normalized accepted output",
			result,
			result.FailureMetadata,
			result.FailureDetail,
		)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("script command calls = %d, want one canonical Workers attempt", runner.CallCount())
	}
	request := runner.LastRequest()
	if request.Command != "echo" || len(request.Args) != 1 || request.Args[0] != "adapter-output" {
		t.Fatalf("script command request = %#v, want mapped invocation command", request)
	}
}

// TestStatelessInvocationAdapterPreservesCanonicalFailureAndPreStartErrors
// proves both canonical terminal results and pre-start Execute errors retain
// their public invocation shape at the compatibility boundary.
func TestStatelessInvocationAdapterPreservesCanonicalFailureAndPreStartErrors(t *testing.T) {
	t.Run("canonical failure result", func(t *testing.T) {
		runner := nonZeroExitCommandRunner{stderr: "adapter failure", exitCode: 17}
		service, err := root.BuildStatelessWorkers(t.Context(), serviceedges.Edges{
			ScriptCommandRunner: runner,
		})
		if err != nil {
			t.Fatalf("BuildStatelessWorkers() error = %v", err)
		}
		executor := workerwire.NewInvocationWithProgressFromService(service, nil)
		result, err := executor.Execute(context.Background(), workerexecution.InvocationInput{
			Attempt: 2,
			Request: workerexecution.ProviderInferenceRequest{
				RunnerID: "script",
				Command:  "echo",
				Args:     []string{"failure"},
				Dispatch: work.WorkDispatch{DispatchID: "adapter-failure"},
			},
		})
		if err != nil {
			t.Fatalf("invocation Execute() error = %v", err)
		}
		if result.Attempt != 2 || result.Response.Outcome != workerexecution.OutcomeFailed ||
			result.FailureMetadata == nil || result.FailureDetail == nil {
			t.Fatalf("failure result = %#v, want mapped canonical failure", result)
		}
	})

	t.Run("pre-start error", func(t *testing.T) {
		service, err := root.BuildStatelessWorkers(t.Context(), serviceedges.Edges{})
		if err != nil {
			t.Fatalf("BuildStatelessWorkers() error = %v", err)
		}
		executor := workerwire.NewInvocationWithProgressFromService(service, nil)
		result, err := executor.Execute(context.Background(), workerexecution.InvocationInput{
			Attempt: 3,
			Request: workerexecution.ProviderInferenceRequest{
				RunnerID: "unknown-functional-runner",
				Dispatch: work.WorkDispatch{DispatchID: "adapter-pre-start"},
			},
		})
		if err == nil || result.Attempt != 3 || result.FailureDetail == nil ||
			result.FailureMetadata == nil {
			t.Fatalf("pre-start result = %#v, error %v, want invocation failure", result, err)
		}
	})

	t.Run("nil context", func(t *testing.T) {
		service := &functionalInvocationService{result: workerexecution.ExecuteResult{
			Outcome: workerexecution.ExecutionOutcomeAccepted,
		}}
		executor := workerwire.NewInvocationWithProgressFromService(service, nil)
		result, err := executor.Execute(nil, workerexecution.InvocationInput{
			Attempt: 5,
			Request: workerexecution.ProviderInferenceRequest{RunnerID: "script"},
		})
		if err == nil || result.Attempt != 5 || result.FailureDetail == nil {
			t.Fatalf("nil-context result = %#v, error %v, want invocation failure", result, err)
		}
	})
}

// TestInvocationAdapterMapsAllPublicOutcomesFunctional keeps result mapping
// observable at the public bridge while the root-backed tests above exercise
// the workstation execution path.
func TestInvocationAdapterMapsAllPublicOutcomesFunctional(t *testing.T) {
	if executor := workerwire.NewInvocationWithProgressFromService(nil, nil); executor != nil {
		t.Fatal("nil Workers service produced an invocation executor")
	}

	cases := []struct {
		name    string
		outcome workerexecution.ExecutionOutcome
		want    workerexecution.WorkOutcome
	}{
		{name: "accepted", outcome: workerexecution.ExecutionOutcomeAccepted, want: workerexecution.OutcomeAccepted},
		{name: "continue", outcome: workerexecution.ExecutionOutcomeContinue, want: workerexecution.OutcomeContinue},
		{name: "rejected", outcome: workerexecution.ExecutionOutcomeRejected, want: workerexecution.OutcomeRejected},
		{name: "canceled", outcome: workerexecution.ExecutionOutcomeCanceled, want: workerexecution.OutcomeFailed},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			continuation := (&providers.SessionMetadata{
				Provider: "functional-provider",
				Kind:     "thread",
				ID:       "functional-continuation",
			}).ContinuationRef()
			service := &functionalInvocationService{result: workerexecution.ExecuteResult{
				Outcome:      test.outcome,
				Continuation: continuation,
				Output: workerexecution.ProposedOutput{
					Primary:        []work.WorkContentPart{{Text: "first"}, {Text: "second"}},
					Feedback:       "feedback",
					Classification: "classification",
				},
			}}
			executor := workerwire.NewInvocationWithProgressFromService(service, nil)
			result, err := executor.Execute(context.Background(), workerexecution.InvocationInput{
				Request: workerexecution.ProviderInferenceRequest{RunnerID: "script"},
				Attempt: 4,
			})
			if err != nil {
				t.Fatalf("invocation Execute() error = %v", err)
			}
			if result.Attempt != 4 || result.Response.Outcome != test.want ||
				result.Response.Content != "firstsecond" || result.Response.Feedback != "feedback" ||
				result.Response.Classification != "classification" || result.Continuation == nil ||
				result.Response.Continuation == nil {
				t.Fatalf("mapped result = %#v, want %q outcome and joined output", result, test.want)
			}
		})
	}

	t.Run("non-ACP provider and workflow context fallback", func(t *testing.T) {
		service := &functionalInvocationService{result: workerexecution.ExecuteResult{
			Outcome: workerexecution.ExecutionOutcomeAccepted,
		}}
		executor := workerwire.NewInvocationWithProgressFromService(service, nil)
		_, err := executor.Execute(context.Background(), workerexecution.InvocationInput{
			Request: workerexecution.ProviderInferenceRequest{
				RunnerID:         "script",
				ExecutorProvider: "custom-executor",
				WorkflowContext:  &workerexecution.Context{SessionID: "functional-session"},
				Dispatch:         work.WorkDispatch{DispatchID: "fallback-dispatch"},
			},
		})
		if err != nil {
			t.Fatalf("invocation Execute() error = %v", err)
		}
		if service.request.Target.Provider.ID != "custom-executor" ||
			service.request.Correlation.FactorySessionID != "functional-session" {
			t.Fatalf("canonical fallback request = %#v, want provider and workflow identity", service.request)
		}
	})
}

type functionalInvocationService struct {
	result  workerexecution.ExecuteResult
	request workerexecution.ExecuteRequest
}

func (service *functionalInvocationService) Execute(
	_ context.Context,
	request workerexecution.ExecuteRequest,
) (workerexecution.ExecuteResult, error) {
	service.request = request
	return service.result, nil
}

func (service *functionalInvocationService) InvokeModel(
	context.Context,
	string,
	modelinference.Request,
) (modelinference.Result, error) {
	return modelinference.Result{}, nil
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
