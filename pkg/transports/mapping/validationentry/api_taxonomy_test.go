package validationentry_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

func TestValidateFactoryAPI_ProfileTopology_ReturnsTaxonomyCompatibilityTargets(t *testing.T) {
	t.Parallel()

	factory := taxonomyMismatchFactoryAPI(t)
	result, err := validationentry.ValidateFactoryAPI(context.Background(), factory, factoryvalidation.Options{
		Profile: factoryvalidation.ProfileTopology,
	})
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking taxonomy compatibility targets")
	}

	target := result.BlockingTargets()[0]
	if target.Code != factoryvalidation.CodeWorkerWorkstationBehaviorCompatibility {
		t.Fatalf("target code = %q, want %q", target.Code, factoryvalidation.CodeWorkerWorkstationBehaviorCompatibility)
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
