package openapitests

import (
	"encoding/json"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	. "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

func TestFactoryConfigFromOpenAPIJSON_DefaultsLegacyPetriFactoryToOrchestratorProjection(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"legacy-petri",
		"workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],
		"workers":[{"name":"worker","type":"MODEL_WORKER"}],
		"workstations":[{"name":"execute","worker":"worker","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"done"}]}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if interfaces.EffectiveOrchestratorKind(cfg) != interfaces.OrchestratorKindPetri {
		t.Fatalf("effective orchestrator kind = %q, want PETRI", interfaces.EffectiveOrchestratorKind(cfg))
	}

	public := ProjectEffectiveOrchestratorForAPIRead(FactoryConfigToOpenAPI(cfg), cfg)
	if public.Orchestrator == nil || public.Orchestrator.Kind != factoryapi.PETRI {
		t.Fatalf("public orchestrator = %#v, want projected PETRI orchestrator", public.Orchestrator)
	}

	flattened, err := NewFactoryConfigMapper().Flatten(cfg)
	if err != nil {
		t.Fatalf("Flatten: %v", err)
	}
	if stringContainsOrchestrator(flattened) {
		t.Fatalf("flattened legacy factory should omit default orchestrator block: %s", flattened)
	}
}

func TestFactoryConfigFromOpenAPIJSON_RoundTripsJavaScriptOrchestratorFactory(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"dynamic-workflow",
		"orchestrator":{
			"kind":"JAVASCRIPT",
			"javascript":{
				"sourceRef":"factory/workflows/review.js",
				"entrypoint":"main",
				"metadata":{"team":"platform"},
				"argsSchema":{"type":"object","properties":{"topic":{"type":"string"}}},
				"defaultPolicy":{"maxAgents":3},
				"agents":{"reviewer":{"preset":"careful-review"}}
			}
		}
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.Orchestrator == nil || cfg.Orchestrator.Kind != interfaces.OrchestratorKindJavaScript {
		t.Fatalf("orchestrator = %#v, want JAVASCRIPT orchestrator", cfg.Orchestrator)
	}
	if cfg.Orchestrator.JavaScript == nil || cfg.Orchestrator.JavaScript.SourceRef != "factory/workflows/review.js" {
		t.Fatalf("javascript source ref = %#v", cfg.Orchestrator.JavaScript)
	}
	if got := cfg.Orchestrator.JavaScript.Agents["reviewer"].Preset; got != "careful-review" {
		t.Fatalf("reviewer preset = %q, want careful-review", got)
	}

	public := FactoryConfigToOpenAPI(cfg)
	encoded, err := json.Marshal(public)
	if err != nil {
		t.Fatalf("marshal public factory: %v", err)
	}
	if !containsAll(encoded, `"kind":"JAVASCRIPT"`, `"sourceRef":"factory/workflows/review.js"`, `"argsSchema"`, `"defaultPolicy"`, `"agents":{"reviewer":{"preset":"careful-review"}}`) {
		t.Fatalf("public factory JSON missing JavaScript orchestrator fields: %s", encoded)
	}
}

func stringContainsOrchestrator(data []byte) bool {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return false
	}
	_, ok := root["orchestrator"]
	return ok
}

func containsAll(data []byte, values ...string) bool {
	text := string(data)
	for _, value := range values {
		if !jsonStringContains(text, value) {
			return false
		}
	}
	return true
}

func jsonStringContains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})())
}
