package service

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
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
