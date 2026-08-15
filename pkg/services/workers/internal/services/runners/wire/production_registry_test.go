package wire

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
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
