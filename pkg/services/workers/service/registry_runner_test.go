package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
)

type registryRunnerRecorder struct {
	calls int
}

func (r *registryRunnerRecorder) Execute(
	context.Context,
	workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	r.calls++
	return workers.RunnerExecutionResult{}, nil
}

func TestRegistryCapabilityRunnerUsesManifestMaximumBeforeNativeExecution(t *testing.T) {
	t.Parallel()
	providers := builtInProviderRegistry(t)
	next := &registryRunnerRecorder{}
	runner := registryCapabilityRunner{next: next, providers: providers}

	_, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		RunnerID: workers.RunnerIDGemini,
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilitySessionResume,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "session resume is not supported by the gemini runner") {
		t.Fatalf("Execute() error = %v", err)
	}
	if next.calls != 0 {
		t.Fatalf("native runner calls = %d, want 0", next.calls)
	}

	_, err = runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		RunnerID: workers.RunnerIDCodex,
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilityStructuredOutput,
		},
	})
	if err != nil {
		t.Fatalf("Execute(codex) error = %v", err)
	}
	if next.calls != 1 {
		t.Fatalf("native runner calls = %d, want 1", next.calls)
	}
}

func TestRegistrySelectionRejectsUnknownInsteadOfDefaulting(t *testing.T) {
	t.Parallel()
	providers := builtInProviderRegistry(t)

	_, err := resolveRuntimeRunnerSelection(providers, "", "", "unknown-provider")
	if err == nil || !strings.Contains(err.Error(), `provider "unknown-provider" is unknown`) {
		t.Fatalf("resolveRuntimeRunnerSelection() error = %v", err)
	}
}

func TestRuntimeSelectionCompatibilityWithoutRegistry(t *testing.T) {
	t.Parallel()

	selection, err := resolveRuntimeRunnerSelection(nil, "", "", workers.RunnerIDCodex)
	if err != nil {
		t.Fatalf("resolveRuntimeRunnerSelection() error = %v", err)
	}
	if selection.RunnerID != workers.RunnerIDCodex {
		t.Fatalf("resolveRuntimeRunnerSelection() = %#v", selection)
	}
	if err := validateRuntimeRunnerIdentity(nil, workers.RunnerIDCodex); err != nil {
		t.Fatalf("validateRuntimeRunnerIdentity(codex) error = %v", err)
	}
	if err := validateRuntimeRunnerIdentity(nil, "unknown-provider"); err == nil {
		t.Fatal("validateRuntimeRunnerIdentity(unknown) succeeded")
	}
}

func TestRegistryCapabilityValidationWithoutRegistryPreservesNativeRunner(t *testing.T) {
	t.Parallel()

	next := &registryRunnerRecorder{}
	runner := registryCapabilityRunner{next: next}
	_, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		RunnerID: workers.RunnerIDCodex,
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilityStructuredOutput,
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if next.calls != 1 {
		t.Fatalf("native runner calls = %d, want 1", next.calls)
	}
}

func TestRegistryCapabilityRunnerRejectsUnknownRunnerBeforeNativeExecution(t *testing.T) {
	t.Parallel()

	next := &registryRunnerRecorder{}
	runner := registryCapabilityRunner{next: next, providers: builtInProviderRegistry(t)}
	_, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		RunnerID: "unknown-provider",
	})
	if err == nil || !strings.Contains(err.Error(), `provider "unknown-provider" is unknown`) {
		t.Fatalf("Execute() error = %v, want unknown-provider diagnostic", err)
	}
	if next.calls != 0 {
		t.Fatalf("native runner calls = %d, want 0", next.calls)
	}
}

func builtInProviderRegistry(t *testing.T) *providerregistry.Registry {
	t.Helper()
	registrations, err := providerregistry.BuiltInRegistrations()
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	providers, err := providerregistry.New(registrations...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return providers
}

type conductorRouteRecordingRunner struct {
	calls int
}

func (r *conductorRouteRecordingRunner) Execute(
	context.Context,
	workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	r.calls++
	return workers.RunnerExecutionResult{Content: "native"}, nil
}

type successfulConductorIntegration struct {
	identity inference.Identity
	maximum  inference.CapabilitySet
	calls    int
}

func (i *successfulConductorIntegration) Identity() inference.Identity { return i.identity }

func (i *successfulConductorIntegration) MaximumCapabilities() inference.CapabilitySet {
	return inference.NewCapabilitySet(i.maximum.Values()...)
}

func (i *successfulConductorIntegration) Discover(context.Context) (inference.Discovery, error) {
	return inference.NewDiscovery(inference.ReadinessReady), nil
}

func (i *successfulConductorIntegration) Capabilities(
	context.Context,
	inference.InvocationRequest,
) (inference.CapabilitySet, error) {
	return i.MaximumCapabilities(), nil
}

func (i *successfulConductorIntegration) Invoke(
	ctx context.Context,
	_ inference.InvocationRequest,
	writer inference.ResponseWriter,
) error {
	i.calls++
	return writer.Close(ctx, inference.SuccessfulCompletion(inference.NewResponse(inference.ResponseInput{
		Content: "conductor-ok",
	})))
}

func TestConductorInvocationRunnerRoutesExternalIntegrationsThroughConductor(t *testing.T) {
	t.Parallel()

	providers, integration := externalConductorRegistry(t)
	native := &conductorRouteRecordingRunner{}
	runner := conductorInvocationRunner{
		next:      native,
		conductor: conductor.New(providers),
		providers: providers,
	}

	result, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		Dispatch:    work.WorkDispatch{DispatchID: "dispatch-conductor-1"},
		RunnerID:    "customer.provider",
		Model:       "fixture-model",
		UserMessage: "hello",
	})
	if err != nil {
		t.Fatalf("Execute(external) error = %v", err)
	}
	if result.Content != "conductor-ok" {
		t.Fatalf("Execute(external) content = %q, want conductor-ok", result.Content)
	}
	if integration.calls != 1 {
		t.Fatalf("integration invoke calls = %d, want 1", integration.calls)
	}
	if native.calls != 0 {
		t.Fatalf("native runner calls = %d, want 0", native.calls)
	}
}

func TestConductorInvocationRunnerPreservesNativeBuiltInPath(t *testing.T) {
	t.Parallel()

	providers, integration := externalConductorRegistry(t)
	native := &conductorRouteRecordingRunner{}
	runner := conductorInvocationRunner{
		next:      native,
		conductor: conductor.New(providers),
		providers: providers,
	}

	result, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-native-1"},
		RunnerID: workers.RunnerIDCodex,
	})
	if err != nil {
		t.Fatalf("Execute(codex) error = %v", err)
	}
	if result.Content != "native" {
		t.Fatalf("Execute(codex) content = %q, want native", result.Content)
	}
	if native.calls != 1 {
		t.Fatalf("native runner calls = %d, want 1", native.calls)
	}
	if integration.calls != 0 {
		t.Fatalf("integration invoke calls = %d, want 0", integration.calls)
	}
}

func TestConductorInvocationRunnerBypassedWhenProviderOverrideDisablesRegistryDecorators(t *testing.T) {
	t.Parallel()

	providers, _ := externalConductorRegistry(t)
	service := &Service{
		providerRegistry:    providers,
		invocationConductor: conductor.New(providers),
	}
	decorators := service.runtimeRunnerDecorators(nil, nil, nil, nil, false)
	for _, decorator := range decorators {
		runner := decorator(&conductorRouteRecordingRunner{}, nil)
		if _, ok := runner.(conductorInvocationRunner); ok {
			t.Fatal("ProviderOverride path attached conductorInvocationRunner")
		}
		if _, ok := runner.(registryCapabilityRunner); ok {
			t.Fatal("ProviderOverride path attached registryCapabilityRunner")
		}
	}
}

func TestConductorInvocationRunnerRejectsCapabilityEscalationBeforeProviderIO(t *testing.T) {
	t.Parallel()

	providers, integration := externalConductorRegistry(t)
	native := &conductorRouteRecordingRunner{}
	runner := conductorInvocationRunner{
		next:      native,
		conductor: conductor.New(providers),
		providers: providers,
	}

	_, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-escalate-1"},
		RunnerID: "customer.provider",
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilitySessionResume,
		},
	})
	if err == nil {
		t.Fatal("Execute(escalation) error = nil, want rejection")
	}
	var providerErr *workerprovider.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("Execute(escalation) error = %v, want *ProviderError", err)
	}
	if providerErr.Type != workers.WorkFailureTypePermanentBadRequest {
		t.Fatalf("provider error type = %q, want permanent bad request", providerErr.Type)
	}
	if !strings.Contains(providerErr.Error(), "session_resume") &&
		!strings.Contains(providerErr.Message, "session_resume") {
		t.Fatalf("provider error = %#v, want session_resume diagnostic", providerErr)
	}
	if integration.calls != 0 || native.calls != 0 {
		t.Fatalf("provider I/O occurred: integration=%d native=%d", integration.calls, native.calls)
	}
}

func TestNewRuntimeWithSelectionComposesConductorFromRegistry(t *testing.T) {
	t.Parallel()

	providers, _ := externalConductorRegistry(t)
	service := &Service{
		providerRegistry:    providers,
		invocationConductor: conductor.New(providers),
	}
	if service.invocationConductor == nil {
		t.Fatal("invocationConductor = nil")
	}
	decorators := service.runtimeRunnerDecorators(nil, nil, nil, nil, true)
	var sawConductor bool
	for _, decorator := range decorators {
		runner := decorator(&conductorRouteRecordingRunner{}, nil)
		if _, ok := runner.(conductorInvocationRunner); ok {
			sawConductor = true
		}
	}
	if !sawConductor {
		t.Fatal("runtimeRunnerDecorators omitted conductorInvocationRunner")
	}
}

func externalConductorRegistry(t *testing.T) (*providerregistry.Registry, *successfulConductorIntegration) {
	t.Helper()
	builtIns, err := providerregistry.BuiltInRegistrations()
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	manifest := externalConductorManifest(t, "customer.provider", "customer")
	integration := &successfulConductorIntegration{
		identity: inference.Identity(manifest.ID),
		maximum: inference.NewCapabilitySet(
			inference.CapabilityPromptSubmission,
		),
	}
	providers, err := providerregistry.New(append(
		builtIns,
		providerregistry.ExternalRegistration(manifest, integration),
	)...)
	if err != nil {
		t.Fatalf("registry.New() error = %v", err)
	}
	return providers, integration
}

func externalConductorManifest(t *testing.T, identity, alias string) providerregistry.Manifest {
	t.Helper()
	var catalog struct {
		Providers []providerregistry.Manifest `json:"providers"`
	}
	if err := json.Unmarshal(modelproviders.CatalogJSON(), &catalog); err != nil {
		t.Fatalf("decode embedded provider catalog: %v", err)
	}
	manifest := catalog.Providers[0]
	manifest.ID = identity
	manifest.Aliases = []string{alias}
	manifest.ImplementationAvailability = providerregistry.ImplementationExternallySupplied
	manifest.TechnicalSupportLevel = providerregistry.SupportProduction
	manifest.Deprecation = nil
	manifest.MaximumExecutionCapabilities = providerregistry.ExecutionCapabilities{
		PromptSubmission: true,
	}
	manifest.MaximumResponseFidelityCapabilities = providerregistry.ResponseFidelityCapabilities{}
	return manifest
}
