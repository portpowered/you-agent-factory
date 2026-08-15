package internal

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRegistryCapabilityRunnerRejectsUnsupportedRequirement(t *testing.T) {
	next := &registryTestRunner{}
	runner := registryCapabilityRunner{
		next: next,
		providers: registryTestCatalog{metadata: workers.RunnerMetadata{
			ID: "test-provider",
		}},
	}

	_, err := runner.Execute(t.Context(), workers.RunnerExecutionRequest{
		RunnerID:                     "test-provider",
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
			ID: "test-provider",
			Capabilities: workers.RunnerCapabilities{Optional: []workers.RunnerOptionalCapabilitySupport{{
				Capability: workers.RunnerOptionalCapabilityImageInput,
				Status:     workers.RunnerOptionalCapabilityStatusSupported,
			}}},
		}},
	}

	result, err := runner.Execute(t.Context(), workers.RunnerExecutionRequest{
		RunnerID:                     "test-provider",
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

type registryTestCatalog struct {
	testutil.ProviderServiceAdapter
	metadata workers.RunnerMetadata
}

func (catalog registryTestCatalog) GetProvider(_ context.Context, request providers.GetProviderRequest) (providers.GetProviderResult, error) {
	if err := request.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	return providers.GetProviderResult{
		Provider: providers.Descriptor{
			ID:           providers.ID(catalog.metadata.ID),
			Capabilities: providerCapabilities(catalog.metadata),
		},
	}, nil
}

func providerCapabilities(metadata workers.RunnerMetadata) []providers.Capability {
	capabilities := make([]providers.Capability, 0, len(metadata.Capabilities.Optional))
	for _, optional := range metadata.Capabilities.Optional {
		if optional.Status != workers.RunnerOptionalCapabilityStatusSupported {
			continue
		}
		switch optional.Capability {
		case workers.RunnerOptionalCapabilityImageInput:
			capabilities = append(capabilities, providers.CapabilityImageInput)
		case workers.RunnerOptionalCapabilitySessionResume:
			capabilities = append(capabilities, providers.CapabilitySessionResume)
		case workers.RunnerOptionalCapabilityStructuredOutput:
			capabilities = append(capabilities, providers.CapabilityStructuredOutput)
		}
	}
	return capabilities
}
