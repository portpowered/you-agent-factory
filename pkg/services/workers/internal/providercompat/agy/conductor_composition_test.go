package agy_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	agypkg "github.com/portpowered/infinite-you/pkg/services/workers/internal/providercompat/agy"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/internal/providercompat/inferencecontract"
)

func TestProvidersServiceSelectsAntigravityThroughAuthoritativeCatalogIdentity(t *testing.T) {
	t.Parallel()

	providersService := newProductionAgyProvidersService(t)
	resolved, err := providersService.ResolveIdentity(
		context.Background(),
		providers.ResolveIdentityRequest{Identity: " ANTIGRAVITY "},
	)
	if err != nil {
		t.Fatalf("ResolveIdentity(antigravity) error = %v", err)
	}
	if resolved.ID != providers.IDAntigravity {
		t.Fatalf("ResolveIdentity identity = %q, want antigravity", resolved.ID)
	}
	descriptor, err := providersService.GetProvider(
		context.Background(),
		providers.GetProviderRequest{ID: resolved.ID},
	)
	if err != nil {
		t.Fatalf("GetProvider(antigravity) error = %v", err)
	}
	if descriptor.Provider.ID != providers.IDAntigravity ||
		descriptor.Provider.Availability != providers.AvailabilitySelectable {
		t.Fatalf("GetProvider(antigravity) = %#v, want selectable canonical descriptor", descriptor.Provider)
	}
	if !slices.Contains(descriptor.Provider.Capabilities, providers.CapabilityPromptSubmission) ||
		!slices.Contains(descriptor.Provider.Capabilities, providers.CapabilityMessageSnapshots) {
		t.Fatalf("GetProvider(antigravity) capabilities = %v, want prompt_submission and message_snapshots", descriptor.Provider.Capabilities)
	}
	if slices.Contains(descriptor.Provider.Capabilities, providers.CapabilityNativeStreaming) {
		t.Fatal("GetProvider(antigravity) overclaims native streaming")
	}
}

func TestAgyIntegrationInvokesProviderThroughProvidersRoot(t *testing.T) {
	t.Parallel()

	mock := &workers.MockPTYAllocator{
		Result: workers.PTYSessionResult{ExitCode: 0, CleanedText: "agy provider answer"},
	}
	providersService := newAgyProvidersServiceWithPTY(t, mock)
	integration := agypkg.NewIntegration(agypkg.IntegrationDependencies{
		ProvidersService: providersService,
	})
	destination := &orderedWriter{}
	providerSession := inference.NewProviderSession(
		string(providers.IDAntigravity),
		"transcript",
		"prior-session",
		map[string]string{"source": "test"},
	)
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID:    "inv-agy-integration",
		Model:           "agy-default",
		UserMessage:     "say hello",
		Required:        inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
		ProviderSession: &providerSession,
		Execution: workers.ProviderInferenceRequest{
			Dispatch: work.WorkDispatch{
				DispatchID: "inv-agy-integration",
			},
			WorkingDirectory: t.TempDir(),
		},
	})
	if _, err := providersService.GetProvider(
		context.Background(),
		providers.GetProviderRequest{ID: providers.IDAntigravity},
	); err != nil {
		t.Fatalf("providers.GetProvider(antigravity) error = %v", err)
	}
	err := inference.ExecuteInvocation(
		context.Background(),
		integration,
		request,
		destination,
	)
	if err != nil {
		t.Fatalf("ExecuteInvocation(agy) error = %v", err)
	}
	if destination.completion == nil || destination.completion.Response() == nil {
		t.Fatalf("completion = %#v, want successful response", destination.completion)
	}
	if got := destination.completion.Response().Content(); got != "agy provider answer" {
		t.Fatalf("response content = %q, want agy provider answer", got)
	}
	if len(mock.Sessions) != 1 {
		t.Fatalf("pty sessions = %d, want 1", len(mock.Sessions))
	}
}

func TestAgyIntegrationDoesNotOverclaimCapabilityBeforeProviderIO(t *testing.T) {
	t.Parallel()

	mock := &workers.MockPTYAllocator{
		Result: workers.PTYSessionResult{ExitCode: 0, CleanedText: "should-not-run"},
	}
	providersService := newAgyProvidersServiceWithPTY(t, mock)
	descriptor, err := providersService.GetProvider(
		context.Background(),
		providers.GetProviderRequest{ID: providers.IDAntigravity},
	)
	if err != nil {
		t.Fatalf("providers.GetProvider(antigravity) error = %v", err)
	}
	if slices.Contains(descriptor.Provider.Capabilities, providers.CapabilityNativeStreaming) {
		t.Fatal("GetProvider(antigravity) overclaims native streaming")
	}
	integration := agypkg.NewIntegration(agypkg.IntegrationDependencies{
		ProvidersService: providersService,
	})
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "inv-agy-escalate",
		UserMessage:  "hello",
		Required: inference.NewCapabilitySet(
			inference.CapabilityPromptSubmission,
			inference.CapabilityNativeStreaming,
			inference.CapabilityNativeStreaming,
		),
	})
	capabilities, err := integration.Capabilities(context.Background(), request)
	if err != nil {
		t.Fatalf("Agy Capabilities() error = %v", err)
	}
	if capabilities.Has(inference.CapabilityNativeStreaming) {
		t.Fatal("Agy integration overclaimed native streaming")
	}
	if len(mock.Sessions) != 0 {
		t.Fatalf("provider I/O occurred: pty sessions = %d", len(mock.Sessions))
	}
}

func TestAgyIntegrationClassifiesNativeFailureSafely(t *testing.T) {
	t.Parallel()

	mock := &workers.MockPTYAllocator{
		Result: workers.PTYSessionResult{
			ExitCode: 1,
			RawBytes: []byte("failed reading /tmp/secret-key and private prompt"),
		},
	}
	providersService := newAgyProvidersServiceWithPTY(t, mock)
	integration := agypkg.NewIntegration(agypkg.IntegrationDependencies{
		ProvidersService: providersService,
	})
	destination := &orderedWriter{}
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "inv-agy-failure",
		UserMessage:  "private prompt",
		Required:     inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
	})
	if _, err := providersService.GetProvider(
		context.Background(),
		providers.GetProviderRequest{ID: providers.IDAntigravity},
	); err != nil {
		t.Fatalf("providers.GetProvider(antigravity) error = %v", err)
	}
	err := inference.ExecuteInvocation(
		context.Background(),
		integration,
		request,
		destination,
	)
	if err != nil {
		t.Fatalf("ExecuteInvocation(agy failure) error = %v", err)
	}
	if destination.completion == nil || destination.completion.Failure() == nil {
		t.Fatalf("completion = %#v, want normalized failure", destination.completion)
	}
	failure := destination.completion.Failure()
	if strings.Contains(failure.Message(), "/tmp/") ||
		strings.Contains(failure.Message(), "secret-key") ||
		strings.Contains(failure.Message(), "private prompt") {
		t.Fatalf("failure message leaked unsafe detail: %q", failure.Message())
	}
}

func TestAgyIntegrationPropagatesCancellationWithTerminalFailure(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	integration := agypkg.NewIntegration(agypkg.IntegrationDependencies{
		ProvidersService: canceledProvidersFake{},
	})
	destination := &orderedWriter{}
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "inv-agy-canceled",
		UserMessage:  "hello",
	})
	err := inference.ExecuteInvocation(ctx, integration, request, destination)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ExecuteInvocation(canceled) error = %v, want context.Canceled", err)
	}
	if destination.completion == nil || destination.completion.Failure() == nil {
		t.Fatalf("completion = %#v, want cancellation failure", destination.completion)
	}
	if got := destination.completion.Failure().Kind(); got != inference.FailureCanceled {
		t.Fatalf("failure kind = %q, want %q", got, inference.FailureCanceled)
	}
}

type canceledProvidersFake struct {
	providers.Service
}

func (canceledProvidersFake) Execute(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, context.Canceled
}

func newProductionAgyProvidersService(t *testing.T) providers.Service {
	t.Helper()
	providersService, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("providerswire.NewService() error = %v", err)
	}
	return providersService
}
