package wire

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	runnerinference "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/inference"
)

func TestNewProductionRegistryBuildsOneInertRegistryForEveryStrategy(t *testing.T) {
	var commandCalls atomic.Int32
	var modelCalls atomic.Int32
	provider := newAgentProvidersFake()
	command := &scriptConformanceCommand{calls: &commandCalls}
	modelsEdge := &inferenceConformanceModels{calls: &modelCalls}

	registry, err := NewProductionRegistry(
		runners.AgentDependencies{
			Providers: provider,
			Publish:   agentNoopPublisher,
		},
		runners.ScriptConfig{Command: "fixture"},
		scriptDependencies(command, func(string) (map[string]string, error) {
			return nil, nil
		}),
		inferenceRegistryConfig(),
		inferenceDependencies(modelsEdge, nil),
	)
	if err != nil {
		t.Fatalf("NewProductionRegistry() error = %v", err)
	}
	assertRegistryEffects(t, provider, &commandCalls, &modelCalls, 0, "construction")
	assertProductionBindings(t, registry)
	assertRegistryEffects(t, provider, &commandCalls, &modelCalls, 0, "resolution")
	assertProductionExecutions(t, registry)
	assertRegistryEffects(t, provider, &commandCalls, &modelCalls, 1, "execution")
}

func assertRegistryEffects(
	t *testing.T,
	provider *agentProvidersFake,
	commandCalls, modelCalls *atomic.Int32,
	want int32,
	stage string,
) {
	t.Helper()
	if provider.calls.Load() != want || commandCalls.Load() != want || modelCalls.Load() != want {
		t.Fatalf("%s effects = provider %d command %d model %d, want %d each",
			stage, provider.calls.Load(), commandCalls.Load(), modelCalls.Load(), want)
	}
}

func assertProductionBindings(t *testing.T, registry runners.Service) {
	t.Helper()
	for _, identity := range []string{
		runners.AgentIdentity,
		runners.ScriptIdentity,
		runners.InferenceIdentity,
	} {
		binding, err := registry.Resolve(runners.ResolutionRequest{Identity: identity})
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v", identity, err)
		}
		if binding.Identity != identity || binding.Runner == nil {
			t.Fatalf("Resolve(%q) = %#v, want complete binding", identity, binding)
		}
	}
	if _, err := registry.Resolve(runners.ResolutionRequest{Identity: runners.MockIdentity}); !errors.Is(err, workers.ErrUnknownRunnerSelection) {
		t.Fatalf("Resolve(mock) error = %v, want unknown production strategy", err)
	}
}

func assertProductionExecutions(t *testing.T, registry runners.Service) {
	t.Helper()
	requests := []struct {
		identity string
		request  workers.RunnerExecutionRequest
	}{
		{identity: runners.AgentIdentity, request: agentRequest()},
		{identity: runners.ScriptIdentity, request: scriptRequest()},
		{identity: runners.InferenceIdentity, request: inferenceRequest()},
	}
	for _, test := range requests {
		t.Run(test.identity, func(t *testing.T) {
			result, err := registry.Execute(t.Context(), runners.ExecuteRequest{
				Identity: test.identity,
				Attempt:  test.request,
			})
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if result.Content != "fixture output" {
				t.Fatalf("Execute() content = %q, want fixture output", result.Content)
			}
		})
	}
}

func TestNewProductionRegistryResolvesAndExecutesConcurrently(t *testing.T) {
	provider := newAgentProvidersFake()
	command := &scriptConformanceCommand{}
	modelsEdge := &inferenceConformanceModels{}
	registry, err := NewProductionRegistry(
		runners.AgentDependencies{Providers: provider, Publish: agentNoopPublisher},
		runners.ScriptConfig{Command: "fixture"},
		scriptDependencies(command, func(string) (map[string]string, error) { return nil, nil }),
		inferenceRegistryConfig(),
		inferenceDependencies(modelsEdge, nil),
	)
	if err != nil {
		t.Fatalf("NewProductionRegistry() error = %v", err)
	}

	const executionsPerStrategy = 16
	identities := []string{
		runners.AgentIdentity,
		runners.ScriptIdentity,
		runners.InferenceIdentity,
	}
	errs := make(chan error, len(identities)*executionsPerStrategy)
	var group sync.WaitGroup
	for _, identity := range identities {
		identity := identity
		for index := 0; index < executionsPerStrategy; index++ {
			index := index
			group.Add(1)
			go func() {
				defer group.Done()
				if _, resolveErr := registry.Resolve(runners.ResolutionRequest{Identity: identity}); resolveErr != nil {
					errs <- resolveErr
					return
				}
				request := productionRegistryRequest(identity, index)
				result, executeErr := registry.Execute(t.Context(), runners.ExecuteRequest{
					Identity: identity,
					Attempt:  request,
				})
				if executeErr != nil {
					errs <- executeErr
					return
				}
				if result.Content != "fixture output" {
					errs <- fmt.Errorf("%s result content = %q", identity, result.Content)
				}
			}()
		}
	}
	group.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent %s: %v", "resolve/execute", err)
	}
}

func TestNewProductionRegistryInferenceFallsBackThroughAgentRunner(t *testing.T) {
	provider := newAgentProvidersFake()
	modelsEdge := &inferenceConformanceModels{}
	registry, err := NewProductionRegistry(
		runners.AgentDependencies{
			Providers: provider,
			Publish:   agentNoopPublisher,
		},
		runners.ScriptConfig{Command: "fixture"},
		scriptDependencies(&scriptConformanceCommand{}, func(string) (map[string]string, error) {
			return nil, nil
		}),
		inferenceRegistryConfig(),
		inferenceDependencies(modelsEdge, nil),
	)
	if err != nil {
		t.Fatalf("NewProductionRegistry() error = %v", err)
	}

	request := inferenceRequest()
	request.ModelOperation = inferenceFixtureExecutionFailure
	request.ModelProvider = workers.RunnerIDCodex
	request.UserMessage = "provider fallback"
	result, err := registry.Execute(t.Context(), runners.ExecuteRequest{
		Identity: runners.InferenceIdentity,
		Attempt:  request,
	})
	if err != nil {
		t.Fatalf("inference fallback Execute() error = %v", err)
	}
	if result.Content != "fixture output" {
		t.Fatalf("inference fallback content = %q, want provider output", result.Content)
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want one fallback attempt", provider.calls.Load())
	}
	if got := provider.Request().Provider; got != providers.IDCodex {
		t.Fatalf("fallback provider = %q, want %q", got, providers.IDCodex)
	}
}

func TestNewProductionRegistryPreservesStrategyConstructionErrors(t *testing.T) {
	cases := []struct {
		name     string
		identity string
	}{
		{name: "agent", identity: runners.AgentIdentity},
		{name: "script", identity: runners.ScriptIdentity},
		{name: "inference", identity: runners.InferenceIdentity},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			agentDependencies, scriptConfig, scriptDeps, inferenceConfig, inferenceDeps := validProductionRegistryInputs()
			switch test.identity {
			case runners.AgentIdentity:
				agentDependencies.Providers = nil
			case runners.ScriptIdentity:
				scriptConfig.Command = ""
				scriptConfig.RequestSelected = false
			case runners.InferenceIdentity:
				inferenceConfig = runners.InferenceConfig{}
			}

			_, err := NewProductionRegistry(
				agentDependencies,
				scriptConfig,
				scriptDeps,
				inferenceConfig,
				inferenceDeps,
			)
			if !errors.Is(err, workers.ErrInvalidRunnerRegistration) {
				t.Fatalf("NewProductionRegistry(%s) error = %v, want invalid registration", test.identity, err)
			}
			if !strings.Contains(err.Error(), test.identity) {
				t.Fatalf("NewProductionRegistry(%s) error = %v, want strategy identity", test.identity, err)
			}
		})
	}
}

func TestNewInferenceCompositionRunnerDelegatesThroughRegistry(t *testing.T) {
	var localRequest models.LocalInvocationRequest
	local := localInvokerFunc(func(_ context.Context, request models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
		localRequest = request
		return models.LocalInvocationResult{Handled: false}, nil
	})
	delegate := &compositionDelegate{}
	runner := NewInferenceCompositionRunner(
		delegate,
		local,
		models.RuntimeScopeRef{},
		&interfaces.FactoryWorkerConfig{
			Name:          "whisper-worker",
			Type:          interfaces.WorkerTypeInference,
			Model:         "WHISPER",
			ModelLocality: models.RuntimeModelLocalityLocal,
		},
		[]interfaces.ResourceConfig{{
			ID: "resource-1", Name: "gpu", Type: "MODEL", Capacity: 1, Model: "WHISPER",
		}},
	)

	request := inferenceRequest()
	request.RunnerID = workers.RunnerIDCodex
	request.ModelOperation = "transcribe"
	result, err := runner.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "delegated output" {
		t.Fatalf("Execute() content = %q, want delegated output", result.Content)
	}
	if delegate.calls != 1 || delegate.request.RunnerID != workers.RunnerIDCodex {
		t.Fatalf("delegate request = %#v calls=%d, want one codex attempt", delegate.request, delegate.calls)
	}
	if localRequest.ModelOperation != request.ModelOperation {
		t.Fatalf("Models operation = %q, want %q", localRequest.ModelOperation, request.ModelOperation)
	}
}

func TestNewInferenceCompositionRunnerLeavesIncompleteDependenciesUntouched(t *testing.T) {
	delegate := &compositionDelegate{}
	local := localInvokerFunc(func(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
		return models.LocalInvocationResult{Handled: true, Content: "unexpected"}, nil
	})
	worker := &interfaces.FactoryWorkerConfig{
		Name:          "whisper-worker",
		Type:          interfaces.WorkerTypeInference,
		ModelLocality: models.RuntimeModelLocalityLocal,
	}

	cases := []struct {
		name   string
		inner  workers.Runner
		models runnerinference.LocalInvoker
		worker *interfaces.FactoryWorkerConfig
	}{
		{name: "missing inner", inner: nil, models: local, worker: worker},
		{name: "missing Models", inner: delegate, models: nil, worker: worker},
		{name: "missing worker", inner: delegate, models: local, worker: nil},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := NewInferenceCompositionRunner(
				test.inner, test.models, models.RuntimeScopeRef{}, test.worker, nil,
			)
			if got != test.inner {
				t.Fatalf("NewInferenceCompositionRunner() = %T, want original %T", got, test.inner)
			}
		})
	}
}

func TestRegistryExecutionRunnerRequiresRegistry(t *testing.T) {
	_, err := (registryExecutionRunner{identity: runners.InferenceIdentity}).Execute(
		t.Context(), workers.RunnerExecutionRequest{},
	)
	if err == nil {
		t.Fatal("registryExecutionRunner.Execute() error = nil, want missing registry error")
	}
}

func validProductionRegistryInputs() (
	runners.AgentDependencies,
	runners.ScriptConfig,
	runners.ScriptDependencies,
	runners.InferenceConfig,
	runners.InferenceDependencies,
) {
	agentDependencies := runners.AgentDependencies{
		Providers: newAgentProvidersFake(),
		Publish:   agentNoopPublisher,
	}
	scriptConfig := runners.ScriptConfig{Command: "fixture"}
	scriptDeps := scriptDependencies(&scriptConformanceCommand{}, func(string) (map[string]string, error) {
		return nil, nil
	})
	inferenceConfig := inferenceRegistryConfig()
	inferenceDeps := inferenceDependencies(&inferenceConformanceModels{}, nil)
	return agentDependencies, scriptConfig, scriptDeps, inferenceConfig, inferenceDeps
}

type localInvokerFunc func(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error)

func (fn localInvokerFunc) InvokeLocal(
	ctx context.Context,
	request models.LocalInvocationRequest,
) (models.LocalInvocationResult, error) {
	return fn(ctx, request)
}

type compositionDelegate struct {
	calls   int
	request workers.RunnerExecutionRequest
}

func (delegate *compositionDelegate) Execute(
	_ context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	delegate.calls++
	delegate.request = request
	return workers.RunnerExecutionResult{Content: "delegated output"}, nil
}

func productionRegistryRequest(identity string, index int) workers.RunnerExecutionRequest {
	var request workers.RunnerExecutionRequest
	switch identity {
	case runners.AgentIdentity:
		request = agentRequest()
		request.Dispatch.DispatchID = fmt.Sprintf("agent-%d", index)
	case runners.ScriptIdentity:
		request = scriptRequest()
		request.Dispatch.DispatchID = fmt.Sprintf("script-%d", index)
	case runners.InferenceIdentity:
		request = inferenceRequest()
		request.Dispatch.DispatchID = fmt.Sprintf("inference-%d", index)
	}
	return request
}
