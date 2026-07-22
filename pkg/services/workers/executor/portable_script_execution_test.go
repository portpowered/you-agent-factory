package executor

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestPortableCopiedScriptExecution_PreservesCommandArgsWorkingDirectoryAndEnvironment(t *testing.T) {
	factoryDir := t.TempDir()
	worker := &interfaces.FactoryWorkerConfig{
		Name:    "executor",
		Type:    interfaces.WorkerTypeScript,
		Command: "powershell",
		Args:    []string{"-File", "scripts/execute-story.ps1"},
		Timeout: "45m",
	}
	runner := &portableRecordingScriptRunner{stdout: "portable copied script accepted"}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			FactoryPath: factoryDir,
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"executor": worker,
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"execute-story": {
					Type:             interfaces.WorkstationTypeModel,
					WorkingDirectory: "repo/{{ (index .Inputs 0).WorkID }}",
					Env:              map[string]string{"SCRIPT_MODE": "portable"},
				},
			},
		},
		NewScriptExecutorWithRunner(worker, runner, nil, "", nil, time.Now),
	)
	we.Renderer = portableStaticPromptRenderer("")

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "dispatch-1",
		TransitionID:    "transition-1",
		WorkerType:      "executor",
		WorkstationName: "execute-story",
		InputTokens: InputTokens(factoryruntime.RuntimeToken{
			ID:      "token-1",
			PlaceID: "task:init",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID:     "work-001",
				WorkTypeID: "task",
				DataType:   factoryruntime.RuntimeTokenDataTypeWork,
				TraceID:    "trace-001",
				Payload:    []byte("portable task"),
			},
		}),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted ||
		result.Output != "portable copied script accepted" {
		t.Fatalf("result = %#v, want accepted portable output", result)
	}

	request := runner.request
	if request.Command != "powershell" {
		t.Fatalf("command = %q, want powershell", request.Command)
	}
	if len(request.Args) != 2 ||
		request.Args[0] != "-File" ||
		request.Args[1] != "scripts/execute-story.ps1" {
		t.Fatalf("args = %#v, want portable script path", request.Args)
	}
	if request.WorkDir != filepath.Join(factoryDir, "repo", "work-001") {
		t.Fatalf("work dir = %q, want Factory-relative work directory", request.WorkDir)
	}
	if !portableEnvironmentContains(request.Env, "SCRIPT_MODE=portable") {
		t.Fatalf("environment = %v, want SCRIPT_MODE=portable", request.Env)
	}
}

type portableStaticPromptRenderer string

func (renderer portableStaticPromptRenderer) Render(
	string,
	[]workerexecution.Token,
	*workerexecution.Context,
) (string, error) {
	return string(renderer), nil
}

type portableRecordingScriptRunner struct {
	request CommandRequest
	stdout  string
}

func (runner *portableRecordingScriptRunner) Run(
	_ context.Context,
	request CommandRequest,
) (CommandResult, error) {
	request.Args = append([]string(nil), request.Args...)
	request.Env = append([]string(nil), request.Env...)
	runner.request = request
	return CommandResult{Stdout: []byte(runner.stdout)}, nil
}

func portableEnvironmentContains(environment []string, want string) bool {
	for _, value := range environment {
		if value == want {
			return true
		}
	}
	return false
}
