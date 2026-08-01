package taxonomyvalidationtests_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestValidateFactory_ReturnsTaxonomyCompatibilityTargets(t *testing.T) {
	t.Parallel()

	srv := newAPITestServer(taxonomyCompatibilityValidator{})
	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-validations",
		bytes.NewBufferString(taxonomyMismatchFactoryValidationBody),
	)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /factory-validations status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var result factoryapi.FactoryValidationResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(result.Targets) == 0 {
		t.Fatalf("targets = %#v, want taxonomy compatibility target", result.Targets)
	}
	target := result.Targets[0]
	if target.Code != "workstation-worker-behavior-compatibility" {
		t.Fatalf("target code = %q, want workstation-worker-behavior-compatibility", target.Code)
	}
	if !strings.Contains(target.Message, "agent-run") || !strings.Contains(target.Message, "INFERENCE_WORKER") {
		t.Fatalf("target message = %q, want agent-run and INFERENCE_WORKER terminology", target.Message)
	}
}

func newAPITestServer(validator factorydefinitions.SubmittedDefinitionValidationOperation) *api.Server {
	logger := zap.NewNop()
	handler := factorysessionshttp.NewHandler(factorysessionshttp.Dependencies{FactoryValidation: validator}, logger)
	return api.NewServer(handler, nil, nil, logger, nil)
}

// taxonomyCompatibilityValidator scripts the service-root finding whose HTTP
// representation is under test. Factory Definitions owner tests retain the
// worker/workstation taxonomy policy.
type taxonomyCompatibilityValidator struct{}

func (validator taxonomyCompatibilityValidator) ValidateSubmittedDefinition(
	ctx context.Context,
	request factorydefinitions.SubmittedDefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	result := validator.Validate(ctx, request.Config, request.WorkflowSourceReader)
	return result, nil
}

func (taxonomyCompatibilityValidator) Validate(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.WorkflowSourceReader,
) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{
		Targets: []factorydefinitions.ValidationTarget{{
			Code:     "workstation-worker-behavior-compatibility",
			Severity: factorydefinitions.ValidationSeverityError,
			Message:  "agent-run requires a compatible worker; INFERENCE_WORKER is not supported",
			Subject: factorydefinitions.ValidationSubject{
				Type: factorydefinitions.ValidationSubjectTypeWorkstation,
				ID:   "agent-with-infer",
			},
		}},
	}
}

func (taxonomyCompatibilityValidator) ValidateBlockingLoad(
	context.Context,
	*factorydefinitions.FactoryConfig,
) factorydefinitions.ValidationResult {
	panic("unexpected Factory Definition ValidateBlockingLoad call")
}

func (taxonomyCompatibilityValidator) ValidateTopology(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.RequiredToolChecker,
) factorydefinitions.TopologyValidationResult {
	panic("unexpected Factory Definition ValidateTopology call")
}

func (taxonomyCompatibilityValidator) WorkerWorkstationBehaviorCompatibility(
	context.Context,
	*factorydefinitions.FactoryConfig,
) []factorydefinitions.ValidationTarget {
	return nil
}

func (taxonomyCompatibilityValidator) WorkTypeHandlingBehavior(
	context.Context,
	*factorydefinitions.FactoryConfig,
	bool,
) []factorydefinitions.ValidationTarget {
	panic("unexpected Factory Definition WorkTypeHandlingBehavior call")
}

func (taxonomyCompatibilityValidator) PruneLayout(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.PendingFactoryGraphTopology,
) factorydefinitions.ValidationResult {
	panic("unexpected Factory Definition PruneLayout call")
}

const taxonomyMismatchFactoryValidationBody = `{
  "name": "taxonomy-api",
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
