package provider

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestWorkDiagnosticsForInferenceRequest_IncludesOpenCodeAgentWhenConfigured(t *testing.T) {
	t.Parallel()

	diagnostics := workDiagnosticsForInferenceRequest(interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderOpenCode),
		Model:         "openai/gpt-5",
		OpenCodeAgent: "implementer",
		WorkerType:    interfaces.WorkerTypeModel,
	})

	if got := diagnostics.Provider.RequestMetadata["opencode_agent"]; got != "implementer" {
		t.Fatalf("opencode_agent = %q, want implementer", got)
	}
}

func TestWorkDiagnosticsForInferenceRequest_OmitsOpenCodeAgentWhenUnset(t *testing.T) {
	t.Parallel()

	diagnostics := workDiagnosticsForInferenceRequest(interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderOpenCode),
		Model:         "openai/gpt-5",
		WorkerType:    interfaces.WorkerTypeModel,
	})

	if _, ok := diagnostics.Provider.RequestMetadata["opencode_agent"]; ok {
		t.Fatalf("request metadata = %#v, want opencode_agent omitted", diagnostics.Provider.RequestMetadata)
	}
}

func TestWorkDiagnosticsForInferenceRequest_SafeProjectionPreservesOpenCodeAgent(t *testing.T) {
	t.Parallel()

	diagnostics := workDiagnosticsForInferenceRequest(interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderOpenCode),
		OpenCodeAgent: "implementer",
	})
	safe := interfaces.SafeWorkDiagnosticsFromWorkDiagnostics(diagnostics)

	if got := safe.Provider.RequestMetadata["opencode_agent"]; got != "implementer" {
		t.Fatalf("safe opencode_agent = %q, want implementer", got)
	}
}
