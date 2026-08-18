package validationentry_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	factorydefinitionfixtures "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

type programmableFactoryValidator struct {
	validateResult factorydefinitions.ValidationResult
	blockingResult factorydefinitions.ValidationResult
}

func testFactoryDefinitionValidator(
	results ...factorydefinitions.ValidationResult,
) *programmableFactoryValidator {
	validator := &programmableFactoryValidator{}
	if len(results) > 0 {
		validator.validateResult = results[0]
	}
	if len(results) > 1 {
		validator.blockingResult = results[1]
	}
	return validator
}

func (v *programmableFactoryValidator) Validate(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.WorkflowSourceReader,
) factorydefinitions.ValidationResult {
	return v.validateResult
}

func (v *programmableFactoryValidator) ValidateDefinition(
	_ context.Context,
	request factorydefinitions.DefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	if factorydefinitions.ResolveValidationProfile(request.Profile) == factorydefinitions.ValidationProfilePrePersist &&
		v.blockingResult.HasTargets() {
		return v.blockingResult, nil
	}
	return v.validateResult, nil
}

func (v *programmableFactoryValidator) ValidateSubmittedDefinition(
	_ context.Context,
	request factorydefinitions.SubmittedDefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	return v.validateResult, nil
}

func (v *programmableFactoryValidator) ValidateBlockingLoad(
	context.Context,
	*factorydefinitions.FactoryConfig,
) factorydefinitions.ValidationResult {
	return v.blockingResult
}

func (*programmableFactoryValidator) ValidateTopology(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.RequiredToolChecker,
) factorydefinitions.TopologyValidationResult {
	return factorydefinitions.TopologyValidationResult{}
}

func (*programmableFactoryValidator) WorkerWorkstationBehaviorCompatibility(
	context.Context,
	*factorydefinitions.FactoryConfig,
) []factorydefinitions.ValidationTarget {
	return nil
}

func (*programmableFactoryValidator) WorkTypeHandlingBehavior(
	context.Context,
	*factorydefinitions.FactoryConfig,
	bool,
) []factorydefinitions.ValidationTarget {
	return nil
}

func (*programmableFactoryValidator) PruneLayout(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.PendingFactoryGraphTopology,
) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{}
}

func testCanonicalFactoryLoader(
	payload []byte,
	loader factorydefinitions.WorkstationLoader,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	config, err := factorymapping.NewFactoryConfigMapper().Expand(payload)
	if err != nil {
		return nil, err
	}
	return factorydefinitionfixtures.NewLoadedSource("", config, nil, nil)

}

// invokeSubmittedDefinitionRole maps the transport payload and invokes only a
// strict programmable Factory Definition root role. Domain-policy invariants
// belong to Factory Definitions; these transport tests cover representation
// mapping and preservation of the detached role result.
func invokeSubmittedDefinitionRole(
	t *testing.T,
	factory factoryapi.Factory,
	result factorydefinitions.ValidationResult,
	representationTargets ...factorydefinitions.ValidationTarget,
) factorydefinitions.ValidationResult {
	t.Helper()
	cfg, err := factorymapping.FactoryConfigFromOpenAPI(factory)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	result.Targets = append(result.Targets, representationTargets...)
	got, err := testFactoryDefinitionValidator(result).ValidateSubmittedDefinition(
		t.Context(),
		factorydefinitions.SubmittedDefinitionValidationRequest{Config: &cfg},
	)
	if err != nil {
		t.Fatalf("ValidateSubmittedDefinition role: %v", err)
	}
	return got
}

// invokeDefinitionValidationRole exercises the profile-bearing public role
// with a transport-mapped request and a detached programmed result.
func invokeDefinitionValidationRole(
	t *testing.T,
	factory factoryapi.Factory,
	profile factorydefinitions.ValidationProfile,
	validator *programmableFactoryValidator,
) factorydefinitions.ValidationResult {
	t.Helper()
	cfg, err := factorymapping.FactoryConfigFromOpenAPI(factory)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	payload, err := json.Marshal(factory)
	if err != nil {
		t.Fatalf("marshal Factory: %v", err)
	}
	result, err := validator.ValidateDefinition(t.Context(), factorydefinitions.DefinitionValidationRequest{
		Profile:          profile,
		Config:           &cfg,
		CanonicalPayload: payload,
	})
	if err != nil {
		t.Fatalf("ValidateDefinition role: %v", err)
	}
	return result
}

func decodeEditableSnapshot(snapshot *factorydefinitions.FactorySnapshot) (factoryapi.Factory, error) {
	if snapshot == nil {
		return factoryapi.Factory{}, fmt.Errorf("Factory snapshot is required")
	}
	var factory factoryapi.Factory
	if err := snapshot.Decode(&factory); err != nil {
		return factoryapi.Factory{}, err
	}
	return factory, nil
}

func validateEditableSnapshotThroughRole(
	t *testing.T,
	snapshot *factorydefinitions.FactorySnapshot,
	validator *programmableFactoryValidator,
) error {
	t.Helper()
	factory, err := decodeEditableSnapshot(snapshot)
	if err != nil {
		return fmt.Errorf("%w: %v", apisurface.ErrInvalidNamedFactory, err)
	}
	result := invokeDefinitionValidationRole(
		t,
		factory,
		factorydefinitions.ValidationProfilePrePersist,
		validator,
	)
	if !result.HasBlockingTargets() {
		return nil
	}
	return apisurface.NewTopologyValidationError(
		"Factory topology contains invalid graph references.",
		apisurface.FactoryValidationTargetsToAPI(result.BlockingTargets()),
	)
}
