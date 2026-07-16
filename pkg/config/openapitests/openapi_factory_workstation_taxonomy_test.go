package openapitests

import (
	"encoding/json"
	"strings"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

func TestFactoryConfigFromOpenAPIJSON_AcceptsNewWorkstationTaxonomyAndProjectsLegacyRuntimeTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		workstationType     string
		workerType          string
		workstationBehavior string
		wantRuntimeType     string
		wantGeneratedType   factoryapi.WorkstationType
	}{
		{
			name:              "inference run",
			workstationType:   interfaces.WorkstationTypeInference,
			workerType:        interfaces.WorkerTypeInference,
			wantRuntimeType:   interfaces.WorkstationTypeInvoke,
			wantGeneratedType: factoryapi.WorkstationTypeInferenceRun,
		},
		{
			name:              "legacy model invoke alias",
			workstationType:   interfaces.WorkstationTypeInvoke,
			workerType:        interfaces.WorkerTypeModel,
			wantRuntimeType:   interfaces.WorkstationTypeInvoke,
			wantGeneratedType: factoryapi.WorkstationTypeInferenceRun,
		},
		{
			name:              "agent run",
			workstationType:   interfaces.WorkstationTypeAgent,
			workerType:        interfaces.WorkerTypeAgent,
			wantRuntimeType:   interfaces.WorkstationTypeModel,
			wantGeneratedType: factoryapi.WorkstationTypeAgentRun,
		},
		{
			name:              "legacy model workstation alias",
			workstationType:   interfaces.WorkstationTypeModel,
			workerType:        interfaces.WorkerTypeModel,
			wantRuntimeType:   interfaces.WorkstationTypeModel,
			wantGeneratedType: factoryapi.WorkstationTypeAgentRun,
		},
		{
			name:              "script run",
			workstationType:   interfaces.WorkstationTypeScript,
			workerType:        interfaces.WorkerTypeScript,
			wantRuntimeType:   interfaces.WorkstationTypeModel,
			wantGeneratedType: factoryapi.WorkstationTypeScriptRun,
		},
		{
			name:                "poller run",
			workstationType:     interfaces.WorkstationTypePoller,
			workerType:          interfaces.WorkerTypePoller,
			workstationBehavior: "POLLER",
			wantRuntimeType:     "",
			wantGeneratedType:   factoryapi.WorkstationTypePollerRun,
		},
		{
			name:                "legacy poller workstation without explicit type",
			workstationType:     "",
			workerType:          interfaces.WorkerTypeHosted,
			workstationBehavior: "POLLER",
			wantRuntimeType:     "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfgJSON := workstationTaxonomyFactoryJSON(tt.workstationType, tt.workerType, tt.workstationBehavior)
			generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
			if err != nil {
				t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
			}
			if generated.Workstations == nil || len(*generated.Workstations) != 1 {
				t.Fatalf("generated workstations = %#v, want one workstation", generated.Workstations)
			}
			workstation := (*generated.Workstations)[0]
			if tt.wantGeneratedType != "" {
				if workstation.Type == nil || *workstation.Type != tt.wantGeneratedType {
					t.Fatalf("generated workstation type = %#v, want %s", workstation.Type, tt.wantGeneratedType)
				}
			} else if workstation.Type != nil {
				t.Fatalf("generated workstation type = %#v, want nil", workstation.Type)
			}

			cfg, err := FactoryConfigFromOpenAPI(generated)
			if err != nil {
				t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
			}
			if len(cfg.Workstations) != 1 {
				t.Fatalf("runtime workstations = %#v, want one workstation", cfg.Workstations)
			}
			if cfg.Workstations[0].Type != tt.wantRuntimeType {
				t.Fatalf("runtime workstation type = %q, want %q", cfg.Workstations[0].Type, tt.wantRuntimeType)
			}
		})
	}
}

func TestMarshalCanonicalFactoryConfig_PrefersPollerRunOnRoundTrip(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		Name: "poller-taxonomy-round-trip",
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "queued", Type: interfaces.StateTypeProcessing},
			},
		}},
		Workers: []workerconfig.Config{{
			Name:     "linear-poller",
			Type:     interfaces.WorkerTypeHosted,
			Provider: interfaces.HostedWorkerProviderLinear,
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "poll-linear",
			Kind:           interfaces.WorkstationKindPoller,
			WorkerTypeName: "linear-poller",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "queued"}},
		}},
	}

	flattened, err := NewFactoryConfigMapper().Flatten(cfg)
	if err != nil {
		t.Fatalf("Flatten: %v", err)
	}
	if !strings.Contains(string(flattened), `"type":"POLLER_RUN"`) {
		t.Fatalf("expected flattened workstation type POLLER_RUN, got %s", string(flattened))
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_PreservesExplicitAgentRunOnSaveRoundTrip(t *testing.T) {
	t.Parallel()

	cfgJSON := []byte(`{
		"name":"explicit-agent-run",
		"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers":[{"name":"executor","type":"AGENT_WORKER","model":"claude-sonnet","modelProvider":"CLAUDE"}],
		"workstations":[{"name":"execute-story","type":"AGENT_RUN","worker":"executor","inputs":[{"workType":"story","state":"init"}],"outputs":[{"workType":"story","state":"complete"}],"onFailure":[{"workType":"story","state":"failed"}]}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	workstation := (*generated.Workstations)[0]
	if workstation.Type == nil || string(*workstation.Type) != interfaces.WorkstationTypeAgent {
		t.Fatalf("generated workstation type = %#v, want %s", workstation.Type, interfaces.WorkstationTypeAgent)
	}

	runtimeCfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	canonical, err := MarshalCanonicalFactoryConfig(&runtimeCfg)
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}
	if strings.Contains(string(canonical), `"type":"MODEL_WORKSTATION"`) {
		t.Fatalf("canonical save output downgraded explicit agent run, got %s", string(canonical))
	}
	if !strings.Contains(string(canonical), `"type":"AGENT_RUN"`) {
		t.Fatalf("canonical save output missing AGENT_RUN, got %s", string(canonical))
	}

	regenerated, err := GeneratedFactoryFromOpenAPIJSON(canonical)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON round trip: %v", err)
	}
	regeneratedWorkstation := (*regenerated.Workstations)[0]
	if regeneratedWorkstation.Type == nil || string(*regeneratedWorkstation.Type) != interfaces.WorkstationTypeAgent {
		t.Fatalf("round-tripped workstation type = %#v, want %s", regeneratedWorkstation.Type, interfaces.WorkstationTypeAgent)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_PreservesExplicitInferenceRunOnSaveRoundTrip(t *testing.T) {
	t.Parallel()

	cfgJSON := []byte(`{
		"name":"explicit-inference-run",
		"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers":[{"name":"executor","type":"INFERENCE_WORKER","model":"omnivoice","modelProvider":"CLAUDE"}],
		"workstations":[{"name":"invoke-story","type":"INFERENCE_RUN","worker":"executor","operation":"TTS","inputs":[{"workType":"story","state":"init"}],"outputs":[{"workType":"story","state":"complete"}],"onFailure":[{"workType":"story","state":"failed"}]}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	workstation := (*generated.Workstations)[0]
	if workstation.Type == nil || string(*workstation.Type) != interfaces.WorkstationTypeInference {
		t.Fatalf("generated workstation type = %#v, want %s", workstation.Type, interfaces.WorkstationTypeInference)
	}

	runtimeCfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	canonical, err := MarshalCanonicalFactoryConfig(&runtimeCfg)
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}
	if strings.Contains(string(canonical), `"type":"MODEL_INVOKE"`) {
		t.Fatalf("canonical save output downgraded explicit inference run, got %s", string(canonical))
	}
	if !strings.Contains(string(canonical), `"type":"INFERENCE_RUN"`) {
		t.Fatalf("canonical save output missing INFERENCE_RUN, got %s", string(canonical))
	}

	regenerated, err := GeneratedFactoryFromOpenAPIJSON(canonical)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON round trip: %v", err)
	}
	regeneratedWorkstation := (*regenerated.Workstations)[0]
	if regeneratedWorkstation.Type == nil || string(*regeneratedWorkstation.Type) != interfaces.WorkstationTypeInference {
		t.Fatalf("round-tripped workstation type = %#v, want %s", regeneratedWorkstation.Type, interfaces.WorkstationTypeInference)
	}
}

func TestMarshalCanonicalFactoryConfig_PrefersNewWorkstationTaxonomyOnRoundTrip(t *testing.T) {
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
		Workers: []workerconfig.Config{{
			Name:          "executor",
			Type:          interfaces.WorkerTypeModel,
			Model:         "omnivoice",
			ModelProvider: string(modelprovider.Claude),
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "execute-story",
			Type:           interfaces.WorkstationTypeInvoke,
			WorkerTypeName: "executor",
			Operation:      "TTS",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
		}},
	}

	flattened, err := NewFactoryConfigMapper().Flatten(cfg)
	if err != nil {
		t.Fatalf("Flatten: %v", err)
	}
	if !strings.Contains(string(flattened), `"type":"INFERENCE_RUN"`) {
		t.Fatalf("expected flattened workstation type INFERENCE_RUN, got %s", string(flattened))
	}

	expanded, err := NewFactoryConfigMapper().Expand(flattened)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if expanded.Workstations[0].Type != interfaces.WorkstationTypeInvoke {
		t.Fatalf("expanded runtime workstation type = %q, want %q", expanded.Workstations[0].Type, interfaces.WorkstationTypeInvoke)
	}
}

func workstationTaxonomyFactoryJSON(workstationType, workerType, workstationBehavior string) []byte {
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
	}
	if strings.TrimSpace(workstationBehavior) != "" {
		workstation["behavior"] = workstationBehavior
	}

	payload := map[string]any{
		"name": "workstation-taxonomy-factory",
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
