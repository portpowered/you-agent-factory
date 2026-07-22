package profiletests

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestResult_FactoryValidationResultAndTopologyValidationErrorInput(t *testing.T) {
	t.Parallel()

	result := factoryvalidation.Result{
		Targets: []factoryvalidation.Target{{
			Code:     factoryvalidation.CodeDuplicateIdentifier,
			Severity: factoryvalidation.SeverityError,
			Message:  "duplicate worker",
			Subject: factoryvalidation.Subject{
				Type:     factoryvalidation.SubjectTypeWorker,
				ID:       "worker-a",
				Location: factoryvalidation.SubjectLocationDefinition,
			},
		}},
	}

	apiResult := apisurface.FactoryValidationResultToAPI(result)
	if len(apiResult.Targets) != 1 {
		t.Fatalf("api targets = %d, want 1", len(apiResult.Targets))
	}
	if apiResult.Targets[0].Code != factoryvalidation.CodeDuplicateIdentifier {
		t.Fatalf("api target code = %q", apiResult.Targets[0].Code)
	}

	message, targets := apisurface.FactoryTopologyValidationErrorInput(result, "custom message")
	if message != "custom message" {
		t.Fatalf("topology message = %q, want custom message", message)
	}
	if targets[0].Code != factoryvalidation.CodeDuplicateIdentifier {
		t.Fatalf("topology target code = %q", targets[0].Code)
	}
	topologyErr := apisurface.NewTopologyValidationError(message, targets)
	if topologyErr.Message != "custom message" {
		t.Fatalf("built topology message = %q", topologyErr.Message)
	}
}

func TestResolveValidationProfile_DefaultsToTopology(t *testing.T) {
	t.Parallel()

	if got := factorydefinitions.ResolveValidationProfile(""); got != factorydefinitions.ValidationProfileTopology {
		t.Fatalf("ResolveValidationProfile() = %q, want %q", got, factorydefinitions.ValidationProfileTopology)
	}
}
