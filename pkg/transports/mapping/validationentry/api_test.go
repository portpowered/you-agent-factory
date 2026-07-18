package validationentry_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

func TestValidateEditableFactorySnapshot_PreservesPublicValidationError(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}
	snapshot, err := interfaces.NewFactorySnapshot(factory)
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}

	err = validationentry.ValidateEditableFactorySnapshot(snapshot, nil)
	if !errors.Is(err, apisurface.ErrInvalidNamedFactory) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactory", err)
	}
	var topologyErr *apisurface.TopologyValidationError
	if !errors.As(err, &topologyErr) || len(topologyErr.Targets) == 0 {
		t.Fatalf("error = %#v, want topology targets", err)
	}
}

func TestValidateEditableFactorySnapshot_PreservesLayoutBoundaryPath(t *testing.T) {
	t.Parallel()

	var snapshot interfaces.FactorySnapshot
	err := snapshot.UnmarshalJSON([]byte(`{
		"name":"layout-invalid",
		"layout":{"schemaVersion":1,"annotations":[{
			"id":"note-1","kind":"NOTE","position":{"x":0,"y":0},"size":{"width":0,"height":80},
			"note":{"body":"literal","tone":"NEUTRAL"}
		}]}
	}`))
	if err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	err = validationentry.ValidateEditableFactorySnapshot(&snapshot, nil)
	var topologyErr *apisurface.TopologyValidationError
	if !errors.As(err, &topologyErr) || len(topologyErr.Targets) != 1 {
		t.Fatalf("error = %#v, want one structured layout target", err)
	}
	target := topologyErr.Targets[0]
	if target.Code != factoryvalidation.CodeLayoutInvalidGeometry || target.Path == nil || *target.Path != "factory.layout.annotations[0].size.width" {
		t.Fatalf("target = %#v, want field-specific invalid geometry", target)
	}
}

func TestValidateEditableFactorySnapshot_ValidAndMissing(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	snapshot, err := interfaces.NewFactorySnapshot(factory)
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	if err := validationentry.ValidateEditableFactorySnapshot(snapshot, nil); err != nil {
		t.Fatalf("ValidateEditableFactorySnapshot(valid): %v", err)
	}
	if err := validationentry.ValidateEditableFactorySnapshot(nil, nil); !errors.Is(err, apisurface.ErrInvalidNamedFactory) {
		t.Fatalf("ValidateEditableFactorySnapshot(nil) error = %v, want ErrInvalidNamedFactory", err)
	}
}

func TestValidateFactoryAPI_ProfileTopology_CrossPathInvalidFixture(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathInvalidFactory()
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

	apiResult := apisurface.FactoryValidationResultToAPI(result)
	if len(apiResult.Targets) != len(result.Targets) {
		t.Fatalf("api targets = %d, canonical targets = %d", len(apiResult.Targets), len(result.Targets))
	}
	validationassert.HasTargetCode(t, apiResult.Targets, factoryvalidation.CodeDuplicateIdentifier)
}

func TestValidateFactoryAPI_ProfilePrePersist_CrossPathInvalidFixture(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathInvalidFactory()
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

	message, targets := apisurface.FactoryTopologyValidationErrorInput(result, "")
	topologyErr := apisurface.NewTopologyValidationError(message, targets)
	if len(topologyErr.Targets) != len(result.Targets) {
		t.Fatalf("topology error targets = %d, canonical targets = %d", len(topologyErr.Targets), len(result.Targets))
	}
}

func TestValidateFactoryAPI_ProfileTopology_ValidFactory_NoTargets(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
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

	factory, err := factoryfixtures.DecodeCrossPathInvalidFactory()
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

	validationResult := apisurface.FactoryValidationResultToAPI(apiResult)
	handlerAPI := apisurface.FactoryValidationResultToAPI(handlerEquivalent)
	if !factoryvalidation.EquivalentCanonicalTargetSignatures(
		validationassert.CanonicalAPITargetSignatures(handlerAPI.Targets),
		validationassert.CanonicalAPITargetSignatures(validationResult.Targets),
	) {
		t.Fatalf("api result targets = %#v, handler api targets = %#v",
			validationResult.Targets, handlerAPI.Targets)
	}
}

func TestValidateFactoryAPI_ProfilePrePersist_ValidFactory_NoTargets(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
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
		Workers: []workerconfig.Config{{
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
