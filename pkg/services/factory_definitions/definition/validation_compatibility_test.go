package factorydefinition_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func validateFactoryAPIPrePersistForTest(
	ctx context.Context,
	factory factoryapi.Factory,
) (factorydefinitions.ValidationResult, error) {
	payload, err := json.Marshal(factory)
	if err != nil {
		return factorydefinitions.ValidationResult{}, err
	}
	request, err := validationentry.MapFactoryJSONForPersistence(payload, testCanonicalFactoryLoader)
	if err != nil {
		return factorydefinitions.ValidationResult{}, err
	}
	request.Profile = factorydefinitions.ValidationProfilePrePersist
	return testFactoryDefinitionValidator().ValidateDefinition(ctx, request)
}

func TestValidateEditableFactoryTopology_MatchesValidateFactoryAPIPrePersist(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}

	apiResult, err := validateFactoryAPIPrePersistForTest(context.Background(), factory)
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}

	saveErr := validateEditableFactoryTopology(factory, nil)
	var topologyErr *factorydefinitions.ValidationTopologyError
	if !errors.As(saveErr, &topologyErr) {
		t.Fatalf("validateEditableFactoryTopology error = %v, want topology validation error", saveErr)
	}

	apiSignatures := factoryvalidation.CanonicalTargetSignatures(apiResult.BlockingTargets())
	saveSignatures := factoryvalidation.CanonicalTargetSignatures(topologyErr.Targets)
	if !factoryvalidation.EquivalentCanonicalTargetSignatures(apiSignatures, saveSignatures) {
		t.Fatalf("ValidateFactoryAPI signatures = %#v, save signatures = %#v",
			apiSignatures, saveSignatures)
	}
	validationassert.HasDomainTargetCode(t, topologyErr.Targets, factoryvalidation.CodeDuplicateIdentifier)
}

func TestValidateEditableFactoryTopology_ReturnsTargetsForResourceSlotRoutes(t *testing.T) {
	t.Parallel()

	factory := factoryWithResourceSlotRoutes()

	saveErr := validateEditableFactoryTopology(factory, nil)
	var topologyErr *factorydefinitions.ValidationTopologyError
	if !errors.As(saveErr, &topologyErr) {
		t.Fatalf("validateEditableFactoryTopology error = %v, want topology validation error", saveErr)
	}

	validationassert.HasDomainTargetCode(t, topologyErr.Targets, factoryvalidation.CodeDanglingPlaceReference)
	validationassert.HasDomainTargetSubject(t, topologyErr.Targets, factorydefinitions.ValidationSubject{
		Type: factorydefinitions.ValidationSubjectTypeRoute, ID: "cleaner->executor-slot:available",
		Location: factorydefinitions.ValidationSubjectLocationInputs,
	})
}

func TestValidateEditableFactoryTopology_ValidFactory_NoError(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}

	if err := validateEditableFactoryTopology(factory, nil); err != nil {
		t.Fatalf("validateEditableFactoryTopology: %v", err)
	}
}

func TestValidateEditableFactoryTopology_RejectsDuplicateDefaultHandlingWorkTypes(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	if factory.WorkTypes == nil || len(*factory.WorkTypes) < 1 {
		t.Fatal("expected alpha fixture work types")
	}
	defaultBehavior := factoryapi.WorkTypeHandlingBehaviorDefault
	(*factory.WorkTypes)[0].HandlingBehavior = &[]factoryapi.WorkTypeHandlingBehavior{defaultBehavior}
	second := (*factory.WorkTypes)[0]
	second.Name = "second-default"
	second.HandlingBehavior = &[]factoryapi.WorkTypeHandlingBehavior{defaultBehavior}
	*factory.WorkTypes = append(*factory.WorkTypes, second)

	saveErr := validateEditableFactoryTopology(factory, nil)
	var topologyErr *factorydefinitions.ValidationTopologyError
	if !errors.As(saveErr, &topologyErr) {
		t.Fatalf("validateEditableFactoryTopology error = %v, want topology validation error", saveErr)
	}
	validationassert.HasDomainTargetCode(t, topologyErr.Targets, factoryvalidation.CodeWorkTypeHandlingBehaviorUniqueDefault)
}

func TestValidateEditableFactoryTopology_AllowsSingleDefaultHandlingWorkType(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	if factory.WorkTypes == nil || len(*factory.WorkTypes) < 1 {
		t.Fatal("expected alpha fixture work types")
	}
	defaultBehavior := factoryapi.WorkTypeHandlingBehaviorDefault
	(*factory.WorkTypes)[0].HandlingBehavior = &[]factoryapi.WorkTypeHandlingBehavior{defaultBehavior}

	if err := validateEditableFactoryTopology(factory, nil); err != nil {
		t.Fatalf("validateEditableFactoryTopology: %v", err)
	}
}

func TestValidateUpsertNamedFactoryRequest_RejectsInvalidFactoryName(t *testing.T) {
	t.Parallel()

	factory := factoryapi.Factory{Name: ".."}

	err := validateUpsertNamedFactoryRequest(factory, nil)
	if !errors.Is(err, apisurface.ErrInvalidNamedFactoryName) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactoryName", err)
	}
}

func TestValidateEditableFactoryTopology_MatchesValidateFactoryAPIForInvocationReturnFinding(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	explicit := factoryapi.InvocationReturnPolicyExplicit
	factory.InvocationReturn = &factoryapi.InvocationReturn{
		Policy:        explicit,
		WorkTypeName:  stringPtr("missing-work-type"),
		TerminalState: stringPtr("complete"),
	}

	apiResult, err := validateFactoryAPIPrePersistForTest(context.Background(), factory)
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}

	saveErr := validateEditableFactoryTopology(factory, nil)
	var topologyErr *factorydefinitions.ValidationTopologyError
	if !errors.As(saveErr, &topologyErr) {
		t.Fatalf("validateEditableFactoryTopology error = %v, want topology validation error", saveErr)
	}

	apiSignatures := factoryvalidation.CanonicalTargetSignatures(apiResult.Targets)
	saveSignatures := factoryvalidation.CanonicalTargetSignatures(topologyErr.Targets)
	if !factoryvalidation.EquivalentCanonicalTargetSignatures(apiSignatures, saveSignatures) {
		t.Fatalf("ValidateFactoryAPI signatures = %#v, save signatures = %#v",
			apiSignatures, saveSignatures)
	}
	validationassert.HasDomainTargetCode(t, topologyErr.Targets, factoryvalidation.CodeInvocationReturnUnknownWorkTypeName)
}

func TestValidateEditableFactoryTopology_MatchesValidateFactoryAPIForWorkPropagationFinding(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	unsupportedMode := factoryapi.WorkPropagationMode("MERGE_PAYLOAD")
	workstations := *factory.Workstations
	workstations[0].WorkPropagation = &factoryapi.WorkPropagation{Mode: unsupportedMode}
	factory.Workstations = &workstations

	apiResult, err := validateFactoryAPIPrePersistForTest(context.Background(), factory)
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}

	saveErr := validateEditableFactoryTopology(factory, nil)
	var topologyErr *factorydefinitions.ValidationTopologyError
	if !errors.As(saveErr, &topologyErr) {
		t.Fatalf("validateEditableFactoryTopology error = %v, want topology validation error", saveErr)
	}

	apiSignatures := factoryvalidation.CanonicalTargetSignatures(apiResult.Targets)
	saveSignatures := factoryvalidation.CanonicalTargetSignatures(topologyErr.Targets)
	if !factoryvalidation.EquivalentCanonicalTargetSignatures(apiSignatures, saveSignatures) {
		t.Fatalf("ValidateFactoryAPI signatures = %#v, save signatures = %#v",
			apiSignatures, saveSignatures)
	}
	validationassert.HasDomainTargetCode(t, topologyErr.Targets, factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode)
}

func stringPtr(value string) *string {
	return &value
}

func factoryWithResourceSlotRoutes() factoryapi.Factory {
	workerType := factoryapi.WorkerTypeModelWorker
	workstationType := factoryapi.WorkstationTypeModelWorkstation
	outputs := []factoryapi.WorkstationIO{
		{WorkType: "cron-triggers", State: "complete"},
		{WorkType: "executor-slot", State: "available"},
	}
	onFailure := []factoryapi.WorkstationIO{{WorkType: "cron-triggers", State: "failed"}}
	onRejection := []factoryapi.WorkstationIO{{WorkType: "cron-triggers", State: "failed"}}

	return factoryapi.Factory{
		Name: "UNDEFINED",
		Resources: &[]factoryapi.Resource{{
			Capacity: 10,
			Id:       stringPtr("executor-slot"),
			Name:     "executor-slot",
		}},
		Workers: &[]factoryapi.Worker{{
			Id:   stringPtr("processor"),
			Name: "processor",
			Type: &workerType,
		}},
		WorkTypes: &[]factoryapi.WorkType{{
			Id:   stringPtr("cron-triggers"),
			Name: "cron-triggers",
			States: []factoryapi.WorkState{
				{Id: stringPtr("init"), Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
				{Id: stringPtr("complete"), Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
				{Id: stringPtr("failed"), Name: "failed", Type: factoryapi.WorkStateTypeFAILED},
			},
		}},
		Workstations: &[]factoryapi.Workstation{{
			Id:          stringPtr("cleaner"),
			Inputs:      []factoryapi.WorkstationIO{{WorkType: "executor-slot", State: "available"}},
			Name:        "cleaner",
			OnFailure:   &onFailure,
			OnRejection: &onRejection,
			Outputs:     &outputs,
			Type:        &workstationType,
			Worker:      "processor",
		}},
	}
}

func assertValidationTarget(
	t *testing.T,
	targets []factoryapi.FactoryValidationTarget,
	want factoryapi.FactoryValidationTarget,
) {
	t.Helper()
	for _, target := range targets {
		if target.Code == want.Code &&
			target.Message == want.Message &&
			target.Subject == want.Subject &&
			target.Severity == want.Severity {
			return
		}
	}
	t.Fatalf("validation targets = %#v, want %#v", targets, want)
}
