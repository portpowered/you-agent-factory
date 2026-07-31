package internal

import (
	"context"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRegistryCapabilityRunnerRejectsUnsupportedRequirement(t *testing.T) {
	next := &registryTestRunner{}
	runner := registryCapabilityRunner{
		next: next,
		providers: registryTestCatalog{metadata: workers.RunnerMetadata{
			ID: "codex",
		}},
	}

	_, err := runner.Execute(t.Context(), workers.RunnerExecutionRequest{
		RunnerID:                     "codex",
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{workers.RunnerOptionalCapabilityImageInput},
	})
	if err == nil || !strings.Contains(err.Error(), "image input is not supported") {
		t.Fatalf("Execute() error = %v, want unsupported image input", err)
	}
	if next.called {
		t.Fatal("next runner was called after capability rejection")
	}
}

func TestRegistryCapabilityRunnerDelegatesSupportedRequirement(t *testing.T) {
	next := &registryTestRunner{result: workers.RunnerExecutionResult{Content: "ok"}}
	runner := registryCapabilityRunner{
		next: next,
		providers: registryTestCatalog{metadata: workers.RunnerMetadata{
			ID: "codex",
			Capabilities: workers.RunnerCapabilities{Optional: []workers.RunnerOptionalCapabilitySupport{{
				Capability: workers.RunnerOptionalCapabilityImageInput,
				Status:     workers.RunnerOptionalCapabilityStatusSupported,
			}}},
		}},
	}

	result, err := runner.Execute(t.Context(), workers.RunnerExecutionRequest{
		RunnerID:                     "codex",
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{workers.RunnerOptionalCapabilityImageInput},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !next.called || result.Content != "ok" {
		t.Fatalf("Execute() = %#v, called = %v", result, next.called)
	}
}

type registryTestRunner struct {
	called bool
	result workers.RunnerExecutionResult
}

func (runner *registryTestRunner) Execute(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
	runner.called = true
	return runner.result, nil
}

type registryTestCatalog struct{ metadata workers.RunnerMetadata }

func (catalog registryTestCatalog) UsesNativeRunner(string) bool { return true }
func (catalog registryTestCatalog) CanonicalIdentity(identity string) (string, error) {
	return identity, nil
}
func (catalog registryTestCatalog) RunnerIdentities() []string { return []string{catalog.metadata.ID} }
func (catalog registryTestCatalog) RunnerMetadata(string) (workers.RunnerMetadata, error) {
	return catalog.metadata, nil
}
func (registryTestCatalog) ValidateRunnerPrerequisites(platformprocess.ExecutableLocator, string) error {
	return nil
}
func (catalog registryTestCatalog) ResolveRunnerSelection(workstation, factory, model string) (workers.ResolvedRunnerSelection, error) {
	return workers.ResolveRunnerSelection(workstation, factory, model), nil
}
