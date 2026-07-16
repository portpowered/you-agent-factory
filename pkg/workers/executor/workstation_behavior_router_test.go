package executor

import (
	"context"
	"strings"
	"testing"

	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
)

func TestNewScriptExecutorFromInputValidatesAndUsesSelectedCommandEdge(t *testing.T) {
	runner := &capturingCommandRunner{}
	definition := &workerconfig.Config{Command: "selected-script", Args: []string{"arg"}}
	if _, err := NewScriptExecutorFromInput(ScriptConstructionInput{CommandRunner: runner}); err == nil || !strings.Contains(err.Error(), "definition is required") {
		t.Fatalf("missing definition error = %v", err)
	}
	if _, err := NewScriptExecutorFromInput(ScriptConstructionInput{Definition: definition}); err == nil || !strings.Contains(err.Error(), "command runner is required") {
		t.Fatalf("missing command runner error = %v", err)
	}
	built, err := NewScriptExecutorFromInput(ScriptConstructionInput{Definition: definition, CommandRunner: runner})
	if err != nil || built.CommandRunner != runner {
		t.Fatalf("constructed executor = %+v, error = %v", built, err)
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
			Workers: map[string]*workerconfig.Config{
				"agent-worker": {Type: workertaxonomy.WorkerTypeAgent},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"execute-story": {Type: workertaxonomy.WorkstationTypeAgent},
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
			Workers: map[string]*workerconfig.Config{
				"infer-worker": {Type: workertaxonomy.WorkerTypeInference},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"invoke-story": {Type: workertaxonomy.WorkstationTypeInference},
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
