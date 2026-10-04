package wire

import (
	"context"
	"errors"
	"strings"
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

type scriptNonStreamingCommand struct{}

func (scriptNonStreamingCommand) Run(context.Context, workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	panic("construction must not execute command effects")
}

func TestScriptImplementationRejectsInvalidConfigurationAndEffects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		message string
		mutate  func(*runners.ScriptConfig, *runners.ScriptDependencies)
	}{
		{"command", "script command is required", func(c *runners.ScriptConfig, d *runners.ScriptDependencies) {
			c.Command = "  "
			*d = runners.ScriptDependencies{}
		}},
		{"command runner", "script command runner is required", func(_ *runners.ScriptConfig, d *runners.ScriptDependencies) { d.CommandRunner = nil }},
		{"typed nil command runner", "script command runner is required", func(_ *runners.ScriptConfig, d *runners.ScriptDependencies) {
			d.CommandRunner = (*scriptConformanceCommand)(nil)
		}},
		{"streaming", "script command runner must support streaming", func(_ *runners.ScriptConfig, d *runners.ScriptDependencies) {
			d.CommandRunner = scriptNonStreamingCommand{}
		}},
		{"docs", "script Factory docs loader is required", func(_ *runners.ScriptConfig, d *runners.ScriptDependencies) { d.FactoryDocs = nil }},
		{"clock", "script clock is required", func(_ *runners.ScriptConfig, d *runners.ScriptDependencies) { d.Now = nil }},
		{"publisher", "script progress publisher is required", func(_ *runners.ScriptConfig, d *runners.ScriptDependencies) { d.Publish = nil }},
		{"recorder", "script event recorder is required", func(_ *runners.ScriptConfig, d *runners.ScriptDependencies) { d.Record = nil }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			agentDeps, config, deps, inferenceConfig, inferenceDeps := validProductionRegistryInputs()
			var calls atomic.Int32
			deps.CommandRunner = &scriptConformanceCommand{calls: &calls}
			deps.FactoryDocs = func(string) (map[string]string, error) { calls.Add(1); return nil, nil }
			deps.Now = func() time.Time { calls.Add(1); return time.Unix(0, 0) }
			deps.Publish = func(workers.ProgressFragment) { calls.Add(1) }
			deps.Record = func(workers.ScriptEvent) { calls.Add(1) }
			test.mutate(&config, &deps)
			runner, err := scriptImplementation(config, deps)
			if runner != nil {
				t.Fatal("invalid construction returned a runner")
			}
			assertScriptConstructionError(t, err, test.message)
			registry, err := NewProductionRegistry(agentDeps, config, deps, inferenceConfig, inferenceDeps)
			if registry != nil {
				t.Fatal("invalid construction returned a registry")
			}
			if !errors.Is(err, workers.ErrInvalidRunnerRegistration) {
				t.Fatalf("registry error = %v, want invalid registration", err)
			}
			if !strings.Contains(err.Error(), "script runner construction failed") {
				t.Fatalf("registry error = %v, want script identity", err)
			}
			assertScriptConstructionError(t, err, test.message)
			if calls.Load() != 0 {
				t.Fatalf("construction effect calls = %d, want zero", calls.Load())
			}
		})
	}
}

func assertScriptConstructionError(t *testing.T, err error, message string) {
	t.Helper()
	var providerError *workers.ProviderError
	if !errors.As(err, &providerError) {
		t.Fatalf("error = %v, want ProviderError", err)
	}
	if providerError.Type != workers.WorkFailureTypeMisconfigured {
		t.Fatalf("failure type = %v, want MISCONFIGURED", providerError.Type)
	}
	if providerError.Message != message {
		t.Fatalf("message = %q, want %q", providerError.Message, message)
	}
	if providerError.Cause != nil {
		t.Fatalf("cause = %v, want nil", providerError.Cause)
	}
}

func TestScriptImplementationDefersEffectsUntilRequestSelectedExecution(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	command := &scriptConformanceCommand{calls: &calls}
	deps := scriptDependencies(command, func(string) (map[string]string, error) { calls.Add(1); return nil, nil })
	deps.Now = func() time.Time { calls.Add(1); return time.Unix(0, 0) }
	deps.Publish = func(workers.ProgressFragment) { calls.Add(1) }
	deps.Record = func(workers.ScriptEvent) { calls.Add(1) }
	runner, err := scriptImplementation(runners.ScriptConfig{RequestSelected: true}, deps)
	if err != nil {
		t.Fatalf("construction error = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("construction effect calls = %d, want zero", calls.Load())
	}
	request := scriptRequest()
	request.Command = "selected-command"
	result, err := runner.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if result.Content != "fixture output" {
		t.Fatalf("content = %q, want fixture output", result.Content)
	}
	if command.Request().Command != "selected-command" {
		t.Fatalf("command = %q, want selected-command", command.Request().Command)
	}
	if calls.Load() == 0 {
		t.Fatal("Execute did not invoke effects")
	}
}
