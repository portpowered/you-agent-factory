package validationentry_test

import (
	"encoding/json"
	"strings"
	"testing"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestValidateFactoryAPI_ProfileTopology_ReturnsTaxonomyCompatibilityTargets(t *testing.T) {
	t.Parallel()

	factory := taxonomyMismatchFactoryAPI(t)
	compatibilityTarget := factoryvalidation.ValidationTarget{
		Code:     factoryvalidation.ValidationCodeWorkerWorkstationBehaviorCompatibility,
		Severity: factoryvalidation.ValidationSeverityError,
		Message:  "agent-run is incompatible with INFERENCE_WORKER",
		Subject: factoryvalidation.ValidationSubject{
			Type:     factoryvalidation.ValidationSubjectTypeWorkstation,
			ID:       "agent-with-infer",
			Location: factoryvalidation.ValidationSubjectLocationDefinition,
		},
	}
	result := invokeSubmittedDefinitionRole(
		t,
		factory,
		factoryvalidation.ValidationResult{},
		compatibilityTarget,
	)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking taxonomy compatibility targets")
	}

	target := result.BlockingTargets()[0]
	if target.Code != factoryvalidation.ValidationCodeWorkerWorkstationBehaviorCompatibility {
		t.Fatalf("target code = %q, want %q", target.Code, factoryvalidation.ValidationCodeWorkerWorkstationBehaviorCompatibility)
	}
	if !strings.Contains(target.Message, "agent-run") || !strings.Contains(target.Message, "INFERENCE_WORKER") {
		t.Fatalf("target message = %q, want agent-run and INFERENCE_WORKER terminology", target.Message)
	}
}

func taxonomyMismatchFactoryAPI(t *testing.T) factoryapi.Factory {
	t.Helper()

	var factory factoryapi.Factory
	if err := json.Unmarshal([]byte(taxonomyMismatchFactoryJSON), &factory); err != nil {
		t.Fatalf("Unmarshal taxonomy factory: %v", err)
	}
	return factory
}

const taxonomyMismatchFactoryJSON = `{
  "name": "taxonomy-cli-api",
  "workTypes": [{
    "name": "task",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "done", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{
    "name": "infer",
    "type": "INFERENCE_WORKER",
    "operations": [{
      "name": "TTS",
      "inputs": [{"name": "text", "contentTypes": ["TEXT"]}],
      "outputs": [{"name": "audio", "contentTypes": ["AUDIO"]}]
    }]
  }],
  "workstations": [{
    "name": "agent-with-infer",
    "type": "AGENT_RUN",
    "worker": "infer",
    "inputs": [{"workType": "task", "state": "init"}],
    "outputs": [{"workType": "task", "state": "done"}]
  }]
}`
