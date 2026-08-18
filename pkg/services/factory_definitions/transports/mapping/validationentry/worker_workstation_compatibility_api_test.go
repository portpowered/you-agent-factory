package validationentry

import (
	"encoding/json"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestWorkerWorkstationCompatibilityTargetsFromAPI_PreservesPublicAliases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		workerType      string
		workstationType string
	}{
		{name: "legacy model worker and workstation", workerType: "MODEL_WORKER", workstationType: "MODEL_WORKSTATION"},
		{name: "inference worker and legacy invoke", workerType: "INFERENCE_WORKER", workstationType: "MODEL_INVOKE"},
		{name: "model worker and agent run", workerType: "MODEL_WORKER", workstationType: "AGENT_RUN"},
		{name: "model worker and inference run", workerType: "MODEL_WORKER", workstationType: "INFERENCE_RUN"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			factory := decodeCompatibilityFactory(t, tc.workerType, tc.workstationType, "")
			taxonomy := submittedDefinitionTaxonomyFromAPI(factory)
			if len(taxonomy.Workers) != 1 || taxonomy.Workers[0].Type != tc.workerType ||
				len(taxonomy.Workstations) != 1 || taxonomy.Workstations[0].Type != tc.workstationType {
				t.Fatalf("taxonomy = %#v, want exact public values %q/%q", taxonomy, tc.workerType, tc.workstationType)
			}
		})
	}
}

func TestWorkerWorkstationCompatibilityTargetsFromAPI_ReportsPublicTaxonomyMismatch(t *testing.T) {
	t.Parallel()

	factory := decodeCompatibilityFactory(t, "AGENT_WORKER", "INFERENCE_RUN", "")
	taxonomy := submittedDefinitionTaxonomyFromAPI(factory)
	if len(taxonomy.Workers) != 1 || taxonomy.Workers[0].Type != "AGENT_WORKER" ||
		len(taxonomy.Workstations) != 1 || taxonomy.Workstations[0].Type != "INFERENCE_RUN" ||
		taxonomy.Workstations[0].Worker != "worker-a" || taxonomy.Workstations[0].Index != 0 {
		t.Fatalf("taxonomy = %#v, want lossless detached mismatch input", taxonomy)
	}
}

func TestDisplayWorkstationTypeFromAPI_ProjectsImplicitPoller(t *testing.T) {
	t.Parallel()

	factory := decodeCompatibilityFactory(t, "POLLER_WORKER", "", "POLLER")
	if factory.Workstations == nil || len(*factory.Workstations) != 1 {
		t.Fatalf("workstations = %#v, want one", factory.Workstations)
	}
	taxonomy := submittedDefinitionTaxonomyFromAPI(factory)
	if len(taxonomy.Workstations) != 1 || taxonomy.Workstations[0].Type != "" ||
		string(taxonomy.Workstations[0].Behavior) != "POLLER" {
		t.Fatalf("taxonomy = %#v, want implicit poller values copied without projection", taxonomy)
	}
}

func TestWorkerWorkstationCompatibilityTargetsFromAPI_IgnoresIncompleteReferences(t *testing.T) {
	t.Parallel()

	var factory factoryapi.Factory
	if err := json.Unmarshal([]byte(`{
		"name":"factory",
		"workers":[{"name":""},{"name":"worker-a","type":"AGENT_WORKER"}],
		"workstations":[
			{"name":"no-worker","worker":"","type":"AGENT_RUN"},
			{"name":"unknown-worker","worker":"missing","type":"AGENT_RUN"}
		]
	}`), &factory); err != nil {
		t.Fatalf("unmarshal factory: %v", err)
	}
	taxonomy := submittedDefinitionTaxonomyFromAPI(factory)
	if len(taxonomy.Workers) != 2 || len(taxonomy.Workstations) != 2 {
		t.Fatalf("taxonomy = %#v, want incomplete entries preserved for owner validation", taxonomy)
	}
}

func decodeCompatibilityFactory(t *testing.T, workerType, workstationType, behavior string) factoryapi.Factory {
	t.Helper()
	payload := map[string]any{
		"name":    "factory",
		"workers": []map[string]any{{"name": "worker-a", "type": workerType}},
		"workstations": []map[string]any{{
			"name": "process", "worker": "worker-a",
		}},
	}
	workstation := payload["workstations"].([]map[string]any)[0]
	if workstationType != "" {
		workstation["type"] = workstationType
	}
	if behavior != "" {
		workstation["behavior"] = behavior
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal factory: %v", err)
	}
	var factory factoryapi.Factory
	if err := json.Unmarshal(data, &factory); err != nil {
		t.Fatalf("unmarshal factory: %v", err)
	}
	return factory
}
