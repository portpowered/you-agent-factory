package validationentry_test

import (
	"context"
	"encoding/json"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/factory/validationentry"
	"github.com/portpowered/infinite-you/pkg/testutil/validationassert"
)

func TestValidateFactoryAPI_ProfileTopology_CrossPathInvalidFixture(t *testing.T) {
	t.Parallel()

	factory, err := factoryvalidation.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}

	result, err := validationentry.ValidateFactoryAPI(context.Background(), factory, factoryvalidation.Options{
		Profile: factoryvalidation.ProfileTopology,
	})
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}
	if !result.HasTargets() {
		t.Fatal("expected topology profile targets for cross-path invalid fixture")
	}

	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeDuplicateIdentifier)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeDanglingWorkerReference)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeDanglingPlaceReference)

	apiResult := result.FactoryValidationResult()
	if len(apiResult.Targets) != len(result.Targets) {
		t.Fatalf("api targets = %d, canonical targets = %d", len(apiResult.Targets), len(result.Targets))
	}
	validationassert.HasTargetCode(t, apiResult.Targets, factoryvalidation.CodeDuplicateIdentifier)
}

func TestValidateFactoryAPI_ProfilePrePersist_CrossPathInvalidFixture(t *testing.T) {
	t.Parallel()

	factory, err := factoryvalidation.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}

	result, err := validationentry.ValidateFactoryAPI(context.Background(), factory, factoryvalidation.Options{
		Profile: factoryvalidation.ProfilePrePersist,
	})
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}
	if !result.HasTargets() {
		t.Fatal("expected pre-persist profile targets for cross-path invalid fixture")
	}
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeWorkstationMissingFailureRoute ||
			target.Code == factoryvalidation.CodeWorkstationMissingRejectionRoute {
			t.Fatalf("unexpected deferred outcome-route target %#v", target)
		}
	}
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeDuplicateIdentifier)

	message, targets := result.TopologyValidationErrorInput("")
	topologyErr := apisurface.NewTopologyValidationError(message, targets)
	if len(topologyErr.Targets) != len(result.Targets) {
		t.Fatalf("topology error targets = %d, canonical targets = %d", len(topologyErr.Targets), len(result.Targets))
	}
}

func TestValidateFactoryAPI_ProfileTopology_ValidFactory_NoTargets(t *testing.T) {
	t.Parallel()

	factory, err := factoryvalidation.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}

	result, err := validationentry.ValidateFactoryAPI(context.Background(), factory, factoryvalidation.Options{
		Profile: factoryvalidation.ProfileTopology,
	})
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}
	if result.HasTargets() {
		t.Fatalf("valid factory targets = %#v, want none", result.Targets)
	}
}

func TestValidateFactoryAPI_ProfileTopology_MatchesValidateEndpointPath(t *testing.T) {
	t.Parallel()

	factory, err := factoryvalidation.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}

	apiResult, err := validationentry.ValidateFactoryAPI(context.Background(), factory, factoryvalidation.Options{
		Profile: factoryvalidation.ProfileTopology,
	})
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}

	cfg, err := factoryconfig.FactoryConfigFromOpenAPI(factory)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	handlerEquivalent := factoryvalidation.Validate(&cfg)

	apiSignatures := factoryvalidation.CanonicalTargetSignatures(apiResult.Targets)
	handlerSignatures := factoryvalidation.CanonicalTargetSignatures(handlerEquivalent.Targets)
	if !factoryvalidation.EquivalentCanonicalTargetSignatures(handlerSignatures, apiSignatures) {
		t.Fatalf("ValidateFactoryAPI signatures = %#v, handler path signatures = %#v",
			apiSignatures, handlerSignatures)
	}

	validationResult := apiResult.FactoryValidationResult()
	handlerAPI := handlerEquivalent.FactoryValidationResult()
	if !factoryvalidation.EquivalentCanonicalTargetSignatures(
		factoryvalidation.CanonicalAPITargetSignatures(handlerAPI.Targets),
		factoryvalidation.CanonicalAPITargetSignatures(validationResult.Targets),
	) {
		t.Fatalf("api result targets = %#v, handler api targets = %#v",
			validationResult.Targets, handlerAPI.Targets)
	}
}

func TestValidateFactoryAPI_ProfilePrePersist_MatchesEditableSavePreCheck(t *testing.T) {
	t.Parallel()

	factory, err := factoryvalidation.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}

	apiResult, err := validationentry.ValidateFactoryAPI(context.Background(), factory, factoryvalidation.Options{
		Profile: factoryvalidation.ProfilePrePersist,
	})
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}

	legacyResult := legacyEditableSaveValidation(t, factory)
	apiSignatures := factoryvalidation.CanonicalTargetSignatures(apiResult.Targets)
	legacySignatures := factoryvalidation.CanonicalTargetSignatures(legacyResult.Targets)
	if !factoryvalidation.EquivalentCanonicalTargetSignatures(legacySignatures, apiSignatures) {
		t.Fatalf("ValidateFactoryAPI signatures = %#v, editable save signatures = %#v",
			apiSignatures, legacySignatures)
	}
}

func TestValidateFactoryAPI_ProfilePrePersist_ValidFactory_NoTargets(t *testing.T) {
	t.Parallel()

	factory, err := factoryvalidation.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}

	result, err := validationentry.ValidateFactoryAPI(context.Background(), factory, factoryvalidation.Options{
		Profile: factoryvalidation.ProfilePrePersist,
	})
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}
	if result.HasTargets() {
		t.Fatalf("valid factory targets = %#v, want none", result.Targets)
	}
}

// legacyEditableSaveValidation mirrors factorysave.validateEditableFactoryTopology
// until story 004 wires save to ValidateFactoryAPI.
func legacyEditableSaveValidation(t *testing.T, submitted factoryapi.Factory) factoryvalidation.Result {
	t.Helper()

	payload, err := json.Marshal(submitted)
	if err != nil {
		t.Fatalf("marshal factory: %v", err)
	}
	_, loadErr := configload.LoadFromCanonicalJSON(payload, configload.LoadOptions{})
	cfg, mapErr := factoryconfig.FactoryConfigFromOpenAPI(submitted)
	if mapErr != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", mapErr)
	}
	if loadErr != nil {
		if configload.IsInvalidNamedFactory(loadErr) {
			blocking := factoryvalidation.ValidateBlockingLoad(&cfg)
			if len(blocking.Targets) > 0 {
				return blocking
			}
		}
		t.Fatalf("LoadFromCanonicalJSON: %v", loadErr)
	}
	return factoryvalidation.Validate(&cfg)
}
