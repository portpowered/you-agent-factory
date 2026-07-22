package openapitests

import (
	"encoding/json"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	. "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

func TestFactoryConfigFromOpenAPIJSON_AcceptsNewWorkerTaxonomyAndProjectsLegacyRuntimeTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		workerType        string
		workstationType   string
		wantRuntimeType   string
		wantGeneratedType factoryapi.WorkerType
	}{
		{
			name:              "inference worker",
			workerType:        interfaces.WorkerTypeInference,
			workstationType:   interfaces.WorkstationTypeInference,
			wantRuntimeType:   interfaces.WorkerTypeModel,
			wantGeneratedType: factoryapi.WorkerTypeInferenceWorker,
		},
		{
			name:              "legacy model worker alias",
			workerType:        interfaces.WorkerTypeModel,
			workstationType:   interfaces.WorkstationTypeModel,
			wantRuntimeType:   interfaces.WorkerTypeAgent,
			wantGeneratedType: factoryapi.WorkerTypeAgentWorker,
		},
		{
			name:              "agent worker",
			workerType:        interfaces.WorkerTypeAgent,
			workstationType:   interfaces.WorkstationTypeAgent,
			wantRuntimeType:   interfaces.WorkerTypeAgent,
			wantGeneratedType: factoryapi.WorkerTypeAgentWorker,
		},
		{
			name:              "script worker",
			workerType:        interfaces.WorkerTypeScript,
			workstationType:   interfaces.WorkstationTypeScript,
			wantRuntimeType:   interfaces.WorkerTypeScript,
			wantGeneratedType: factoryapi.WorkerTypeScriptWorker,
		},
		{
			name:              "poller worker",
			workerType:        interfaces.WorkerTypePoller,
			workstationType:   interfaces.WorkstationTypePoller,
			wantRuntimeType:   interfaces.WorkerTypeHosted,
			wantGeneratedType: factoryapi.WorkerTypePollerWorker,
		},
		{
			name:              "legacy hosted worker alias",
			workerType:        interfaces.WorkerTypeHosted,
			workstationType:   interfaces.WorkstationTypePoller,
			wantRuntimeType:   interfaces.WorkerTypeHosted,
			wantGeneratedType: factoryapi.WorkerTypePollerWorker,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfgJSON := workerTaxonomyFactoryJSON(tt.workerType, tt.workstationType)
			generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
			if err != nil {
				t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
			}
			if generated.Workers == nil || len(*generated.Workers) != 1 {
				t.Fatalf("generated workers = %#v, want one worker", generated.Workers)
			}
			worker := (*generated.Workers)[0]
			if worker.Type == nil || *worker.Type != tt.wantGeneratedType {
				t.Fatalf("generated worker type = %#v, want %s", worker.Type, tt.wantGeneratedType)
			}

			cfg, err := FactoryConfigFromOpenAPI(generated)
			if err != nil {
				t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
			}
			if len(cfg.Workers) != 1 {
				t.Fatalf("runtime workers = %#v, want one worker", cfg.Workers)
			}
			if cfg.Workers[0].Type != tt.wantRuntimeType {
				t.Fatalf("runtime worker type = %q, want %q", cfg.Workers[0].Type, tt.wantRuntimeType)
			}
		})
	}
}

func TestMarshalCanonicalFactoryConfig_PrefersNewWorkerTaxonomyOnRoundTrip(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		Name: "taxonomy-round-trip",
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{{
			Name:          "executor",
			Type:          interfaces.WorkerTypeModel,
			Model:         "omnivoice",
			ModelProvider: string(modelprovider.ProviderClaude),
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "execute-story",
			Type:           interfaces.WorkstationTypeInvoke,
			WorkerTypeName: "executor",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
		}},
	}

	flattened, err := NewFactoryConfigMapper().Flatten(cfg)
	if err != nil {
		t.Fatalf("Flatten: %v", err)
	}
	if !strings.Contains(string(flattened), `"type":"INFERENCE_WORKER"`) {
		t.Fatalf("expected flattened worker type INFERENCE_WORKER, got %s", string(flattened))
	}

	expanded, err := NewFactoryConfigMapper().Expand(flattened)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if expanded.Workers[0].Type != interfaces.WorkerTypeModel {
		t.Fatalf("expanded runtime worker type = %q, want %q", expanded.Workers[0].Type, interfaces.WorkerTypeModel)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_PreservesExplicitAgentWorkerOnSaveRoundTrip(t *testing.T) {
	t.Parallel()

	cfgJSON := []byte(`{
		"name":"explicit-agent-worker",
		"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers":[{"name":"executor","type":"AGENT_WORKER","model":"claude-sonnet","modelProvider":"CLAUDE"}],
		"workstations":[{"name":"execute-story","type":"AGENT_RUN","worker":"executor","inputs":[{"workType":"story","state":"init"}],"outputs":[{"workType":"story","state":"complete"}],"onFailure":[{"workType":"story","state":"failed"}]}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	worker := (*generated.Workers)[0]
	if worker.Type == nil || string(*worker.Type) != interfaces.WorkerTypeAgent {
		t.Fatalf("generated worker type = %#v, want %s", worker.Type, interfaces.WorkerTypeAgent)
	}

	runtimeCfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	canonical, err := MarshalCanonicalFactoryConfig(&runtimeCfg)
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}
	if strings.Contains(string(canonical), `"type":"MODEL_WORKER"`) {
		t.Fatalf("canonical save output downgraded explicit agent worker, got %s", string(canonical))
	}
	if !strings.Contains(string(canonical), `"type":"AGENT_WORKER"`) {
		t.Fatalf("canonical save output missing AGENT_WORKER, got %s", string(canonical))
	}

	regenerated, err := GeneratedFactoryFromOpenAPIJSON(canonical)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON round trip: %v", err)
	}
	if regenerated.Workers == nil || len(*regenerated.Workers) != 1 {
		t.Fatalf("regenerated workers = %#v, want one worker", regenerated.Workers)
	}
	regeneratedWorker := (*regenerated.Workers)[0]
	if regeneratedWorker.Type == nil || string(*regeneratedWorker.Type) != interfaces.WorkerTypeAgent {
		t.Fatalf("round-tripped worker type = %#v, want %s", regeneratedWorker.Type, interfaces.WorkerTypeAgent)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_PreservesAgentWorkerToolPolicyOnSaveRoundTrip(t *testing.T) {
	t.Parallel()

	cfgJSON := []byte(`{
		"name":"agent-tool-policy",
		"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers":[{"name":"executor","type":"AGENT_WORKER","model":"claude-sonnet","modelProvider":"CLAUDE","agentTools":{"policy":"READ_ONLY"}}],
		"workstations":[{"name":"execute-story","type":"AGENT_RUN","worker":"executor","inputs":[{"workType":"story","state":"init"}],"outputs":[{"workType":"story","state":"complete"}],"onFailure":[{"workType":"story","state":"failed"}]}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	worker := (*generated.Workers)[0]
	if worker.AgentTools == nil || worker.AgentTools.Policy != factoryapi.AgentWorkerToolPolicyREADONLY {
		t.Fatalf("generated agent tool policy = %#v, want READ_ONLY", worker.AgentTools)
	}

	runtimeCfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	if runtimeCfg.Workers[0].AgentTools == nil || runtimeCfg.Workers[0].AgentTools.Policy != interfaces.AgentToolPolicyReadOnly {
		t.Fatalf("runtime agent tool policy = %#v, want READ_ONLY", runtimeCfg.Workers[0].AgentTools)
	}

	canonical, err := MarshalCanonicalFactoryConfig(&runtimeCfg)
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}
	if !strings.Contains(string(canonical), `"agentTools":{"policy":"READ_ONLY"}`) {
		t.Fatalf("canonical save output missing agentTools policy, got %s", string(canonical))
	}

	regenerated, err := GeneratedFactoryFromOpenAPIJSON(canonical)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON round trip: %v", err)
	}
	if regenerated.Workers == nil || (*regenerated.Workers)[0].AgentTools == nil ||
		(*regenerated.Workers)[0].AgentTools.Policy != factoryapi.AgentWorkerToolPolicyREADONLY {
		t.Fatalf("regenerated agent tool policy = %#v, want READ_ONLY", (*regenerated.Workers)[0].AgentTools)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_PreservesExplicitInferenceWorkerOnSaveRoundTrip(t *testing.T) {
	t.Parallel()

	cfgJSON := []byte(`{
		"name":"explicit-inference-worker",
		"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers":[{"name":"executor","type":"INFERENCE_WORKER","model":"omnivoice","modelProvider":"CLAUDE"}],
		"workstations":[{"name":"invoke-story","type":"INFERENCE_RUN","worker":"executor","operation":"TTS","inputs":[{"workType":"story","state":"init"}],"outputs":[{"workType":"story","state":"complete"}],"onFailure":[{"workType":"story","state":"failed"}]}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	worker := (*generated.Workers)[0]
	if worker.Type == nil || string(*worker.Type) != interfaces.WorkerTypeInference {
		t.Fatalf("generated worker type = %#v, want %s", worker.Type, interfaces.WorkerTypeInference)
	}

	runtimeCfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	canonical, err := MarshalCanonicalFactoryConfig(&runtimeCfg)
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}
	if strings.Contains(string(canonical), `"type":"MODEL_WORKER"`) {
		t.Fatalf("canonical save output downgraded explicit inference worker, got %s", string(canonical))
	}
	if !strings.Contains(string(canonical), `"type":"INFERENCE_WORKER"`) {
		t.Fatalf("canonical save output missing INFERENCE_WORKER, got %s", string(canonical))
	}

	regenerated, err := GeneratedFactoryFromOpenAPIJSON(canonical)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON round trip: %v", err)
	}
	regeneratedWorker := (*regenerated.Workers)[0]
	if regeneratedWorker.Type == nil || string(*regeneratedWorker.Type) != interfaces.WorkerTypeInference {
		t.Fatalf("round-tripped worker type = %#v, want %s", regeneratedWorker.Type, interfaces.WorkerTypeInference)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_PreservesMixedLegacyModelWorkerOnSaveRoundTrip(t *testing.T) {
	t.Parallel()

	cfgJSON := []byte(`{
		"name":"mixed-legacy-model-worker",
		"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers":[{"name":"executor","type":"MODEL_WORKER","model":"claude-sonnet","modelProvider":"CLAUDE"}],
		"workstations":[
			{"name":"execute-story","type":"MODEL_WORKSTATION","worker":"executor","inputs":[{"workType":"story","state":"init"}],"outputs":[{"workType":"story","state":"complete"}],"onFailure":[{"workType":"story","state":"failed"}]},
			{"name":"invoke-story","type":"MODEL_INVOKE","worker":"executor","inputs":[{"workType":"story","state":"init"}],"outputs":[{"workType":"story","state":"complete"}],"onFailure":[{"workType":"story","state":"failed"}]}
		]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	if generated.Workers == nil || len(*generated.Workers) != 1 {
		t.Fatalf("generated workers = %#v, want one worker", generated.Workers)
	}
	worker := (*generated.Workers)[0]
	if worker.Type == nil || string(*worker.Type) != interfaces.WorkerTypeModel {
		t.Fatalf("generated worker type = %#v, want %s", worker.Type, interfaces.WorkerTypeModel)
	}

	runtimeCfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	canonical, err := MarshalCanonicalFactoryConfig(&runtimeCfg)
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}
	if !strings.Contains(string(canonical), `"type":"MODEL_WORKER"`) {
		t.Fatalf("canonical save output downgraded mixed legacy worker, got %s", string(canonical))
	}

	regenerated, err := GeneratedFactoryFromOpenAPIJSON(canonical)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON round trip: %v", err)
	}
	regeneratedWorker := (*regenerated.Workers)[0]
	if regeneratedWorker.Type == nil || string(*regeneratedWorker.Type) != interfaces.WorkerTypeModel {
		t.Fatalf("round-tripped worker type = %#v, want %s", regeneratedWorker.Type, interfaces.WorkerTypeModel)
	}
}

func workerTaxonomyFactoryJSON(workerType, workstationType string) []byte {
	workstation := map[string]any{
		"name":   "execute-story",
		"worker": "executor",
		"inputs": []map[string]string{{"workType": "story", "state": "init"}},
		"outputs": []map[string]string{{
			"workType": "story",
			"state":    "complete",
		}},
	}
	if strings.TrimSpace(workstationType) != "" {
		workstation["type"] = workstationType
		if workstationType == interfaces.WorkstationTypePoller {
			workstation["behavior"] = "POLLER"
		}
	}
	payload := map[string]any{
		"name": "worker-taxonomy-factory",
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]any{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]any{{
			"name": "executor",
			"type": workerType,
		}},
		"workstations": []map[string]any{workstation},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return data
}
