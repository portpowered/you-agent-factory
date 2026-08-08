package executor

import (
	"context"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type promptReloadExecutor struct {
	calls     []workerexecution.WorkstationExecutionRequest
	onExecute func(workerexecution.WorkstationExecutionRequest)
}

func (e *promptReloadExecutor) Execute(
	_ context.Context,
	request workerexecution.WorkstationExecutionRequest,
) (workerexecution.WorkResult, error) {
	if e.onExecute != nil {
		e.onExecute(request)
	}
	e.calls = append(e.calls, workerexecution.CloneWorkstationExecutionRequest(request))
	return workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted}, nil
}

func TestWorkstationExecutorReloadsFileBackedPromptsPerDispatch(t *testing.T) {
	workerPath := "factory/workers/worker-a/AGENTS.md"
	workstationPath := "factory/workstations/review/AGENTS.md"
	files := &workstationFileSystemStub{files: map[string][]byte{
		workerPath:      []byte("---\ntype: MODEL\n---\nworker old"),
		workstationPath: []byte("---\ntype: MODEL\n---\nworkstation old"),
	}}
	runtimeConfig := staticRuntimeConfig{
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			"worker-a": {
				Type:             interfaces.WorkerTypeModel,
				Body:             "cached worker",
				PromptSourcePath: workerPath,
			},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"review": {
				Type:             interfaces.WorkstationTypeModel,
				PromptTemplate:   "cached workstation",
				PromptSourcePath: workstationPath,
			},
		},
	}
	capture := &promptReloadExecutor{}
	we := newTestWorkstationExecutor(runtimeConfig, capture)
	we.FileSystem = files

	editAfterSnapshot := true
	capture.onExecute = func(request workerexecution.WorkstationExecutionRequest) {
		if !editAfterSnapshot || request.Dispatch.DispatchID != "dispatch-1" {
			return
		}
		files.files[workerPath] = []byte("---\ntype: MODEL\n---\nworker new")
		files.files[workstationPath] = []byte("---\ntype: MODEL\n---\nworkstation new")
		editAfterSnapshot = false
		if request.SystemPrompt != "worker old" || request.UserMessage != "workstation old" {
			t.Errorf("first request changed after source edit: system=%q user=%q", request.SystemPrompt, request.UserMessage)
		}
	}

	for _, dispatchID := range []string{"dispatch-1", "dispatch-2"} {
		result, err := we.Execute(context.Background(), work.WorkDispatch{
			DispatchID:      dispatchID,
			TransitionID:    "transition-" + dispatchID,
			WorkerType:      "worker-a",
			WorkstationName: "review",
			InputTokens: InputTokens(factoryruntime.RuntimeToken{
				ID:    "token-" + dispatchID,
				Color: factoryruntime.RuntimeTokenColor{WorkID: "work-" + dispatchID},
			}),
		})
		if err != nil {
			t.Fatalf("Execute(%s) error = %v", dispatchID, err)
		}
		if result.Outcome != workerexecution.OutcomeAccepted {
			t.Fatalf("Execute(%s) outcome = %s, want accepted", dispatchID, result.Outcome)
		}
	}

	if len(capture.calls) != 2 {
		t.Fatalf("provider-bound calls = %d, want 2", len(capture.calls))
	}
	if got := capture.calls[0].SystemPrompt; got != "worker old" {
		t.Fatalf("first system prompt = %q, want worker old", got)
	}
	if got := capture.calls[0].UserMessage; got != "workstation old" {
		t.Fatalf("first user message = %q, want workstation old", got)
	}
	if got := capture.calls[1].SystemPrompt; got != "worker new" {
		t.Fatalf("second system prompt = %q, want worker new", got)
	}
	if got := capture.calls[1].UserMessage; got != "workstation new" {
		t.Fatalf("second user message = %q, want workstation new", got)
	}
	if runtimeConfig.Workers["worker-a"].Body != "cached worker" {
		t.Fatalf("runtime worker body mutated to %q", runtimeConfig.Workers["worker-a"].Body)
	}
	if runtimeConfig.Workstations["review"].PromptTemplate != "cached workstation" {
		t.Fatalf("runtime workstation prompt mutated to %q", runtimeConfig.Workstations["review"].PromptTemplate)
	}
}

func TestWorkstationExecutorKeepsInlinePromptsImmutable(t *testing.T) {
	files := &workstationFileSystemStub{files: map[string][]byte{}}
	capture := &promptReloadExecutor{}
	we := newTestWorkstationExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			"worker-a": {Type: interfaces.WorkerTypeModel, Body: "inline worker"},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"review": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "inline workstation"},
		},
	}, capture)
	we.FileSystem = files

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "dispatch-inline",
		TransitionID:    "transition-inline",
		WorkerType:      "worker-a",
		WorkstationName: "review",
	})
	if err != nil || result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Execute() = (%#v, %v), want accepted", result, err)
	}
	if len(files.reads) != 0 {
		t.Fatalf("inline prompt reads = %#v, want none", files.reads)
	}
	if len(capture.calls) != 1 || capture.calls[0].SystemPrompt != "inline worker" || capture.calls[0].UserMessage != "inline workstation" {
		t.Fatalf("inline request = %#v", capture.calls)
	}
}

func TestWorkstationExecutorFailsPromptSourceWithoutProviderCallAndRecovers(t *testing.T) {
	tests := []struct {
		name        string
		role        string
		path        string
		worker      *interfaces.FactoryWorkerConfig
		workstation *interfaces.FactoryWorkstationConfig
		repair      string
	}{
		{
			name: "worker",
			role: "worker",
			path: "factory/workers/worker-a/AGENTS.md",
			worker: &interfaces.FactoryWorkerConfig{
				Type:             interfaces.WorkerTypeModel,
				Body:             "stale worker",
				PromptSourcePath: "factory/workers/worker-a/AGENTS.md",
			},
			workstation: &interfaces.FactoryWorkstationConfig{
				Type:           interfaces.WorkstationTypeModel,
				PromptTemplate: "inline workstation",
			},
			repair: "---\ntype: MODEL\n---\nrepaired worker",
		},
		{
			name: "workstation",
			role: "workstation",
			path: "factory/workstations/review/prompt.md",
			worker: &interfaces.FactoryWorkerConfig{
				Type: interfaces.WorkerTypeModel,
				Body: "inline worker",
			},
			workstation: &interfaces.FactoryWorkstationConfig{
				Type:                   interfaces.WorkstationTypeModel,
				PromptTemplate:         "stale workstation",
				PromptSourcePath:       "factory/workstations/review/prompt.md",
				PromptSourceIsTemplate: true,
			},
			repair: "repaired workstation",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := &workstationFileSystemStub{files: map[string][]byte{}}
			capture := &promptReloadExecutor{}
			we := newTestWorkstationExecutor(staticRuntimeConfig{
				Workers:      map[string]*interfaces.FactoryWorkerConfig{"worker-a": test.worker},
				Workstations: map[string]*interfaces.FactoryWorkstationConfig{"review": test.workstation},
			}, capture)
			we.FileSystem = files

			result, err := we.Execute(context.Background(), work.WorkDispatch{
				DispatchID:      "dispatch-missing",
				TransitionID:    "transition-missing",
				WorkerType:      "worker-a",
				WorkstationName: "review",
			})
			if err != nil {
				t.Fatalf("missing source Execute() error = %v", err)
			}
			if result.Outcome != workerexecution.OutcomeFailed {
				t.Fatalf("missing source outcome = %s, want failed", result.Outcome)
			}
			if !strings.Contains(result.Error, test.role) || !strings.Contains(result.Error, test.path) {
				t.Fatalf("missing source error = %q, want role and path", result.Error)
			}
			if len(capture.calls) != 0 {
				t.Fatal("provider executor was called after prompt source failure")
			}

			files.files[test.path] = []byte(test.repair)
			result, err = we.Execute(context.Background(), work.WorkDispatch{
				DispatchID:      "dispatch-repaired",
				TransitionID:    "transition-repaired",
				WorkerType:      "worker-a",
				WorkstationName: "review",
			})
			if err != nil || result.Outcome != workerexecution.OutcomeAccepted {
				t.Fatalf("repaired Execute() = (%#v, %v), want accepted", result, err)
			}
			if len(capture.calls) != 1 {
				t.Fatalf("provider calls after repair = %d, want 1", len(capture.calls))
			}
		})
	}
}

var _ workerexecution.WorkstationRequestExecutor = (*promptReloadExecutor)(nil)
