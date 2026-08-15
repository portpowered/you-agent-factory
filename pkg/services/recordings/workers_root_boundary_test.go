package recordings_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type providersRootPortProbe struct{}

func (providersRootPortProbe) ListProviders(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (providersRootPortProbe) GetProvider(_ context.Context, request providers.GetProviderRequest) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{Provider: providers.Descriptor{ID: request.ID}}, nil
}

func (providersRootPortProbe) ResolveIdentity(_ context.Context, request providers.ResolveIdentityRequest) (providers.ResolveIdentityResult, error) {
	return providers.ResolveIdentityResult{ID: providers.ID(request.Identity)}, nil
}

func (providersRootPortProbe) ResolveSelection(_ context.Context, request providers.ResolveSelectionRequest) (providers.ResolveSelectionResult, error) {
	return providers.ResolveSelectionResult{Provider: providers.ID(request.ModelProvider)}, nil
}

func (providersRootPortProbe) ValidatePrerequisites(context.Context, providers.ValidatePrerequisitesRequest) error {
	return nil
}

func (providersRootPortProbe) Execute(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, nil
}

func (providersRootPortProbe) ControlAttempt(_ context.Context, request providers.ControlAttemptRequest) (providers.ControlAttemptResult, error) {
	return providers.ControlAttemptResult{
		Provider:  request.Provider,
		AttemptID: request.AttemptID,
		Action:    request.Action,
		Outcome:   providers.ControlOutcomeUnsupported,
	}, nil
}

func (providersRootPortProbe) Continue(_ context.Context, request providers.ContinueRequest) (providers.ContinueResult, error) {
	return providers.ContinueResult{Reference: request.Reference, Outcome: providers.ContinuationOutcomeUnsupported}, nil
}

func (providersRootPortProbe) Run(
	_ context.Context,
	_ workers.CommandRequest,
) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

// TestReplayBindingContractsAcceptProvidersRootPorts proves published replay
// binding contracts carry the Providers root rather than a Workers provider
// client.
func TestReplayBindingContractsAcceptWorkersRootPorts(t *testing.T) {
	t.Parallel()

	probe := providersRootPortProbe{}
	var provider providers.Service = probe
	var runner workers.CommandRunner = probe

	binding := recordings.BindReplayExecutionResult{
		Provider:      provider,
		CommandRunner: runner,
	}
	if binding.Provider == nil || binding.CommandRunner == nil {
		t.Fatal("BindReplayExecutionResult must accept workers root ports")
	}

	var factory recordings.ReplayExecutionFactory = func(
		_ *recordings.ReplayArtifact,
	) (
		providers.Service,
		workers.CommandRunner,
		[]recordings.ReplayHook,
		recordings.CompletionDeliveryPlanner,
		error,
	) {
		return provider, runner, nil, nil, nil
	}
	if factory == nil {
		t.Fatal("ReplayExecutionFactory must be constructible with workers root ports")
	}

	p, r, _, _, err := factory(&recordings.ReplayArtifact{SchemaVersion: "replay.v1"})
	if err != nil {
		t.Fatalf("ReplayExecutionFactory: %v", err)
	}
	if p == nil || r == nil {
		t.Fatalf("factory ports = (%v,%v), want non-nil provider and runner", p, r)
	}
}
