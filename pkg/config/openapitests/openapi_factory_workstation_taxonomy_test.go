package openapitests

import (
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	. "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestGeneratedFactoryFromOpenAPIJSON_AcceptsRuntimeTaxonomyWorkstationTypes(t *testing.T) {
	for _, tt := range runtimeTaxonomyWorkstationTypeCases() {
		t.Run(tt.name, func(t *testing.T) {
			assertRuntimeTaxonomyWorkstationTypeLoad(t, tt)
		})
	}
}

type runtimeTaxonomyWorkstationTypeCase struct {
	name            string
	workstationType string
	workerType      string
	behavior        string
	operation       string
	workerExtra     string
	wantType        string
}

func runtimeTaxonomyWorkstationTypeCases() []runtimeTaxonomyWorkstationTypeCase {
	return []runtimeTaxonomyWorkstationTypeCase{
		{
			name:            "inference run",
			workstationType: "INFERENCE_RUN",
			workerType:      "INFERENCE_WORKER",
			operation:       "TTS",
			workerExtra:     `"modelProvider":"CLAUDE","operations":[{"name":"TTS","inputs":[{"name":"text","contentTypes":["TEXT"]}],"outputs":[{"name":"audio","contentTypes":["AUDIO"]}]}]`,
			wantType:        interfaces.WorkstationTypeInference,
		},
		{
			name:            "legacy model invoke",
			workstationType: "MODEL_INVOKE",
			workerType:      "MODEL_WORKER",
			operation:       "TTS",
			workerExtra:     `"modelProvider":"CLAUDE","operations":[{"name":"TTS","inputs":[{"name":"text","contentTypes":["TEXT"]}],"outputs":[{"name":"audio","contentTypes":["AUDIO"]}]}]`,
			wantType:        interfaces.WorkstationTypeInvoke,
		},
		{
			name:            "agent run",
			workstationType: "AGENT_RUN",
			workerType:      "AGENT_WORKER",
			workerExtra:     `"modelProvider":"CLAUDE","stopToken":"COMPLETE"`,
			wantType:        interfaces.WorkstationTypeAgent,
		},
		{
			name:            "legacy model workstation",
			workstationType: "MODEL_WORKSTATION",
			workerType:      "MODEL_WORKER",
			workerExtra:     `"modelProvider":"CLAUDE","stopToken":"COMPLETE"`,
			wantType:        interfaces.WorkstationTypeModel,
		},
		{
			name:            "script run",
			workstationType: "SCRIPT_RUN",
			workerType:      "SCRIPT_WORKER",
			workerExtra:     `"command":"go","args":["run","./worker"]`,
			wantType:        interfaces.WorkstationTypeScript,
		},
		{
			name:            "poller run",
			workstationType: "POLLER_RUN",
			workerType:      "POLLER_WORKER",
			behavior:        "POLLER",
			workerExtra:     `"provider":"LINEAR","auth":{"secretRef":"secrets/linear-api-key"},"linear":{"pollInterval":"45s","teamIds":["team-a"],"stateIds":["state-b"],"mapping":{"workType":"story","state":"init"}}`,
			wantType:        interfaces.WorkstationTypePoller,
		},
	}
}

func assertRuntimeTaxonomyWorkstationTypeLoad(t *testing.T, tt runtimeTaxonomyWorkstationTypeCase) {
	t.Helper()

	workstation := map[string]any{
		"name":   "execute-story",
		"worker": "worker-a",
		"inputs": []map[string]string{{"workType": "story", "state": "init"}},
		"outputs": []map[string]string{{
			"workType": "story",
			"state":    "complete",
		}},
		"type": tt.workstationType,
	}
	if tt.operation != "" {
		workstation["operation"] = tt.operation
	}
	if tt.behavior != "" {
		workstation["behavior"] = tt.behavior
	}

	worker := map[string]any{
		"name": "worker-a",
		"type": tt.workerType,
	}
	for key, value := range decodeWorkstationTaxonomyJSONMap(`{` + tt.workerExtra + `}`) {
		worker[key] = value
	}

	factory := map[string]any{
		"name": "taxonomy-workstation-factory",
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers":      []map[string]any{worker},
		"workstations": []map[string]any{workstation},
	}

	cfgJSON, err := json.Marshal(factory)
	if err != nil {
		t.Fatalf("marshal factory: %v", err)
	}

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	if generated.Workstations == nil || len(*generated.Workstations) != 1 {
		t.Fatalf("generated workstations = %#v", generated.Workstations)
	}
	workstationType := (*generated.Workstations)[0].Type
	if workstationType == nil || string(*workstationType) != tt.workstationType {
		t.Fatalf("generated workstation type = %#v, want %q", workstationType, tt.workstationType)
	}

	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	if len(cfg.Workstations) != 1 {
		t.Fatalf("runtime workstations = %#v", cfg.Workstations)
	}
	if cfg.Workstations[0].Type != tt.wantType {
		t.Fatalf("runtime workstation type = %q, want %q", cfg.Workstations[0].Type, tt.wantType)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_ProjectsLegacyModelInvokeToInferenceRunBehavior(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"legacy-model-invoke-projection-factory",
		"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers":[{"name":"executor","type":"MODEL_WORKER","modelProvider":"CLAUDE","operations":[{"name":"TTS","inputs":[{"name":"text","contentTypes":["TEXT"]}],"outputs":[{"name":"audio","contentTypes":["AUDIO"]}]}]}],
		"workstations":[{"name":"tts","type":"MODEL_INVOKE","operation":"TTS","worker":"executor","inputs":[{"workType":"story","state":"init"}],"outputs":[{"workType":"story","state":"complete"}]}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	if got := interfaces.ProjectWorkstationBehaviorClass(cfg.Workstations[0].Type, cfg.Workstations[0].Kind); got != interfaces.WorkstationTypeInference {
		t.Fatalf("legacy MODEL_INVOKE behavior class = %q, want %q", got, interfaces.WorkstationTypeInference)
	}
}

func TestMarshalCanonicalFactoryConfig_PreservesNewAndLegacyWorkstationTaxonomyOnRoundTrip(t *testing.T) {
	tests := []struct {
		name         string
		workstation  interfaces.FactoryWorkstationConfig
		wantContains string
	}{
		{
			name: "new inference run",
			workstation: interfaces.FactoryWorkstationConfig{
				Name:           "execute-story",
				Type:           interfaces.WorkstationTypeInference,
				WorkerTypeName: "executor",
				Operation:      "TTS",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
			},
			wantContains: `"type":"INFERENCE_RUN"`,
		},
		{
			name: "legacy model invoke",
			workstation: interfaces.FactoryWorkstationConfig{
				Name:           "execute-story",
				Type:           interfaces.WorkstationTypeInvoke,
				WorkerTypeName: "executor",
				Operation:      "TTS",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
			},
			wantContains: `"type":"MODEL_INVOKE"`,
		},
		{
			name: "poller run",
			workstation: interfaces.FactoryWorkstationConfig{
				Name:           "poll-linear",
				Type:           interfaces.WorkstationTypePoller,
				Kind:           interfaces.WorkstationKindPoller,
				WorkerTypeName: "linear-poller",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "queued"}},
			},
			wantContains: `"type":"POLLER_RUN"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &interfaces.FactoryConfig{
				Name: "taxonomy-round-trip",
				WorkTypes: []interfaces.WorkTypeConfig{{
					Name: "story",
					States: []interfaces.StateConfig{
						{Name: "init", Type: interfaces.StateTypeInitial},
						{Name: "queued", Type: interfaces.StateTypeProcessing},
						{Name: "complete", Type: interfaces.StateTypeTerminal},
					},
				}},
				Workers: []interfaces.WorkerConfig{{
					Name:          "executor",
					Type:          interfaces.WorkerTypeInference,
					ModelProvider: string(interfaces.ModelProviderClaude),
				}, {
					Name:     "linear-poller",
					Type:     interfaces.WorkerTypeHosted,
					Provider: interfaces.HostedWorkerProviderLinear,
				}},
				Workstations: []interfaces.FactoryWorkstationConfig{tt.workstation},
			}

			flattened, err := NewFactoryConfigMapper().Flatten(cfg)
			if err != nil {
				t.Fatalf("Flatten: %v", err)
			}
			if !strings.Contains(string(flattened), tt.wantContains) {
				t.Fatalf("expected flattened payload to contain %s, got %s", tt.wantContains, string(flattened))
			}

			expanded, err := NewFactoryConfigMapper().Expand(flattened)
			if err != nil {
				t.Fatalf("Expand: %v", err)
			}
			if expanded.Workstations[0].Type != tt.workstation.Type {
				t.Fatalf("expanded runtime workstation type = %q, want %q", expanded.Workstations[0].Type, tt.workstation.Type)
			}
		})
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_AcceptsLegacyPollerWorkstationWithoutExplicitType(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"legacy-poller-factory",
		"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"queued","type":"PROCESSING"}]}],
		"workers":[{"name":"linear-poller","type":"HOSTED_WORKER","provider":"LINEAR","auth":{"secretRef":"secrets/linear-api-key"},"linear":{"pollInterval":"45s","teamIds":["team-a"],"stateIds":["state-b"],"mapping":{"workType":"story","state":"init"}}}],
		"workstations":[{"name":"poll-linear","behavior":"POLLER","worker":"linear-poller","inputs":[{"workType":"story","state":"init"}],"outputs":[{"workType":"story","state":"queued"}]}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	if cfg.Workstations[0].Kind != interfaces.WorkstationKindPoller {
		t.Fatalf("runtime workstation kind = %q, want %q", cfg.Workstations[0].Kind, interfaces.WorkstationKindPoller)
	}
	if got := interfaces.IsPollerRunWorkstationType(cfg.Workstations[0].Type, cfg.Workstations[0].Kind); !got {
		t.Fatalf("legacy poller workstation should project to poller-run behavior")
	}
	if got := interfaces.ProjectWorkstationBehaviorClass(cfg.Workstations[0].Type, cfg.Workstations[0].Kind); got != interfaces.WorkstationTypePoller {
		t.Fatalf("legacy poller behavior class = %q, want %q", got, interfaces.WorkstationTypePoller)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_DerivesPollerKindForPollerRunWithoutExplicitBehavior(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"poller-run-without-behavior-factory",
		"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"queued","type":"PROCESSING"}]}],
		"workers":[{"name":"script-poller","type":"SCRIPT_WORKER","command":"factory/scripts/poll.sh"}],
		"workstations":[{"name":"poll-tasks","type":"POLLER_RUN","worker":"script-poller","inputs":[{"workType":"story","state":"init"}],"outputs":[{"workType":"story","state":"queued"}]}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	if cfg.Workstations[0].Kind != interfaces.WorkstationKindPoller {
		t.Fatalf("runtime workstation kind = %q, want %q", cfg.Workstations[0].Kind, interfaces.WorkstationKindPoller)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_AcceptsInferenceRunWithGeneratedEnum(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"generated-inference-run-factory",
		"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers":[{"name":"executor","type":"INFERENCE_WORKER","modelProvider":"CLAUDE","operations":[{"name":"TTS","inputs":[{"name":"text","contentTypes":["TEXT"]}],"outputs":[{"name":"audio","contentTypes":["AUDIO"]}]}]}],
		"workstations":[{"name":"tts","type":"INFERENCE_RUN","operation":"TTS","worker":"executor","inputs":[{"workType":"story","state":"init"}],"outputs":[{"workType":"story","state":"complete"}]}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	if generated.Workstations == nil || len(*generated.Workstations) != 1 {
		t.Fatalf("generated workstations = %#v", generated.Workstations)
	}
	if got := (*generated.Workstations)[0].Type; got == nil || *got != factoryapi.WorkstationTypeInferenceRun {
		t.Fatalf("generated workstation type = %#v, want %q", got, factoryapi.WorkstationTypeInferenceRun)
	}
}

func decodeWorkstationTaxonomyJSONMap(payload string) map[string]any {
	var out map[string]any
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		panic(err)
	}
	return out
}
