package wire

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/mock"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/testkit"
)

func TestNewMockRegistryIsInertAndResolvesWithoutExecution(t *testing.T) {
	var commandCalls atomic.Int32
	registry, err := NewMockRegistry(
		runners.MockConfig{
			WorkersConfig: &workers.MockWorkersConfig{
				MockWorkers: []workers.MockWorkerConfig{{
					WorkerName: "writer",
					RunType:    workers.MockWorkerRunTypeAccept,
				}},
			},
		},
		runners.MockDependencies{
			Next: &mockCommandSpy{calls: &commandCalls},
		},
	)
	if err != nil {
		t.Fatalf("NewMockRegistry() error = %v", err)
	}
	if commandCalls.Load() != 0 {
		t.Fatalf("construction command calls = %d, want 0", commandCalls.Load())
	}

	binding, err := registry.Resolve(runners.ResolutionRequest{Identity: mock.Identity})
	if err != nil {
		t.Fatalf("Resolve(mock) error = %v", err)
	}
	if binding.Identity != mock.Identity || binding.Runner == nil {
		t.Fatalf("Resolve(mock) = %#v, want complete mock binding", binding)
	}
	if commandCalls.Load() != 0 {
		t.Fatalf("resolution command calls = %d, want 0", commandCalls.Load())
	}
}

func TestMockRunnerThroughRegistryConformsToCommonContract(t *testing.T) {
	registry, err := NewMockRegistry(
		runners.MockConfig{
			WorkersConfig: &workers.MockWorkersConfig{
				MockWorkers: []workers.MockWorkerConfig{{
					WorkerName: "writer",
					RunType:    workers.MockWorkerRunTypeAccept,
				}, {
					WorkerName: "failing",
					RunType:    workers.MockWorkerRunTypeReject,
				}},
			},
		},
		runners.MockDependencies{},
	)
	if err != nil {
		t.Fatalf("NewMockRegistry() error = %v", err)
	}

	valid := mockRequest("writer")
	baseline, err := registry.Execute(t.Context(), runners.ExecuteRequest{
		Identity: mock.Identity,
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

	testkit.RunService(t, testkit.ServiceSubject{
		Service:            registry,
		Identity:           mock.Identity,
		ValidRequest:       valid,
		InvalidRequest:     invalid,
		UnsupportedRequest: unsupported,
		FailureRequest:     failure,
		ExpectedResult:     baseline,
		AssertCaptured:     func(*testing.T) {},
	})
}

func TestMockRegistryResolveAndExecuteConcurrently(t *testing.T) {
	registry, err := NewMockRegistry(
		runners.MockConfig{
			WorkersConfig: &workers.MockWorkersConfig{
				MockWorkers: []workers.MockWorkerConfig{{
					RunType: workers.MockWorkerRunTypeAccept,
				}},
			},
		},
		runners.MockDependencies{},
	)
	if err != nil {
		t.Fatalf("NewMockRegistry() error = %v", err)
	}

	const executions = 24
	var group sync.WaitGroup
	errs := make(chan error, executions)
	for range executions {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, resolveErr := registry.Resolve(runners.ResolutionRequest{
				Identity: mock.Identity,
			}); resolveErr != nil {
				errs <- resolveErr
				return
			}
			result, executeErr := registry.Execute(t.Context(), runners.ExecuteRequest{
				Identity: mock.Identity,
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

func TestNewMockRegistryRejectsMissingConfig(t *testing.T) {
	_, err := NewMockRegistry(runners.MockConfig{}, runners.MockDependencies{})
	if err == nil {
		t.Fatal("NewMockRegistry() error = nil, want missing config")
	}
}

func mockRequest(workerType string) workers.RunnerExecutionRequest {
	return workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-mock",
			InputTokens: []any{map[string]any{
				"nested": []any{"dispatch-original"},
			}},
		},
		RunnerID:    mock.Identity,
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
	workers.CommandRequest,
) (workers.CommandResult, error) {
	if spy.calls != nil {
		spy.calls.Add(1)
	}
	return workers.CommandResult{Stdout: []byte("unused")}, nil
}
