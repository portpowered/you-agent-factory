package executor

import (
	"context"
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestNewScriptExecutorWithDependenciesValidatesAndUsesSelectedCommandEdge(t *testing.T) {
	runner := &capturingCommandRunner{}
	definition := &interfaces.FactoryWorkerConfig{Command: "selected-script", Args: []string{"arg"}}
	docs := func(string) (map[string]string, error) { return map[string]string{}, nil }
	if _, err := NewScriptExecutorWithDependencies(nil, runner, nil, "", nil, nil, docs); err == nil || !strings.Contains(err.Error(), "definition is required") {
		t.Fatalf("missing definition error = %v", err)
	}
	if _, err := NewScriptExecutorWithDependencies(definition, nil, nil, "", nil, nil, docs); err == nil || !strings.Contains(err.Error(), "command runner is required") {
		t.Fatalf("missing command runner error = %v", err)
	}
	built, err := NewScriptExecutorWithDependencies(definition, runner, nil, "", nil, nil, docs)
	if err != nil || built.CommandRunner != runner {
		t.Fatalf("constructed executor = %+v, error = %v", built, err)
	}
}

func TestScriptExecutorExecuteRequiresResolvedRunner(t *testing.T) {
	executor := NewScriptExecutorWithRunner(
		&interfaces.FactoryWorkerConfig{Command: "selected-script"},
		&capturingCommandRunner{}, nil, "", nil, nil,
	)
	if _, err := executor.Execute(context.Background(), workerexecution.WorkstationExecutionRequest{}); err == nil || !strings.Contains(err.Error(), "runner registry is required") {
		t.Fatalf("missing runner registry error = %v", err)
	}
}

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
