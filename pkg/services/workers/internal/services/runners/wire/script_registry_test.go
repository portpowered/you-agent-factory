package wire

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/script"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
)

type scriptConformanceCommand struct {
	mu      sync.Mutex
	request workerprocess.CommandRequest
	calls   *atomic.Int32
}

func (command *scriptConformanceCommand) Run(
	ctx context.Context,
	request workerprocess.CommandRequest,
) (workerprocess.CommandResult, error) {
	return command.RunStreaming(ctx, request, nil)
}

func (command *scriptConformanceCommand) RunStreaming(
	_ context.Context,
	request workerprocess.CommandRequest,
	_ platformprocess.OutputChunkObserver,
) (workerprocess.CommandResult, error) {
	command.mu.Lock()
	command.request = workerprocess.CloneCommandRequest(request)
	command.mu.Unlock()
	if command.calls != nil {
		command.calls.Add(1)
	}
	if containsEnvironment(request.Env, "FAIL=true") {
		return workerprocess.CommandResult{Stderr: []byte("fixture failure")}, errors.New("fixture process failure")
	}
	return workerprocess.CommandResult{Stdout: []byte("fixture output")}, nil
}

func (command *scriptConformanceCommand) Request() workerprocess.CommandRequest {
	command.mu.Lock()
	defer command.mu.Unlock()
	return workerprocess.CloneCommandRequest(command.request)
}

func scriptDependencies(
	command workerprocess.CommandRunner,
	docs workers.FactoryDocsLoader,
) runners.ScriptDependencies {
	return runners.ScriptDependencies{
		CommandRunner: command,
		FactoryDocs:   docs,
		Now:           func() time.Time { return time.Unix(0, 0).UTC() },
		Publish:       func(workers.ProgressFragment) {},
		Record:        func(workers.ScriptEvent) {},
	}
}

func scriptRequest() workers.RunnerExecutionRequest {
	token := map[string]any{
		"color": map[string]any{
			"name":      "input",
			"work_id":   "work-1",
			"data_type": string(workers.DataTypeWork),
		},
		"nested": []any{"original"},
	}
	return workers.RunnerExecutionRequest{
		RunnerID:           script.Identity,
		InputTokens:        []any{token},
		EnvVars:            map[string]string{"FIXTURE": "original"},
		ProcessEnvironment: []string{"BASE=injected"},
		ModelBindings: []workers.ResolvedModelOperationBinding{{
			Slot: "prompt",
			Content: []work.WorkContentPart{{
				Type:     work.WorkContentPartTypeText,
				Text:     "original",
				Metadata: map[string]any{"nested": []any{"metadata-original"}},
			}},
		}},
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilityWorkingDirectory,
		},
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-conformance",
			InputTokens: []any{map[string]any{
				"color": map[string]any{
					"name":      "input",
					"work_id":   "work-1",
					"data_type": string(workers.DataTypeWork),
				},
				"nested": []any{"dispatch-original"},
			}},
		},
	}
}

func containsEnvironment(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertEffectCalls(
	t *testing.T,
	stage string,
	commandCalls *atomic.Int32,
	docsCalls *atomic.Int32,
	wantCommand int32,
	wantDocs int32,
) {
	t.Helper()
	if commandCalls.Load() != wantCommand || docsCalls.Load() != wantDocs {
		t.Fatalf(
			"%s effects = command %d docs %d, want command %d docs %d",
			stage,
			commandCalls.Load(),
			docsCalls.Load(),
			wantCommand,
			wantDocs,
		)
	}
}
