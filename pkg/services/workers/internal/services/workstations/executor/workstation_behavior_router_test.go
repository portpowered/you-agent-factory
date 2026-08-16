package executor

import (
	"context"
	"errors"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type routingStubExecutor struct {
	name string
}

func (executor *routingStubExecutor) Execute(_ context.Context, _ workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
	return workerexecution.WorkResult{
		Outcome: workerexecution.OutcomeAccepted,
		Output:  executor.name,
	}, nil
}

func TestWorkstationBehaviorRouter_RoutesAgentRunToHarnessExecutor(t *testing.T) {
	t.Parallel()

	router := &WorkstationBehaviorRouter{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"agent-worker": {Type: interfaces.WorkerTypeAgent},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"execute-story": {Type: interfaces.WorkstationTypeAgent},
			},
		},
		InferenceExecutor: &routingStubExecutor{name: "inference"},
		AgentRunExecutor:  &routingStubExecutor{name: "agent-run"},
	}

	result, err := router.Execute(context.Background(), workerexecution.WorkstationExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-1",
			TransitionID:    "transition-1",
			WorkerType:      "agent-worker",
			WorkstationName: "execute-story",
		},
		WorkerType: "agent-worker",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Output != "agent-run" {
		t.Fatalf("Output = %q, want agent-run routing", result.Output)
	}
}

func TestWorkstationBehaviorRouter_RoutesInferenceRunToInferenceExecutor(t *testing.T) {
	t.Parallel()

	router := &WorkstationBehaviorRouter{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"infer-worker": {Type: interfaces.WorkerTypeInference},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"invoke-story": {Type: interfaces.WorkstationTypeInference},
			},
		},
		InferenceExecutor: &routingStubExecutor{name: "inference"},
		AgentRunExecutor:  &routingStubExecutor{name: "agent-run"},
	}

	result, err := router.Execute(context.Background(), workerexecution.WorkstationExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-2",
			TransitionID:    "transition-2",
			WorkerType:      "infer-worker",
			WorkstationName: "invoke-story",
		},
		WorkerType: "infer-worker",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Output != "inference" {
		t.Fatalf("Output = %q, want inference routing", result.Output)
	}
}

func TestWorkstationBehaviorRouter_ReturnsFailureWhenInferenceExecutorUnavailable(t *testing.T) {
	t.Parallel()

	request := workerexecution.WorkstationExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID:   "dispatch-unavailable",
			TransitionID: "transition-unavailable",
		},
	}
	tests := []struct {
		name   string
		router *WorkstationBehaviorRouter
	}{
		{name: "nil router"},
		{name: "missing inference executor", router: &WorkstationBehaviorRouter{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := tc.router.Execute(context.Background(), request)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if result.DispatchID != request.Dispatch.DispatchID ||
				result.TransitionID != request.Dispatch.TransitionID ||
				result.Outcome != workerexecution.OutcomeFailed ||
				result.Error != "inference executor unavailable" {
				t.Fatalf("Execute() result = %#v, want unavailable-inference failure with dispatch lineage", result)
			}
		})
	}
}

func TestWorkstationBehaviorRouter_InvalidAgentRunClassificationRoutesInference(t *testing.T) {
	t.Parallel()

	agentConfig := staticRuntimeConfig{
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			"agent-worker": {Type: interfaces.WorkerTypeAgent},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"execute-story": {Type: interfaces.WorkstationTypeAgent},
		},
	}
	tests := []struct {
		name          string
		runtimeConfig interfaces.RuntimeDefinitionLookup
		agentExecutor WorkstationRequestExecutor
		workstation   string
		worker        string
	}{
		{
			name:          "agent executor unavailable",
			runtimeConfig: agentConfig,
			workstation:   "execute-story",
			worker:        "agent-worker",
		},
		{
			name:          "workstation unavailable",
			runtimeConfig: agentConfig,
			agentExecutor: &routingStubExecutor{name: "agent-run"},
			workstation:   "missing",
			worker:        "agent-worker",
		},
		{
			name: "worker unavailable",
			runtimeConfig: staticRuntimeConfig{
				Workstations: agentConfig.Workstations,
			},
			agentExecutor: &routingStubExecutor{name: "agent-run"},
			workstation:   "execute-story",
			worker:        "missing",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			router := &WorkstationBehaviorRouter{
				RuntimeConfig:     tc.runtimeConfig,
				InferenceExecutor: &routingStubExecutor{name: "inference"},
				AgentRunExecutor:  tc.agentExecutor,
			}
			result, err := router.Execute(context.Background(), workerexecution.WorkstationExecutionRequest{
				Dispatch: work.WorkDispatch{
					WorkstationName: tc.workstation,
					WorkerType:      tc.worker,
				},
				WorkerType: tc.worker,
			})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if result.Output != "inference" {
				t.Fatalf("Output = %q, want inference routing", result.Output)
			}
		})
	}
}

func TestWorkstationBehaviorRouter_UsesDispatchWorkerForAgentRunRouting(t *testing.T) {
	t.Parallel()

	router := &WorkstationBehaviorRouter{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"agent-worker": {Type: interfaces.WorkerTypeAgent},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"execute-story": {Type: interfaces.WorkstationTypeAgent},
			},
		},
		InferenceExecutor: &routingStubExecutor{name: "inference"},
		AgentRunExecutor:  &routingStubExecutor{name: "agent-run"},
	}
	result, err := router.Execute(context.Background(), workerexecution.WorkstationExecutionRequest{
		Dispatch: work.WorkDispatch{
			WorkstationName: "execute-story",
			WorkerType:      "agent-worker",
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Output != "agent-run" {
		t.Fatalf("Output = %q, want agent-run routing", result.Output)
	}
}

// TestNewProviderInvocationExecutor_AbsentInvocationYieldsNoExecutor lets
// composition treat "no provider invocation available" as an absent route
// rather than a route that fails at dispatch time.
func TestNewProviderInvocationExecutor_AbsentInvocationYieldsNoExecutor(t *testing.T) {
	if executor := NewProviderInvocationExecutor(nil); executor != nil {
		t.Fatalf("NewProviderInvocationExecutor(nil) = %#v, want nil", executor)
	}
}

// TestProviderInvocationExecutor_ResolvesEverySelectionFromTheRequest pins the
// whole premise of this route: no workstation definition is consulted, so
// every selection the provider needs must arrive on the execution request and
// reach the inference request unchanged.
func TestProviderInvocationExecutor_ResolvesEverySelectionFromTheRequest(t *testing.T) {
	invocation := &recordingInvocation{result: workerexecution.InvocationResult{
		Response: workerexecution.InferenceResponse{Content: `{"text":"done"}`},
	}}
	executor := NewProviderInvocationExecutor(invocation)

	result, err := executor.Execute(context.Background(), workerexecution.WorkstationExecutionRequest{
		Dispatch:         work.WorkDispatch{DispatchID: "dispatch-1", TransitionID: "t-1"},
		WorkerType:       "worker-a",
		ExecutorProvider: "codex",
		ModelProvider:    "codex",
		Model:            "codex-test-model",
		ReasoningEffort:  "high",
		SystemPrompt:     "be brief",
		UserMessage:      "summarize",
		OutputSchema:     `{"type":"object"}`,
		WorkingDirectory: "/project",
		SkipPermissions:  true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	if result.Output != `{"text":"done"}` {
		t.Fatalf("output = %q, want the provider's content", result.Output)
	}

	request := invocation.input.Request
	if request.WorkerType != "worker-a" {
		t.Fatalf("worker type = %q, want the caller's own", request.WorkerType)
	}
	if request.Model != "codex-test-model" || request.ModelProvider != "codex" {
		t.Fatalf("model selection = %q/%q, want the caller's own", request.ModelProvider, request.Model)
	}
	if request.ReasoningEffort != "high" || request.SystemPrompt != "be brief" || request.UserMessage != "summarize" {
		t.Fatalf("prompt selections did not survive: %#v", request)
	}
	if !request.SkipPermissions {
		t.Fatal("skip-permissions = false; the caller already resolved the invocation-effective policy")
	}
	if request.RunnerID != "codex" {
		t.Fatalf("runner = %q, want the runner derived from the executor provider", request.RunnerID)
	}
	if invocation.input.Attempt != 1 {
		t.Fatalf("attempt = %d, want 1; attempt budgeting belongs to Worker Sessions", invocation.input.Attempt)
	}
}

// TestProviderInvocationExecutor_PreservesTheWorkersFailureClassification is
// the property Worker Sessions consults to decide whether an attempt is worth
// retrying. Dropping FailureMetadata here would silently make every provider
// failure terminal.
func TestProviderInvocationExecutor_PreservesTheWorkersFailureClassification(t *testing.T) {
	metadata := &workerexecution.WorkFailureMetadata{
		Family: workerexecution.WorkFailureFamilyRetryable,
		Type:   workerexecution.WorkFailureTypeInternalServerError,
	}
	invocation := &recordingInvocation{
		err: errors.New("provider execution failed"),
		result: workerexecution.InvocationResult{
			FailureMetadata: metadata,
			FailureDetail: &workerexecution.FailureDetail{
				Reason:  workerexecution.WorkFailureTypeInternalServerError,
				Message: "provider returned 500",
			},
		},
	}
	executor := NewProviderInvocationExecutor(invocation)

	result, err := executor.Execute(context.Background(), workerexecution.WorkstationExecutionRequest{
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-1", TransitionID: "t-1"},
	})
	if err == nil {
		t.Fatal("Execute error = nil, want the provider failure surfaced")
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("outcome = %q, want FAILED", result.Outcome)
	}
	if result.FailureMetadata != metadata {
		t.Fatalf("failure metadata = %#v, want the Workers-owned classification preserved", result.FailureMetadata)
	}
	if result.DispatchID != "dispatch-1" || result.TransitionID != "t-1" {
		t.Fatalf("failure identity = %q/%q, want the dispatch's own", result.DispatchID, result.TransitionID)
	}
	if result.Error == "" {
		t.Fatal("failure error = empty, want a non-empty description")
	}
}

// TestProviderInvocationExecutor_UnavailableInvocationFailsTheDispatch keeps a
// misconfigured route a failed Worker rather than a panic that escapes the
// Workers boundary.
func TestProviderInvocationExecutor_UnavailableInvocationFailsTheDispatch(t *testing.T) {
	var executor *ProviderInvocationExecutor
	result, err := executor.Execute(context.Background(), workerexecution.WorkstationExecutionRequest{
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-1"},
	})
	if err != nil {
		t.Fatalf("Execute error = %v, want the failure carried on the result", err)
	}
	if result.Outcome != workerexecution.OutcomeFailed || result.DispatchID != "dispatch-1" {
		t.Fatalf("result = %#v, want a failed result for dispatch-1", result)
	}
}

// TestProviderInvocationExecutor_ExplicitRunnerWins proves the derived runner
// is a fallback, not an override: a child that named its runner keeps it, even
// when its executor provider would have derived a different one.
func TestProviderInvocationExecutor_ExplicitRunnerWins(t *testing.T) {
	invocation := &recordingInvocation{}
	executor := NewProviderInvocationExecutor(invocation)

	if _, err := executor.Execute(context.Background(), workerexecution.WorkstationExecutionRequest{
		Dispatch:         work.WorkDispatch{DispatchID: "dispatch-1"},
		RunnerID:         "claude",
		ExecutorProvider: "codex",
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := invocation.input.Request.RunnerID; got != "claude" {
		t.Fatalf("runner = %q, want the caller's explicit selection", got)
	}
}

type recordingInvocation struct {
	input  workerexecution.InvocationInput
	result workerexecution.InvocationResult
	err    error
}

var _ workerexecution.InvocationExecutor = (*recordingInvocation)(nil)

func (i *recordingInvocation) Execute(
	_ context.Context,
	input workerexecution.InvocationInput,
) (workerexecution.InvocationResult, error) {
	i.input = input
	return i.result, i.err
}
