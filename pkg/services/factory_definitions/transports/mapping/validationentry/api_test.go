package validationentry_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/validationentry"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

func TestPersistenceEntryMapsDetachedCanonicalRequest(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"name":"alpha","workers":[{"name":"executor","type":"MODEL_WORKER"}],"workstations":[{"name":"execute","worker":"executor","type":"AGENT_RUN"}]}`)
	snapshot := interfaces.FactorySnapshot(append([]byte(nil), payload...))
	request, err := validationentry.MapFactoryJSONForPersistence(payload)
	if err != nil {
		t.Fatal(err)
	}
	editable, err := validationentry.MapEditableFactorySnapshot(&snapshot, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range []interfaces.DefinitionValidationRequest{request, editable} {
		if got.Config == nil || got.Config.Name != "alpha" || len(got.SubmittedTaxonomy.Workers) != 1 || got.SubmittedTaxonomy.Workers[0].Type != "MODEL_WORKER" || len(got.SubmittedTaxonomy.Workstations) != 1 || got.SubmittedTaxonomy.Workstations[0].Worker != "executor" {
			t.Fatalf("mapped request = %#v", got)
		}
		assertCanonicalPayload(t, got.CanonicalPayload, payload)
	}
	payload[0] = 'x'
	snapshot[0] = 'x'
	if request.CanonicalPayload[0] != '{' || editable.CanonicalPayload[0] != '{' {
		t.Fatal("mapped canonical payload aliases submitted bytes")
	}
	request.Config.Name = "changed"
	if editable.Config.Name != "alpha" {
		t.Fatal("mapped config aliases another request")
	}
}

func assertCanonicalPayload(t *testing.T, canonicalPayload, submittedPayload []byte) {
	t.Helper()
	var canonical, submitted any
	if err := json.Unmarshal(canonicalPayload, &canonical); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(submittedPayload, &submitted); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(canonical, submitted) {
		t.Fatalf("canonical payload = %s, want %s", canonicalPayload, submittedPayload)
	}
}

func TestSubmittedValidationEntryPreservesOperationResult(t *testing.T) {
	t.Parallel()
	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatal(err)
	}
	want := validationResult("controlled.finding")
	got, err := validationentry.ValidateFactoryAPI(t.Context(), factory, testFactoryDefinitionValidator(want))
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("validation = %#v, %v; want %#v", got, err, want)
	}
	if _, err := validationentry.ValidateFactoryAPI(t.Context(), factory, nil); err == nil || err.Error() != "Factory Definition validation operation is required" {
		t.Fatalf("missing operation = %v", err)
	}
}

func TestPersistenceEntriesClassifyMalformedAndMissingPayloads(t *testing.T) {
	t.Parallel()
	malformed := interfaces.FactorySnapshot(`{"name":`)
	for _, snapshot := range []*interfaces.FactorySnapshot{nil, &malformed} {
		got, err := validationentry.MapEditableFactorySnapshot(snapshot, nil)
		if !errors.Is(err, interfaces.ErrInvalidNamedFactory) || got.Config != nil {
			t.Fatalf("editable mapping = %#v, %v; want invalid Factory and no request", got, err)
		}
	}
	got, err := validationentry.MapFactoryJSONForPersistence([]byte(malformed))
	if !errors.Is(err, interfaces.ErrInvalidNamedFactory) || got.Config != nil {
		t.Fatalf("persistence mapping = %#v, %v; want invalid Factory and no request", got, err)
	}
}

const (
	codeDuplicateIdentifier                              = "factory.duplicateIdentifier"
	codeDanglingWorkerReference                          = "factory.worker.danglingReference"
	codeDanglingPlaceReference                           = "factory.route.danglingPlaceReference"
	codeWorkstationMissingFailureRoute                   = "factory.workstation.missingFailureRoute"
	codeWorkstationMissingRejectionRoute                 = "factory.workstation.missingRejectionRoute"
	codeInvocationSignatureUnknownOutputPathParameter    = "factory.invocationSignature.unknownOutputPathParameter"
	codeInvocationSignatureInvalidInterpolationReference = "factory.invocationSignature.invalidInterpolationReference"
	codeInvocationSignatureIncompatibleInterpolation     = "factory.invocationSignature.incompatibleInterpolationReference"
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

	findings := validationResult(codeDuplicateIdentifier)
	err = validateEditableSnapshotThroughRole(t, snapshot, testFactoryDefinitionValidator(interfaces.ValidationResult{}, findings))
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
	if target.Code != interfaces.ValidationCodeLayoutInvalidGeometry || target.Path == nil || *target.Path != "factory.layout.annotations[0].size.width" {
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
	if err := validateEditableSnapshotThroughRole(t, snapshot, testFactoryDefinitionValidator()); err != nil {
		t.Fatalf("ValidateEditableFactorySnapshot(valid): %v", err)
	}
	if err := validateEditableSnapshotThroughRole(t, nil, testFactoryDefinitionValidator()); !errors.Is(err, apisurface.ErrInvalidNamedFactory) {
		t.Fatalf("ValidateEditableFactorySnapshot(nil) error = %v, want ErrInvalidNamedFactory", err)
	}
}

func TestValidateEditableFactorySnapshot_RejectsUndecodableSnapshot(t *testing.T) {
	t.Parallel()

	snapshot := interfaces.FactorySnapshot(`{"name":`)
	err := validationentry.ValidateEditableFactorySnapshot(&snapshot, nil)
	if !errors.Is(err, apisurface.ErrInvalidNamedFactory) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactory", err)
	}
}

func TestValidateFactoryAPI_ProfileTopology_CrossPathInvalidFixture(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}

	findings := validationResult(codeDuplicateIdentifier, codeDanglingWorkerReference, codeDanglingPlaceReference)
	result := invokeSubmittedDefinitionRole(t, factory, findings)
	if !result.HasTargets() {
		t.Fatal("expected topology profile targets for cross-path invalid fixture")
	}

	validationassert.HasDomainTargetCode(t, result.Targets, codeDuplicateIdentifier)
	validationassert.HasDomainTargetCode(t, result.Targets, codeDanglingWorkerReference)
	validationassert.HasDomainTargetCode(t, result.Targets, codeDanglingPlaceReference)

	apiResult := apisurface.FactoryValidationResultToAPI(result)
	if len(apiResult.Targets) != len(result.Targets) {
		t.Fatalf("api targets = %d, canonical targets = %d", len(apiResult.Targets), len(result.Targets))
	}
	validationassert.HasTargetCode(t, apiResult.Targets, codeDuplicateIdentifier)
}

func TestValidateFactoryAPI_ProfilePrePersist_CrossPathInvalidFixture(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}

	findings := validationResult(codeDuplicateIdentifier)
	result := invokeDefinitionValidationRole(
		t,
		factory,
		interfaces.ValidationProfilePrePersist,
		testFactoryDefinitionValidator(interfaces.ValidationResult{}, findings),
	)
	if !result.HasTargets() {
		t.Fatal("expected pre-persist profile targets for cross-path invalid fixture")
	}
	for _, target := range result.Targets {
		if target.Code == codeWorkstationMissingFailureRoute ||
			target.Code == codeWorkstationMissingRejectionRoute {
			t.Fatalf("unexpected deferred outcome-route target %#v", target)
		}
	}
	validationassert.HasDomainTargetCode(t, result.Targets, codeDuplicateIdentifier)

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

	result := invokeSubmittedDefinitionRole(t, factory, interfaces.ValidationResult{})
	if result.HasTargets() {
		t.Fatalf("valid factory targets = %#v, want none", result.Targets)
	}
}

func TestValidateFactoryAPI_ProfileTopology_PreservesInjectedEndpointFindings(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}

	findings := validationResult(codeDuplicateIdentifier, codeDanglingWorkerReference)
	apiResult := invokeSubmittedDefinitionRole(t, factory, findings)

	if len(apiResult.Targets) != len(findings.Targets) {
		t.Fatalf("ValidateFactoryAPI targets = %#v, want injected findings %#v", apiResult.Targets, findings.Targets)
	}

	validationResult := apisurface.FactoryValidationResultToAPI(apiResult)
	if len(validationResult.Targets) != len(findings.Targets) {
		t.Fatalf("API targets = %#v, want all injected findings", validationResult.Targets)
	}
}

func TestValidateFactoryAPI_ProfilePrePersist_ValidFactory_NoTargets(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}

	result := invokeDefinitionValidationRole(
		t,
		factory,
		interfaces.ValidationProfilePrePersist,
		testFactoryDefinitionValidator(),
	)
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
		Workers: []interfaces.FactoryWorkerConfig{{
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
	factory, err := factorymapping.FactoryConfigToOpenAPI(cfg)
	if err != nil {
		t.Fatalf("FactoryConfigToOpenAPI: %v", err)
	}

	findings := validationResult(
		codeInvocationSignatureUnknownOutputPathParameter,
		codeInvocationSignatureInvalidInterpolationReference,
		codeInvocationSignatureIncompatibleInterpolation,
	)
	result := invokeSubmittedDefinitionRole(t, factory, findings)

	validationassert.HasDomainTargetCode(t, result.Targets, codeInvocationSignatureUnknownOutputPathParameter)
	validationassert.HasDomainTargetCode(t, result.Targets, codeInvocationSignatureInvalidInterpolationReference)
	validationassert.HasDomainTargetCode(t, result.Targets, codeInvocationSignatureIncompatibleInterpolation)
}

func validationResult(codes ...string) interfaces.ValidationResult {
	targets := make([]interfaces.ValidationTarget, 0, len(codes))
	for _, code := range codes {
		targets = append(targets, interfaces.ValidationTarget{
			Code: code, Severity: interfaces.ValidationSeverityError,
			Message: code,
			Subject: interfaces.ValidationSubject{
				Type: interfaces.ValidationSubjectTypeFactory,
				ID:   code, Location: interfaces.ValidationSubjectLocationDefinition,
			},
		})
	}
	return interfaces.ValidationResult{Targets: targets}
}
