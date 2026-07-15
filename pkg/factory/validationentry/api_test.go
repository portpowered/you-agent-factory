package validationentry_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/factory/validationentry"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
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

func TestValidateFactoryAPI_ProfileTopology_RejectsInvalidInvocationSignature(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		Name: "signature-invalid",
		InvocationSignature: &interfaces.InvocationSignatureConfig{
			UnknownNamedArgumentPolicy: "COLLECT",
			Parameters: []interfaces.InvocationParameterConfig{
				{
					Name:         "items",
					ExternalName: "items",
					ValueMode:    "REPEATED",
					Bindings: []interfaces.InvocationParameterBindingConfig{
						{Kind: "NAMED"},
					},
				},
			},
			OutputContract: &interfaces.InvocationOutputContractConfig{
				Mode:          "FILE",
				PathParameter: "missing-output",
			},
		},
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "queued", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{
			Name:  "worker-a",
			Type:  interfaces.WorkerTypeInference,
			Model: "${missing}",
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "process",
			WorkerTypeName: "worker-a",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "queued"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
			OnFailure:      []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
			Body:           "Use ${items}",
		}},
	}
	factory := factoryconfig.FactoryConfigToOpenAPI(cfg)

	result, err := validationentry.ValidateFactoryAPI(context.Background(), factory, factoryvalidation.Options{
		Profile: factoryvalidation.ProfileTopology,
	})
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}

	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeInvocationSignatureUnknownOutputPathParameter)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeInvocationSignatureInvalidInterpolationReference)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeInvocationSignatureIncompatibleInterpolationReference)
}
