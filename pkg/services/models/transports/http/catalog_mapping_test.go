package http

import (
	"testing"

	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestDetailToGeneratedPreservesCatalogSemantics(t *testing.T) {
	required := true
	model := "VOICE_MODEL"
	provider := "LOCAL_PROVIDER"
	operation := modelcatalog.Operation{
		Name: "TTS",
		Inputs: []modelcatalog.OperationSlot{{
			Name: "text", ContentTypes: []string{"TEXT", "SSML"}, Required: &required,
		}},
		Outputs: []modelcatalog.OperationSlot{{Name: "audio", ContentTypes: []string{"AUDIO"}}},
	}
	detail := modelcatalog.Detail{
		Summary: modelcatalog.Summary{
			Name: "VOICE_MODEL", ProviderLocality: modelcatalog.LocalityLocal,
			Status: modelcatalog.StatusReady, LoadState: modelcatalog.LoadStateUnloaded,
			Operations: []modelcatalog.Operation{operation}, Modalities: []string{"AUDIO", "TEXT"},
			Resources: []modelcatalog.ResourceSummary{{
				Name: "voice-cache", Type: "MODEL", Capacity: 1, Model: &model,
			}},
			ManagedRuntime: modelcatalog.Runtime{
				Identity: "VOICE_MODEL", Locality: modelcatalog.LocalityLocal,
				ReadinessState:      modelcatalog.ReadinessStateReady,
				LifecycleState:      modelcatalog.LifecycleStateInstalled,
				SupportedOperations: []modelcatalog.Operation{operation},
				Diagnostics:         map[string]string{"revision": "rev-1"},
			},
		},
		Capabilities: []modelcatalog.Capability{{
			Worker: "voice-worker", ProviderLocality: modelcatalog.LocalityLocal,
			ModelProvider: &provider, Operations: []modelcatalog.Operation{operation},
			ResourceNames: []string{"voice-cache"},
		}},
		Diagnostics: map[string]string{"workerCount": "1"},
	}

	got := detailToGenerated(detail)
	listed := listToGenerated(modelcatalog.List{Results: []modelcatalog.Summary{detail.Summary}})
	if len(listed.Results) != 1 || listed.Results[0].Name != detail.Name {
		t.Fatalf("list = %#v, want one mapped model summary", listed.Results)
	}
	if got.Name != detail.Name || got.Status != factoryapi.ModelStatusREADY || got.LoadState != factoryapi.UNLOADED {
		t.Fatalf("identity/status/load = (%q, %q, %q), want model-owned values", got.Name, got.Status, got.LoadState)
	}
	assertGeneratedOperations(t, got)
	assertGeneratedCatalogMetadata(t, got, provider, model)
	if got.ManagedRuntime.Diagnostics == nil || (*got.ManagedRuntime.Diagnostics)["revision"] != "rev-1" || got.Diagnostics["workerCount"] != "1" {
		t.Fatalf("diagnostics = managed:%#v detail:%#v, want both projections", got.ManagedRuntime.Diagnostics, got.Diagnostics)
	}
}

func assertGeneratedOperations(t *testing.T, got factoryapi.ModelDetail) {
	t.Helper()
	if len(got.Operations) != 1 || got.Operations[0].Inputs == nil || len(*got.Operations[0].Inputs) != 1 {
		t.Fatalf("operations = %#v, want TTS input/output contract", got.Operations)
	}
	input := (*got.Operations[0].Inputs)[0]
	if input.Required == nil || !*input.Required || len(input.ContentTypes) != 2 {
		t.Fatalf("input = %#v, want required TEXT/SSML input", input)
	}
}

func assertGeneratedCatalogMetadata(t *testing.T, got factoryapi.ModelDetail, provider, model string) {
	t.Helper()
	if len(got.Capabilities) != 1 || got.Capabilities[0].ModelProvider == nil || string(*got.Capabilities[0].ModelProvider) != provider {
		t.Fatalf("capabilities = %#v, want provider-preserving worker capability", got.Capabilities)
	}
	if len(got.Resources) != 1 || got.Resources[0].Model == nil || *got.Resources[0].Model != model {
		t.Fatalf("resources = %#v, want model-scoped cache resource", got.Resources)
	}
}
