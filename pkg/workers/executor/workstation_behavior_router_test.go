package executor

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type routingStubExecutor struct {
	name string
}

func (executor *routingStubExecutor) Execute(_ context.Context, _ interfaces.WorkstationExecutionRequest) (interfaces.WorkResult, error) {
	return interfaces.WorkResult{
		Outcome: interfaces.OutcomeAccepted,
		Output:  executor.name,
	}, nil
}

func TestWorkstationBehaviorRouter_RoutesAgentRunToHarnessExecutor(t *testing.T) {
	t.Parallel()

	router := &WorkstationBehaviorRouter{
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"agent-worker": {Type: interfaces.WorkerTypeAgent},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"execute-story": {Type: interfaces.WorkstationTypeAgent},
			},
		},
		InferenceExecutor: &routingStubExecutor{name: "inference"},
		AgentRunExecutor:  &routingStubExecutor{name: "agent-run"},
	}

	result, err := router.Execute(context.Background(), interfaces.WorkstationExecutionRequest{
		Dispatch: interfaces.WorkDispatch{
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
			Workers: map[string]*interfaces.WorkerConfig{
				"infer-worker": {Type: interfaces.WorkerTypeInference},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"invoke-story": {Type: interfaces.WorkstationTypeInference},
			},
		},
		InferenceExecutor: &routingStubExecutor{name: "inference"},
		AgentRunExecutor:  &routingStubExecutor{name: "agent-run"},
	}

	result, err := router.Execute(context.Background(), interfaces.WorkstationExecutionRequest{
		Dispatch: interfaces.WorkDispatch{
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
