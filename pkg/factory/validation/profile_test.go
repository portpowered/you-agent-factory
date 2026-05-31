package validation_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
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

	apiResult := result.FactoryValidationResult()
	if len(apiResult.Targets) != 1 {
		t.Fatalf("api targets = %d, want 1", len(apiResult.Targets))
	}
	if apiResult.Targets[0].Code != factoryvalidation.CodeDuplicateIdentifier {
		t.Fatalf("api target code = %q", apiResult.Targets[0].Code)
	}

	message, targets := result.TopologyValidationErrorInput("custom message")
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

func TestOptions_ResolvedProfileDefaultsToTopology(t *testing.T) {
	t.Parallel()

	if got := (factoryvalidation.Options{}).ResolvedProfile(); got != factoryvalidation.ProfileTopology {
		t.Fatalf("ResolvedProfile() = %q, want %q", got, factoryvalidation.ProfileTopology)
	}
}
