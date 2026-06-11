package openapitests

import (
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	. "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestGeneratedFactoryFromOpenAPIJSON_WorkstationModelProviderRoundTripsConcreteValue(t *testing.T) {
	generated, err := GeneratedFactoryFromOpenAPIJSON(factoryJSONWithWorkstationModelProvider("GEMINI", "", "CODEX"))
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	if generated.Workstations == nil || len(*generated.Workstations) != 1 {
		t.Fatalf("generated workstations = %#v, want one workstation", generated.Workstations)
	}
	workstation := (*generated.Workstations)[0]
	if workstation.ModelProvider == nil || *workstation.ModelProvider != factoryapi.ModelProviderSelectionGemini {
		t.Fatalf("generated workstation modelProvider = %#v, want GEMINI", workstation.ModelProvider)
	}

	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	if got := cfg.Workstations[0].ModelProvider; got != string(interfaces.ModelProviderGemini) {
		t.Fatalf("runtime workstation modelProvider = %q, want %q", got, interfaces.ModelProviderGemini)
	}

	flattened, err := MarshalCanonicalFactoryConfig(&cfg)
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}
	if strings.Contains(string(flattened), `"runner"`) {
		t.Fatalf("flattened factory config must not include runner, got %s", flattened)
	}
	if !strings.Contains(string(flattened), `"modelProvider":"GEMINI"`) {
		t.Fatalf("flattened factory config = %s, want workstation modelProvider GEMINI", flattened)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_WorkstationModelProviderDefaultDefersToFactory(t *testing.T) {
	cfg, err := FactoryConfigFromOpenAPIJSON(factoryJSONWithWorkstationModelProvider("DEFAULT", "CODEX", "CLAUDE"))
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if got := cfg.Workstations[0].ModelProvider; got != interfaces.FactoryModelProviderDefault {
		t.Fatalf("runtime workstation modelProvider = %q, want %q", got, interfaces.FactoryModelProviderDefault)
	}

	selection := interfaces.ResolveRunnerSelection(cfg.Workstations[0].ModelProvider, cfg.ModelProvider, string(interfaces.ModelProviderClaude))
	if selection.RunnerID != interfaces.RunnerIDCodex || selection.Source != interfaces.RunnerSelectionSourceFactory {
		t.Fatalf("selection = %#v, want codex from factory modelProvider", selection)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_WorkstationModelProviderConcreteOverridesFactoryWorkerAndDefault(t *testing.T) {
	cfg, err := FactoryConfigFromOpenAPIJSON(factoryJSONWithWorkstationModelProvider("GEMINI", "CODEX", "CLAUDE"))
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	selection := interfaces.ResolveRunnerSelection(cfg.Workstations[0].ModelProvider, cfg.ModelProvider, string(interfaces.ModelProviderClaude))
	if selection.RunnerID != interfaces.RunnerIDGemini || selection.Source != interfaces.RunnerSelectionSourceWorkstation {
		t.Fatalf("selection = %#v, want gemini from workstation modelProvider", selection)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_WorkstationModelProviderAbsentFallsBackThroughFactoryWorkerDefault(t *testing.T) {
	cfg, err := FactoryConfigFromOpenAPIJSON(factoryJSONWithWorkstationModelProvider("", "DEFAULT", "CODEX"))
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.Workstations[0].ModelProvider != "" {
		t.Fatalf("runtime workstation modelProvider = %q, want empty", cfg.Workstations[0].ModelProvider)
	}

	selection := interfaces.ResolveRunnerSelection(cfg.Workstations[0].ModelProvider, cfg.ModelProvider, string(interfaces.ModelProviderCodex))
	if selection.RunnerID != interfaces.RunnerIDCodex || selection.Source != interfaces.RunnerSelectionSourceLegacyProvider {
		t.Fatalf("selection = %#v, want codex from worker modelProvider", selection)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsRetiredWorkstationRunnerField(t *testing.T) {
	raw := factoryJSONWithWorkstationModelProvider("", "", "CODEX")
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	workstations := payload["workstations"].([]any)
	workstation := workstations[0].(map[string]any)
	workstation["runner"] = "gemini"
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	_, err = GeneratedFactoryFromOpenAPIJSON(encoded)
	if err == nil {
		t.Fatal("expected retired workstation.runner to be rejected")
	}
	if !strings.Contains(err.Error(), "workstations[0].runner is retired; use workstations[0].modelProvider") {
		t.Fatalf("error = %v, want retired workstation.runner guidance", err)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsUnknownWorkstationModelProviderAtBoundary(t *testing.T) {
	assertGeneratedFactoryRejectsMisCasedEnumValue(
		t,
		"workstations[0].modelProvider",
		"MYSTERY-PROVIDER",
		string(factoryJSONWithWorkstationModelProvider("MYSTERY-PROVIDER", "", "CODEX")),
	)
}

func factoryJSONWithWorkstationModelProvider(workstationProvider, factoryProvider, workerProvider string) []byte {
	workstationProviderField := ""
	if strings.TrimSpace(workstationProvider) != "" {
		workstationProviderField = `"modelProvider":` + jsonStringLiteral(workstationProvider) + `,`
	}
	factoryProviderField := ""
	if strings.TrimSpace(factoryProvider) != "" {
		factoryProviderField = `"modelProvider":` + jsonStringLiteral(factoryProvider) + `,`
	}
	return []byte(`{
		"name":"workstation-level-model-provider",
		` + factoryProviderField + `
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","modelProvider":` + jsonStringLiteral(workerProvider) + `}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"type":"MODEL_WORKSTATION",
			` + workstationProviderField + `
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)
}
