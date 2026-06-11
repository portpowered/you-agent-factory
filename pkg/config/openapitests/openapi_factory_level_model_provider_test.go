package openapitests

import (
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	. "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestGeneratedFactoryFromOpenAPIJSON_FactoryModelProviderRoundTripsConcreteValue(t *testing.T) {
	generated, err := GeneratedFactoryFromOpenAPIJSON(factoryJSONWithTopLevelModelProvider("GEMINI"))
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	if generated.ModelProvider == nil || *generated.ModelProvider != factoryapi.ModelProviderSelectionGemini {
		t.Fatalf("generated factory modelProvider = %#v, want GEMINI", generated.ModelProvider)
	}

	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	if got := cfg.ModelProvider; got != string(interfaces.ModelProviderGemini) {
		t.Fatalf("runtime modelProvider = %q, want %q", got, interfaces.ModelProviderGemini)
	}

	flattened, err := MarshalCanonicalFactoryConfig(&cfg)
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}
	if strings.Contains(string(flattened), `"runner"`) {
		t.Fatalf("flattened factory config must not include runner, got %s", flattened)
	}
	if !strings.Contains(string(flattened), `"modelProvider":"GEMINI"`) {
		t.Fatalf("flattened factory config = %s, want modelProvider GEMINI", flattened)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_FactoryModelProviderDefaultDefersAtRuntimeSelection(t *testing.T) {
	cfg, err := FactoryConfigFromOpenAPIJSON(factoryJSONWithTopLevelModelProvider("DEFAULT"))
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if got := cfg.ModelProvider; got != interfaces.FactoryModelProviderDefault {
		t.Fatalf("runtime modelProvider = %q, want %q", got, interfaces.FactoryModelProviderDefault)
	}

	selection := interfaces.ResolveRunnerSelection("", cfg.ModelProvider, string(interfaces.ModelProviderCodex))
	if selection.RunnerID != interfaces.RunnerIDCodex || selection.Source != interfaces.RunnerSelectionSourceLegacyProvider {
		t.Fatalf("selection = %#v, want codex from worker modelProvider", selection)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_FactoryModelProviderAbsentFallsBackToWorkerProvider(t *testing.T) {
	cfg, err := FactoryConfigFromOpenAPIJSON(factoryJSONWithTopLevelModelProvider(""))
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.ModelProvider != "" {
		t.Fatalf("runtime modelProvider = %q, want empty", cfg.ModelProvider)
	}

	selection := interfaces.ResolveRunnerSelection("", cfg.ModelProvider, string(interfaces.ModelProviderGemini))
	if selection.RunnerID != interfaces.RunnerIDGemini || selection.Source != interfaces.RunnerSelectionSourceLegacyProvider {
		t.Fatalf("selection = %#v, want gemini from worker modelProvider", selection)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsRetiredFactoryRunnerField(t *testing.T) {
	raw := factoryJSONWithTopLevelModelProvider("")
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	payload["runner"] = "gemini"
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	_, err = GeneratedFactoryFromOpenAPIJSON(encoded)
	if err == nil {
		t.Fatal("expected retired factory.runner to be rejected")
	}
	if !strings.Contains(err.Error(), "factory.runner is retired; use factory.modelProvider") {
		t.Fatalf("error = %v, want retired factory.runner guidance", err)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsUnknownFactoryModelProviderAtBoundary(t *testing.T) {
	assertGeneratedFactoryRejectsMisCasedEnumValue(
		t,
		"modelProvider",
		"MYSTERY-PROVIDER",
		string(factoryJSONWithTopLevelModelProvider("MYSTERY-PROVIDER")),
	)
}

func factoryJSONWithTopLevelModelProvider(provider string) []byte {
	providerField := ""
	if strings.TrimSpace(provider) != "" {
		providerField = `"modelProvider":` + jsonStringLiteral(provider) + `,`
	}
	return []byte(`{
		"name":"factory-level-model-provider",
		` + providerField + `
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","modelProvider":"CODEX"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"type":"MODEL_WORKSTATION",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)
}

func jsonStringLiteral(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
