package validationentry

import (
	"encoding/json"
	"strings"
	"testing"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
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
			if targets := workerWorkstationCompatibilityTargetsFromAPI(factory); len(targets) != 0 {
				t.Fatalf("compatibility targets = %#v, want none", targets)
			}
		})
	}
}

func TestWorkerWorkstationCompatibilityTargetsFromAPI_ReportsPublicTaxonomyMismatch(t *testing.T) {
	t.Parallel()

	factory := decodeCompatibilityFactory(t, "AGENT_WORKER", "INFERENCE_RUN", "")
	targets := workerWorkstationCompatibilityTargetsFromAPI(factory)
	if len(targets) != 1 {
		t.Fatalf("compatibility targets = %#v, want one", targets)
	}
	target := targets[0]
	if target.Code != factoryvalidation.CodeWorkerWorkstationBehaviorCompatibility ||
		target.Subject.Type != factoryvalidation.SubjectTypeWorkstation ||
		target.Subject.ID != "process" ||
		target.Subject.Location != factoryvalidation.SubjectLocationReference ||
		target.Path != "factory.workstations[0].worker" {
		t.Fatalf("compatibility target = %#v, want canonical workstation reference", target)
	}
	if !strings.Contains(target.Message, "INFERENCE_RUN") {
		t.Fatalf("message = %q, want public INFERENCE_RUN taxonomy", target.Message)
	}
}

func TestDisplayWorkstationTypeFromAPI_ProjectsImplicitPoller(t *testing.T) {
	t.Parallel()

	factory := decodeCompatibilityFactory(t, "POLLER_WORKER", "", "POLLER")
	if factory.Workstations == nil || len(*factory.Workstations) != 1 {
		t.Fatalf("workstations = %#v, want one", factory.Workstations)
	}
	if got := displayWorkstationTypeFromAPI((*factory.Workstations)[0]); got != "POLLER_RUN" {
		t.Fatalf("display workstation type = %q, want POLLER_RUN", got)
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
	if targets := workerWorkstationCompatibilityTargetsFromAPI(factory); len(targets) != 0 {
		t.Fatalf("compatibility targets = %#v, want none for incomplete references", targets)
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
