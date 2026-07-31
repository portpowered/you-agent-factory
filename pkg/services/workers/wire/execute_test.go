package wire

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

func TestNewServiceExecuteIsInertUntilCalled(t *testing.T) {
	t.Parallel()

	providersFake := &wireProvidersFake{}
	inputs := validNewServiceInputs()
	inputs.agentDependencies.Providers = providersFake

	var observations []workers.ExecutionObservation
	var mu sync.Mutex
	service, err := NewService(
		inputs.agentDependencies,
		inputs.scriptConfig,
		inputs.scriptDependencies,
		inputs.inferenceConfig,
		inputs.inferenceDependencies,
		ExecuteOptions{
			Observe: func(_ context.Context, observation workers.ExecutionObservation) error {
				mu.Lock()
				defer mu.Unlock()
				observations = append(observations, observation.Clone())
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if providersFake.calls.Load() != 0 {
		t.Fatalf("Providers.Execute called during construction: %d", providersFake.calls.Load())
	}

	result, err := service.Execute(context.Background(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			DispatchID: "dispatch-wire-1",
			AttemptID:  "attempt-wire-1",
		},
		Target: workers.ExecutionTarget{
			RunnerID: string(providers.IDCodex),
			Provider: workers.ProviderReference{ID: string(providers.IDCodex)},
			Prompt:   workers.PromptPolicy{UserMessage: "wire execute"},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	if result.Correlation.DispatchID != "dispatch-wire-1" {
		t.Fatalf("correlation = %#v", result.Correlation)
	}
	if providersFake.calls.Load() != 1 {
		t.Fatalf("Providers.Execute calls = %d, want 1", providersFake.calls.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(observations) < 2 {
		t.Fatalf("observations = %#v", observations)
	}
}

func TestNewServiceDoesNotRegisterMockAsProductionBypass(t *testing.T) {
	t.Parallel()

	service, err := validNewServiceInputs().callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Execute(context.Background(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			DispatchID: "dispatch-mock-blocked",
			AttemptID:  "attempt-mock-blocked",
		},
		Target: workers.ExecutionTarget{
			RunnerID: runners.MockIdentity,
			Prompt:   workers.PromptPolicy{UserMessage: "mock must not bypass"},
		},
	})
	if !errors.Is(err, workers.ErrInvalidExecuteRequest) {
		t.Fatalf("Execute(mock) error = %v, want ErrInvalidExecuteRequest", err)
	}
}
