package wire

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/script"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/testkit"
)

func TestNewScriptRegistryIsInertAndResolvesDetachedMetadata(t *testing.T) {
	var commandCalls atomic.Int32
	var docsCalls atomic.Int32
	command := &scriptConformanceCommand{calls: &commandCalls}
	dependencies := scriptDependencies(command, func(string) (map[string]string, error) {
		docsCalls.Add(1)
		return map[string]string{"arg": "original"}, nil
	})
	config := script.Config{
		Command:          "fixture",
		Args:             []string{`{{ index .Docs "arg" }}`},
		FactoryDirectory: "factory-root",
	}

	registry, err := NewScriptRegistry(config, dependencies)
	if err != nil {
		t.Fatalf("NewScriptRegistry() error = %v", err)
	}
	config.Args[0] = "mutated"
	dependencies.FactoryDocs = func(string) (map[string]string, error) {
		t.Fatal("mutated dependency was retained")
		return nil, nil
	}
	assertEffectCalls(t, "construction", &commandCalls, &docsCalls, 0, 0)

	first, err := registry.Resolve(runners.ResolutionRequest{
		Identity: script.Identity,
		RequiredCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilityWorkingDirectory,
			workers.RunnerOptionalCapabilityWorktree,
		},
	})
	if err != nil {
		t.Fatalf("Resolve(script) error = %v", err)
	}
	if first.Identity != script.Identity ||
		first.Metadata.ID != script.Identity ||
		first.Metadata.DisplayName != "Script" ||
		first.Runner == nil {
		t.Fatalf("Resolve(script) = %#v, want complete Script binding", first)
	}
	assertEffectCalls(t, "resolution", &commandCalls, &docsCalls, 0, 0)

	first.Metadata.Capabilities.Baseline[0] = "mutated"
	first.Metadata.Capabilities.Optional[0].Detail = "mutated"
	second, err := registry.Resolve(runners.ResolutionRequest{Identity: script.Identity})
	if err != nil {
		t.Fatalf("second Resolve(script) error = %v", err)
	}
	if reflect.DeepEqual(first.Metadata, second.Metadata) {
		t.Fatalf("second metadata = %#v, want detached registry snapshot", second.Metadata)
	}

	_, err = second.Runner.Execute(t.Context(), scriptRequest())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := command.Request().Args; !reflect.DeepEqual(got, []string{"original"}) {
		t.Fatalf("command args = %#v, want construction snapshot", got)
	}
	if docsCalls.Load() != 1 {
		t.Fatalf("Factory docs calls = %d, want original injected effect once", docsCalls.Load())
	}
}

func TestScriptRunnerThroughRegistryConformsToCommonContract(t *testing.T) {
	command := &scriptConformanceCommand{}
	registry, err := NewScriptRegistry(
		script.Config{Command: "fixture", Args: []string{"{{ .Context.Env.FIXTURE }}"}},
		scriptDependencies(command, func(string) (map[string]string, error) {
			return map[string]string{}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewScriptRegistry() error = %v", err)
	}
	binding, err := registry.Resolve(runners.ResolutionRequest{Identity: script.Identity})
	if err != nil {
		t.Fatalf("Resolve(script) error = %v", err)
	}

	valid := scriptRequest()
	baseline, err := binding.Runner.Execute(t.Context(), valid)
	if err != nil {
		t.Fatalf("baseline Execute() error = %v", err)
	}
	invalid := workers.CloneProviderInferenceRequest(valid)
	invalid.RunnerID = ""
	unsupported := workers.CloneProviderInferenceRequest(valid)
	unsupported.RequiredOptionalCapabilities = []workers.RunnerOptionalCapability{
		workers.RunnerOptionalCapabilityImageInput,
	}
	failure := workers.CloneProviderInferenceRequest(valid)
	failure.EnvVars["FAIL"] = "true"

	testkit.Run(t, testkit.Subject{
		Runner:             binding.Runner,
		ValidRequest:       valid,
		InvalidRequest:     invalid,
		UnsupportedRequest: unsupported,
		FailureRequest:     failure,
		ExpectedResult:     baseline,
		AssertCaptured: func(t *testing.T) {
			t.Helper()
			request := command.Request()
			if !reflect.DeepEqual(request.Args, []string{"original"}) {
				t.Fatalf("captured args = %#v, want caller-owned snapshot", request.Args)
			}
			if !containsEnvironment(request.Env, "FIXTURE=original") {
				t.Fatalf("captured environment = %#v, want original value", request.Env)
			}
		},
	})
}

func TestScriptRegistryResolveAndExecuteConcurrently(t *testing.T) {
	command := &scriptConformanceCommand{}
	registry, err := NewScriptRegistry(
		script.Config{Command: "fixture"},
		scriptDependencies(command, func(string) (map[string]string, error) {
			return map[string]string{}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewScriptRegistry() error = %v", err)
	}

	const executions = 24
	var group sync.WaitGroup
	errs := make(chan error, executions)
	for range executions {
		group.Add(1)
		go func() {
			defer group.Done()
			binding, resolveErr := registry.Resolve(runners.ResolutionRequest{
				Identity: script.Identity,
			})
			if resolveErr != nil {
				errs <- resolveErr
				return
			}
			result, executeErr := binding.Runner.Execute(t.Context(), scriptRequest())
			if executeErr != nil {
				errs <- executeErr
				return
			}
			if result.Content != "fixture output" {
				errs <- errors.New("unexpected detached result content")
			}
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent resolve/execute: %v", err)
	}
}

type scriptConformanceCommand struct {
	mu      sync.Mutex
	request workers.CommandRequest
	calls   *atomic.Int32
}

func (command *scriptConformanceCommand) Run(
	ctx context.Context,
	request workers.CommandRequest,
) (workers.CommandResult, error) {
	return command.RunStreaming(ctx, request, nil)
}

func (command *scriptConformanceCommand) RunStreaming(
	_ context.Context,
	request workers.CommandRequest,
	_ platformprocess.OutputChunkObserver,
) (workers.CommandResult, error) {
	command.mu.Lock()
	command.request = workers.CloneSubprocessExecutionRequest(request)
	command.mu.Unlock()
	if command.calls != nil {
		command.calls.Add(1)
	}
	if containsEnvironment(request.Env, "FAIL=true") {
		return workers.CommandResult{Stderr: []byte("fixture failure")}, errors.New("fixture process failure")
	}
	return workers.CommandResult{Stdout: []byte("fixture output")}, nil
}

func (command *scriptConformanceCommand) Request() workers.CommandRequest {
	command.mu.Lock()
	defer command.mu.Unlock()
	return workers.CloneSubprocessExecutionRequest(command.request)
}

func scriptDependencies(
	command workers.CommandRunner,
	docs workers.FactoryDocsLoader,
) script.Dependencies {
	return script.Dependencies{
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
