package wire

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/testkit"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
)

func TestNewMockProductionRegistryIsInertAndResolvesWithoutExecution(t *testing.T) {
	var commandCalls atomic.Int32
	registry := newExplicitMockRegistry(t, &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName: "writer",
			RunType:    workers.MockWorkerRunTypeAccept,
		}},
	}, &mockCommandSpy{calls: &commandCalls})

	if commandCalls.Load() != 0 {
		t.Fatalf("construction command calls = %d, want 0", commandCalls.Load())
	}

	binding, err := registry.Resolve(runners.ResolutionRequest{Identity: runners.MockIdentity})
	if err != nil {
		t.Fatalf("Resolve(mock) error = %v", err)
	}
	if binding.Identity != runners.MockIdentity || binding.Runner == nil {
		t.Fatalf("Resolve(mock) = %#v, want complete mock binding", binding)
	}
	if commandCalls.Load() != 0 {
		t.Fatalf("resolution command calls = %d, want 0", commandCalls.Load())
	}
}

func TestMockRunnerThroughProductionRegistryConformsToCommonContract(t *testing.T) {
	inputTokens, outputTokens := int64(10), int64(5)
	usage := &workers.MockWorkerUsageConfig{
		Provider: "codex", Model: "gpt-5-codex",
		InputTokens: &inputTokens, OutputTokens: &outputTokens,
	}
	registry := newExplicitMockRegistry(t, &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName: "writer",
			RunType:    workers.MockWorkerRunTypeAccept,
		}, {
			WorkerName: "failing",
			RunType:    workers.MockWorkerRunTypeReject,
			Usage:      usage,
		}},
	}, nil)

	valid := mockRequest("writer")
	baseline, err := registry.Execute(t.Context(), runners.ExecuteRequest{
		Identity: runners.MockIdentity,
		Attempt:  valid,
	})
	if err != nil {
		t.Fatalf("baseline Execute() error = %v", err)
	}
	invalid := workers.CloneProviderInferenceRequest(valid)
	invalid.RunnerID = ""
	unsupported := workers.CloneProviderInferenceRequest(valid)
	unsupported.RequiredOptionalCapabilities = []workers.RunnerOptionalCapability{
		workers.RunnerOptionalCapabilityImageInput,
	}
	failure := mockRequest("failing")
	capture := &workerexecution.MockWorkerUsageCapture{}
	failureResult, failureErr := registry.Execute(
		workerexecution.WithMockWorkerUsageCapture(t.Context(), capture),
		runners.ExecuteRequest{Identity: runners.MockIdentity, Attempt: failure},
	)
	if failureErr == nil {
		t.Fatal("reject Execute() error = nil, want mock rejection")
	}
	if got := capture.Usage(); got == nil || got.Provider != usage.Provider || got.Model != usage.Model {
		t.Fatalf("captured reject usage = %#v, want declared provider/model", got)
	}
	if failureResult.Diagnostics == nil || failureResult.Diagnostics.Provider == nil ||
		failureResult.Diagnostics.Provider.Provider != usage.Provider ||
		failureResult.Diagnostics.Provider.Model != usage.Model ||
		failureResult.Diagnostics.Provider.ResponseMetadata[workers.ProviderResponseMetadataInputTokens] != "10" ||
		failureResult.Diagnostics.Provider.ResponseMetadata[workers.ProviderResponseMetadataOutputTokens] != "5" {
		t.Fatalf("reject diagnostics = %#v, want declared provider usage", failureResult.Diagnostics)
	}

	testkit.RunService(t, testkit.ServiceSubject{
		Service:            registry,
		Identity:           runners.MockIdentity,
		ValidRequest:       valid,
		InvalidRequest:     invalid,
		UnsupportedRequest: unsupported,
		FailureRequest:     failure,
		ExpectedResult:     baseline,
		AssertCaptured:     func(*testing.T) {},
	})
}

func TestMockProductionRegistryResolveAndExecuteConcurrently(t *testing.T) {
	registry := newExplicitMockRegistry(t, &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			RunType: workers.MockWorkerRunTypeAccept,
		}},
	}, nil)

	const executions = 24
	var group sync.WaitGroup
	errs := make(chan error, executions)
	for range executions {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, resolveErr := registry.Resolve(runners.ResolutionRequest{
				Identity: runners.MockIdentity,
			}); resolveErr != nil {
				errs <- resolveErr
				return
			}
			result, executeErr := registry.Execute(t.Context(), runners.ExecuteRequest{
				Identity: runners.MockIdentity,
				Attempt:  mockRequest("writer"),
			})
			if executeErr != nil {
				errs <- executeErr
				return
			}
			if result.Content != "mock worker accepted" {
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

func TestMockRunnerMatchesResolvedAndDispatchWorkInputs(t *testing.T) {
	registry := newExplicitMockRegistry(t, &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{
			{
				WorkInputs: []workers.MockWorkInputSelector{{
					WorkID:    "resolved-work",
					InputName: "prerequisite",
				}},
				RunType: workers.MockWorkerRunTypeReject,
			},
			{
				WorkInputs: []workers.MockWorkInputSelector{{WorkID: "dispatch-work"}},
				RunType:    workers.MockWorkerRunTypeReject,
			},
		},
	}, nil)

	for _, test := range []struct {
		name    string
		request workers.RunnerExecutionRequest
	}{
		{
			name: "resolved Work input",
			request: workers.RunnerExecutionRequest{
				RunnerID: runners.MockIdentity,
				InputTokens: []any{workers.WorkInput{
					WorkID:     "resolved-work",
					InputNames: []string{"prerequisite"},
				}},
			},
		},
		{
			name: "dispatch token",
			request: workers.RunnerExecutionRequest{
				RunnerID: runners.MockIdentity,
				Dispatch: work.WorkDispatch{InputTokens: []any{map[string]any{
					"workId": "dispatch-work",
				}}},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := registry.Execute(t.Context(), runners.ExecuteRequest{
				Identity: runners.MockIdentity,
				Attempt:  test.request,
			})
			if err == nil {
				t.Fatal("Execute() error = nil, want selector-specific rejection")
			}
		})
	}
}

func TestNewMockProductionRegistryRejectsMissingConfig(t *testing.T) {
	_, err := NewMockProductionRegistry(
		runners.AgentDependencies{Providers: newAgentProvidersFake(), Publish: agentNoopPublisher},
		runners.ScriptConfig{Command: "fixture"},
		scriptDependencies(&scriptConformanceCommand{}, func(string) (map[string]string, error) {
			return nil, nil
		}),
		inferenceRegistryConfig(),
		inferenceDependencies(&inferenceConformanceModels{}, nil),
		runners.MockConfig{},
		runners.MockDependencies{},
	)
	if err == nil {
		t.Fatal("NewMockProductionRegistry() error = nil, want missing config")
	}
}

func newExplicitMockRegistry(
	t *testing.T,
	config *workers.MockWorkersConfig,
	next workerprocess.CommandRunner,
) runners.Service {
	t.Helper()
	registry, err := NewMockProductionRegistry(
		runners.AgentDependencies{Providers: newAgentProvidersFake(), Publish: agentNoopPublisher},
		runners.ScriptConfig{Command: "fixture"},
		scriptDependencies(&scriptConformanceCommand{}, func(string) (map[string]string, error) {
			return nil, nil
		}),
		inferenceRegistryConfig(),
		inferenceDependencies(&inferenceConformanceModels{}, nil),
		runners.MockConfig{WorkersConfig: config},
		runners.MockDependencies{Next: next},
	)
	if err != nil {
		t.Fatalf("NewMockProductionRegistry() error = %v", err)
	}
	return registry
}

func mockRequest(workerType string) workers.RunnerExecutionRequest {
	return workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-mock",
			InputTokens: []any{map[string]any{
				"nested": []any{"dispatch-original"},
			}},
		},
		RunnerID:    runners.MockIdentity,
		WorkerType:  workerType,
		UserMessage: "run mock",
		InputTokens: []any{map[string]any{
			"nested": []any{"original"},
		}},
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
		EnvVars: map[string]string{"FIXTURE": "original"},
	}
}

type mockCommandSpy struct {
	calls *atomic.Int32
}

func (spy *mockCommandSpy) Run(
	context.Context,
	workerprocess.CommandRequest,
) (workerprocess.CommandResult, error) {
	if spy.calls != nil {
		spy.calls.Add(1)
	}
	return workerprocess.CommandResult{Stdout: []byte("unused")}, nil
}
