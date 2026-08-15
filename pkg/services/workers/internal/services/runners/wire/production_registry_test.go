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
	if provider.calls.Load() != 0 || commandCalls.Load() != 0 || modelCalls.Load() != 0 {
		t.Fatalf("construction effects = provider %d command %d model %d, want zero",
			provider.calls.Load(), commandCalls.Load(), modelCalls.Load())
	}

	for _, identity := range []string{
		runners.AgentIdentity,
		runners.ScriptIdentity,
		runners.InferenceIdentity,
	} {
		binding, resolveErr := registry.Resolve(runners.ResolutionRequest{Identity: identity})
		if resolveErr != nil {
			t.Fatalf("Resolve(%q) error = %v", identity, resolveErr)
		}
		if binding.Identity != identity || binding.Runner == nil {
			t.Fatalf("Resolve(%q) = %#v, want complete binding", identity, binding)
		}
	}
	if _, err := registry.Resolve(runners.ResolutionRequest{Identity: runners.MockIdentity}); !errors.Is(err, workers.ErrUnknownRunnerSelection) {
		t.Fatalf("Resolve(mock) error = %v, want unknown production strategy", err)
	}
	if provider.calls.Load() != 0 || commandCalls.Load() != 0 || modelCalls.Load() != 0 {
		t.Fatalf("resolution effects = provider %d command %d model %d, want zero",
			provider.calls.Load(), commandCalls.Load(), modelCalls.Load())
	}

	productionRequests := []struct {
		identity string
		request  workers.RunnerExecutionRequest
		want     string
	}{
		{identity: runners.AgentIdentity, request: agentRequest(), want: "fixture output"},
		{identity: runners.ScriptIdentity, request: scriptRequest(), want: "fixture output"},
		{identity: runners.InferenceIdentity, request: inferenceRequest(), want: "fixture output"},
	}
	for _, test := range productionRequests {
		t.Run(test.identity, func(t *testing.T) {
			result, executeErr := registry.Execute(t.Context(), runners.ExecuteRequest{
				Identity: test.identity,
				Attempt:  test.request,
			})
			if executeErr != nil {
				t.Fatalf("Execute() error = %v", executeErr)
			}
			if result.Content != test.want {
				t.Fatalf("Execute() content = %q, want %q", result.Content, test.want)
			}
		})
	}
	if provider.calls.Load() != 1 || commandCalls.Load() != 1 || modelCalls.Load() != 1 {
		t.Fatalf("execution effects = provider %d command %d model %d, want one each",
			provider.calls.Load(), commandCalls.Load(), modelCalls.Load())
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
