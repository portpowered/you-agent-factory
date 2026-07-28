package wire

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

func TestNewServiceConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	service, err := validNewServiceInputs().callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root workers.Service = service
	if root == nil {
		t.Fatal("constructed root is nil")
	}
}

func TestNewServiceAssignsRuntimeRolesWithoutLifecycle(t *testing.T) {
	t.Parallel()

	service, err := validNewServiceInputs().callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}

	for _, runnerID := range []string{
		runners.AgentIdentity,
		runners.ScriptIdentity,
		runners.InferenceIdentity,
	} {
		t.Run(runnerID, func(t *testing.T) {
			t.Parallel()

			result, err := service.BuildRuntime(t.Context(), workers.RuntimeBuildRequest{
				RunnerID: runnerID,
				Roles: []workers.RuntimeBuildRoleRequest{
					{Name: "writer", Kind: workers.RuntimeBuildRoleKindWorker},
					{Name: "review", Kind: workers.RuntimeBuildRoleKindWorkstation},
				},
			})
			if err != nil {
				t.Fatalf("BuildRuntime(%q) error = %v", runnerID, err)
			}
			if result.RunnerSelection.RunnerID != runnerID {
				t.Fatalf(
					"BuildRuntime(%q) selection = %#v, want runner %q",
					runnerID,
					result.RunnerSelection,
					runnerID,
				)
			}
			if len(result.Bindings) != 2 {
				t.Fatalf("BuildRuntime(%q) bindings = %#v, want two", runnerID, result.Bindings)
			}
			for _, binding := range result.Bindings {
				if binding.RunnerSelection.RunnerID != runnerID {
					t.Fatalf("binding selection = %#v, want runner %q", binding.RunnerSelection, runnerID)
				}
			}
		})
	}
}

func TestNewServiceRejectsUnknownRunnerSelection(t *testing.T) {
	t.Parallel()

	service, err := validNewServiceInputs().callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.BuildRuntime(t.Context(), workers.RuntimeBuildRequest{
		RunnerID: workers.RunnerIDCodex,
		Roles: []workers.RuntimeBuildRoleRequest{
			{Name: "writer", Kind: workers.RuntimeBuildRoleKindWorker},
		},
	})
	if !errors.Is(err, workers.ErrUnknownRunnerSelection) {
		t.Fatalf("BuildRuntime(codex) error = %v, want ErrUnknownRunnerSelection", err)
	}
}

func TestNewServiceDoesNotRegisterHostedRunner(t *testing.T) {
	t.Parallel()

	service, err := validNewServiceInputs().callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.BuildRuntime(t.Context(), workers.RuntimeBuildRequest{
		RunnerID: "hosted",
		Roles: []workers.RuntimeBuildRoleRequest{
			{Name: "writer", Kind: workers.RuntimeBuildRoleKindWorker},
		},
	})
	if !errors.Is(err, workers.ErrUnknownRunnerSelection) {
		t.Fatalf("BuildRuntime(hosted) error = %v, want ErrUnknownRunnerSelection", err)
	}
}

type newServiceInputs struct {
	agentDependencies     runners.AgentDependencies
	scriptConfig          runners.ScriptConfig
	scriptDependencies    runners.ScriptDependencies
	inferenceConfig       runners.InferenceConfig
	inferenceDependencies runners.InferenceDependencies
}

func validNewServiceInputs() newServiceInputs {
	return newServiceInputs{
		agentDependencies: runners.AgentDependencies{
			Providers: &wireProvidersFake{},
			Publish:   func(workers.ProgressFragment) {},
		},
		scriptConfig: runners.ScriptConfig{
			Command:          "fixture",
			Args:             []string{"arg"},
			FactoryDirectory: "factory-root",
		},
		scriptDependencies: runners.ScriptDependencies{
			CommandRunner: &wireStreamingCommandRunner{},
			FactoryDocs:   func(string) (map[string]string, error) { return map[string]string{}, nil },
			Now:           func() time.Time { return time.Unix(1, 0) },
			Publish:       func(workers.ProgressFragment) {},
			Record:        func(workers.ScriptEvent) {},
		},
		inferenceConfig: runners.InferenceConfig{
			Worker: models.LocalWorker{
				Name:  "inference-worker",
				Type:  interfaces.WorkerTypeInference,
				Model: "WHISPER",
			},
			Resources: []models.LocalResource{{
				Name: "gpu",
				Type: "gpu",
			}},
		},
		inferenceDependencies: runners.InferenceDependencies{
			Models: &wireInferenceInvoker{},
		},
	}
}

func (in newServiceInputs) callNewService() (workers.Service, error) {
	return NewService(
		in.agentDependencies,
		in.scriptConfig,
		in.scriptDependencies,
		in.inferenceConfig,
		in.inferenceDependencies,
	)
}

type wireProvidersFake struct {
	calls atomic.Int32
}

func (fake *wireProvidersFake) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.calls.Add(1)
	return providers.ExecuteResult{Content: "fixture"}, nil
}

func (*wireProvidersFake) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (*wireProvidersFake) GetProvider(
	context.Context,
	providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, nil
}

type wireStreamingCommandRunner struct{}

func (*wireStreamingCommandRunner) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

func (*wireStreamingCommandRunner) RunStreaming(
	context.Context,
	workers.CommandRequest,
	platformprocess.OutputChunkObserver,
) (workers.CommandResult, error) {
	return workers.CommandResult{ExitCode: 0}, nil
}

type wireInferenceInvoker struct {
	calls atomic.Int32
}

func (invoker *wireInferenceInvoker) InvokeLocal(
	context.Context,
	models.LocalInvocationRequest,
) (models.LocalInvocationResult, error) {
	invoker.calls.Add(1)
	return models.LocalInvocationResult{}, nil
}

var _ providers.Service = (*wireProvidersFake)(nil)
