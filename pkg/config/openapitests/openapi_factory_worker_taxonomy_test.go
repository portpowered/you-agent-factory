package openapitests

import (
	"testing"

	. "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestGeneratedFactoryFromOpenAPIJSON_AcceptsRuntimeTaxonomyWorkerTypes(t *testing.T) {
	tests := []struct {
		name       string
		workerType string
		wantType   string
		extra      string
	}{
		{
			name:       "inference worker",
			workerType: "INFERENCE_WORKER",
			wantType:   interfaces.WorkerTypeInference,
			extra:      `"modelProvider":"CLAUDE","stopToken":"COMPLETE"`,
		},
		{
			name:       "agent worker",
			workerType: "AGENT_WORKER",
			wantType:   interfaces.WorkerTypeAgent,
			extra:      `"modelProvider":"CLAUDE","stopToken":"COMPLETE"`,
		},
		{
			name:       "script worker",
			workerType: "SCRIPT_WORKER",
			wantType:   interfaces.WorkerTypeScript,
			extra:      `"command":"go","args":["run","./worker"]`,
		},
		{
			name:       "poller worker",
			workerType: "POLLER_WORKER",
			wantType:   interfaces.WorkerTypePoller,
			extra:      `"provider":"LINEAR","auth":{"secretRef":"secrets/linear-api-key"},"linear":{"pollInterval":"45s","teamIds":["team-a"],"stateIds":["state-b"],"mapping":{"workType":"story","state":"init"}}`,
		},
		{
			name:       "legacy model worker",
			workerType: "MODEL_WORKER",
			wantType:   interfaces.WorkerTypeModel,
			extra:      `"modelProvider":"CLAUDE","stopToken":"COMPLETE"`,
		},
		{
			name:       "legacy hosted worker",
			workerType: "HOSTED_WORKER",
			wantType:   interfaces.WorkerTypeHosted,
			extra:      `"provider":"LINEAR","auth":{"secretRef":"secrets/linear-api-key"},"linear":{"pollInterval":"45s","teamIds":["team-a"],"stateIds":["state-b"],"mapping":{"workType":"story","state":"init"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfgJSON := []byte(`{
				"name":"taxonomy-worker-factory",
				"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers":[{"name":"worker-a","type":"` + tt.workerType + `",` + tt.extra + `}],
				"workstations":[{"name":"execute-story","worker":"worker-a","inputs":[{"workType":"story","state":"init"}],"outputs":[{"workType":"story","state":"complete"}]}]
			}`)

			generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
			if err != nil {
				t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
			}
			if generated.Workers == nil || len(*generated.Workers) != 1 {
				t.Fatalf("generated workers = %#v", generated.Workers)
			}
			worker := (*generated.Workers)[0]
			if worker.Type == nil || string(*worker.Type) != tt.workerType {
				t.Fatalf("generated worker type = %#v, want %q", worker.Type, tt.workerType)
			}

			cfg, err := FactoryConfigFromOpenAPI(generated)
			if err != nil {
				t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
			}
			if len(cfg.Workers) != 1 {
				t.Fatalf("runtime workers = %#v", cfg.Workers)
			}
			if cfg.Workers[0].Type != tt.wantType {
				t.Fatalf("runtime worker type = %q, want %q", cfg.Workers[0].Type, tt.wantType)
			}
		})
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_ProjectsLegacyModelWorkerToInferenceBehavior(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"legacy-model-projection-factory",
		"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers":[{"name":"executor","type":"MODEL_WORKER","modelProvider":"CLAUDE","operations":[{"name":"TTS","outputs":[{"name":"audio","contentTypes":["AUDIO"]}]}]}],
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
	if got := interfaces.ProjectWorkerBehaviorClass(cfg.Workers[0].Type); got != interfaces.WorkerTypeInference {
		t.Fatalf("legacy MODEL_WORKER behavior class = %q, want %q", got, interfaces.WorkerTypeInference)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_AcceptsPollerWorkerWithHostedLinearConfig(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"poller-worker-factory",
		"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"queued","type":"PROCESSING"}]}],
		"workers":[{
			"name":"linear-poller",
			"type":"POLLER_WORKER",
			"provider":"LINEAR",
			"auth":{"secretRef":"secrets/linear-api-key"},
			"linear":{
				"pollInterval":"45s",
				"teamIds":["team-a"],
				"stateIds":["state-b"],
				"mapping":{"workType":"story","state":"init"}
			}
		}],
		"workstations":[{
			"name":"poll-linear",
			"behavior":"POLLER",
			"worker":"linear-poller",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"queued"}]
		}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	if cfg.Workers[0].Type != interfaces.WorkerTypePoller {
		t.Fatalf("runtime worker type = %q, want %q", cfg.Workers[0].Type, interfaces.WorkerTypePoller)
	}
	if got := interfaces.ProjectWorkerBehaviorClass(cfg.Workers[0].Type); got != interfaces.WorkerTypePoller {
		t.Fatalf("poller worker behavior class = %q, want %q", got, interfaces.WorkerTypePoller)
	}
	if got := interfaces.ProjectWorkerBehaviorClass(interfaces.WorkerTypeHosted); got != interfaces.WorkerTypePoller {
		t.Fatalf("legacy HOSTED_WORKER behavior class = %q, want %q", got, interfaces.WorkerTypePoller)
	}
}
