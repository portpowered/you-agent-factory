package agy_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	agypkg "github.com/portpowered/infinite-you/pkg/services/workers/internal/providercompat/agy"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/internal/providercompat/inferencecontract"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/providercompat/registry"
)

func TestBuiltInRegistrySelectsAntigravityThroughAuthoritativeManifestIdentity(t *testing.T) {
	t.Parallel()

	providers := newProductionAgyRegistry(t)
	entry, err := providers.Lookup(" ANTIGRAVITY ")
	if err != nil {
		t.Fatalf("Lookup(antigravity) error = %v", err)
	}
	if entry.Identity() != inference.Identity(modelprovider.ProviderAntigravity) {
		t.Fatalf("Lookup identity = %q, want antigravity", entry.Identity())
	}
	integration, err := providers.Integration(string(modelprovider.ProviderAntigravity))
	if err != nil {
		t.Fatalf("Integration(agy) error = %v", err)
	}
	if integration.Identity() != inference.Identity(modelprovider.ProviderAntigravity) {
		t.Fatalf("Integration identity = %q, want agy", integration.Identity())
	}
	maximum := integration.MaximumCapabilities()
	if !maximum.Has(inference.CapabilityPromptSubmission) || !maximum.Has(inference.CapabilityMessageSnapshots) {
		t.Fatalf("MaximumCapabilities() = %v, want prompt_submission and message_snapshots", maximum.Values())
	}
	if providers.UsesNativeRunner(string(modelprovider.ProviderAntigravity)) {
		t.Fatal("UsesNativeRunner(agy) = true, want migrated Agy route")
	}
}

func TestAgyIntegrationInvokesProviderThroughProvidersRoot(t *testing.T) {
	t.Parallel()

	mock := &workers.MockPTYAllocator{
		Result: workers.PTYSessionResult{ExitCode: 0, CleanedText: "agy provider answer"},
	}
	providersService := newAgyProvidersServiceWithPTY(t, mock)
	registryProviders := newAgyRegistryWithService(t, providersService)
	integration := agypkg.NewIntegration(agypkg.IntegrationDependencies{
		ProvidersService: providersService,
	})
	destination := &orderedWriter{}
	providerSession := inference.NewProviderSession(
		string(modelprovider.ProviderAntigravity),
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
	if _, err := registryProviders.Capabilities(
		context.Background(),
		string(modelprovider.ProviderAntigravity),
		request,
	); err != nil {
		t.Fatalf("registry.Capabilities(antigravity) error = %v", err)
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

func TestAgyRegistryRejectsCapabilityEscalationBeforeProviderIO(t *testing.T) {
	t.Parallel()

	mock := &workers.MockPTYAllocator{
		Result: workers.PTYSessionResult{ExitCode: 0, CleanedText: "should-not-run"},
	}
	providers := newAgyRegistryWithPTY(t, mock)
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "inv-agy-escalate",
		UserMessage:  "hello",
		Required: inference.NewCapabilitySet(
			inference.CapabilityPromptSubmission,
			inference.CapabilityNativeStreaming,
			inference.CapabilityNativeStreaming,
		),
	})
	_, err := providers.Capabilities(
		context.Background(),
		string(modelprovider.ProviderAntigravity),
		request,
	)
	if err == nil {
		t.Fatal("registry.Capabilities(escalation) error = nil, want rejection")
	}
	var validation *inference.ValidationError
	if !errors.As(err, &validation) ||
		!strings.Contains(validation.Message, string(inference.CapabilityNativeStreaming)) {
		t.Fatalf("registry.Capabilities(escalation) error = %v, want native_streaming validation", err)
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
	registryProviders := newAgyRegistryWithService(t, providersService)
	integration := agypkg.NewIntegration(agypkg.IntegrationDependencies{
		ProvidersService: providersService,
	})
	destination := &orderedWriter{}
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "inv-agy-failure",
		UserMessage:  "private prompt",
		Required:     inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
	})
	if _, err := registryProviders.Capabilities(
		context.Background(),
		string(modelprovider.ProviderAntigravity),
		request,
	); err != nil {
		t.Fatalf("registry.Capabilities(antigravity) error = %v", err)
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

func newProductionAgyRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	registrations, err := registry.BuiltInRegistrations()
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	providers, err := registry.New(registrations...)
	if err != nil {
		t.Fatalf("registry.New() error = %v", err)
	}
	return providers
}

func newAgyRegistryWithPTY(t *testing.T, allocator *workers.MockPTYAllocator) *registry.Registry {
	t.Helper()
	providersService := newAgyProvidersServiceWithPTY(t, allocator)
	return newAgyRegistryWithService(t, providersService)
}

func newAgyRegistryWithService(t *testing.T, providersService providers.Service) *registry.Registry {
	t.Helper()
	registrations, err := registry.BuiltInRegistrations(registry.BuiltInDependencies{
		ProvidersService: providersService,
	})
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	providers, err := registry.New(registrations...)
	if err != nil {
		t.Fatalf("registry.New() error = %v", err)
	}
	return providers
}
