package definitions

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Validation/save parity crosses the public HTTP boundary on the canonical
// shared host. Each parallel case owns its folder, named definition and session.
func validatePublicFactoryForParity(t *testing.T, factory factoryapi.Factory) (factorydefinitions.ValidationResult, error) {
	t.Helper()
	factory.Name = "validation-fixture"
	result, _ := postValidateFactory(t, sharedDefinitionsValidationServer(t).URL(), factory)
	payload, err := json.Marshal(result)
	if err != nil {
		return factorydefinitions.ValidationResult{}, err
	}
	var domain factorydefinitions.ValidationResult
	err = json.Unmarshal(payload, &domain)
	return domain, err
}

func savePublicFactoryForParity(t *testing.T, factory factoryapi.Factory) error {
	t.Helper()
	host := sharedDefinitionsValidationServer(t)
	cfg := validAPIValidationFactoryConfig()
	cfg["name"] = "validation-fixture"
	dir := support.ScaffoldFactory(t, cfg)
	rootDir := t.TempDir()
	support.CreateAndActivateNamedFactoryAtRootWithProcess(t, buildDefinitionsProcess(t), host.env, dir, rootDir, "validation-fixture", filepath.Join(dir, "factory.json"))
	id := openDefinitionsNamedSession(t, host.URL(), rootDir, "validation-fixture")
	t.Cleanup(func() { closeDefinitionsFactorySession(t, host.URL(), id) })
	endpoint := host.URL() + "/factory-sessions/" + id + "/factory"
	before := support.GetJSON[factoryapi.Factory](t, endpoint)
	factory.Name = before.Name
	version := *before.Version
	factory.Version = &version
	factory.Version.Logical++
	factory.Version.Physical = factory.Version.Physical.Add(time.Nanosecond)
	payload, err := json.Marshal(factoryapi.SaveFactoryForSessionRequest{Factory: factory})
	if err != nil {
		return err
	}
	response, _, status := definitionsHTTPRequest(t, http.MethodPut, endpoint, payload)
	if status == http.StatusOK {
		return nil
	}
	if after := support.GetJSON[factoryapi.Factory](t, endpoint); !reflect.DeepEqual(after, before) {
		t.Fatalf("rejected save changed selected definition: before=%#v after=%#v", before, after)
	}
	var failure factoryapi.ErrorResponse
	if err := json.Unmarshal(response, &failure); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusBadRequest || failure.Code != "INVALID_FACTORY" || failure.Targets == nil {
		t.Fatalf("save = %d %s, want typed INVALID_FACTORY with targets", status, response)
	}
	var targets []factorydefinitions.ValidationTarget
	payload, err = json.Marshal(*failure.Targets)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &targets); err != nil {
		t.Fatal(err)
	}
	return &factorydefinitions.ValidationTopologyError{Targets: targets}
}

func publicTargetSignatures(targets []factorydefinitions.ValidationTarget) []string {
	signatures := make([]string, 0, len(targets))
	for _, target := range targets {
		payload, _ := json.Marshal(struct {
			Code     string
			Severity factorydefinitions.ValidationSeverity
			Subject  factorydefinitions.ValidationSubject
		}{target.Code, target.Severity, target.Subject})
		signatures = append(signatures, string(payload))
	}
	sort.Strings(signatures)
	return signatures
}

func TestPublicValidationAndSaveUseTheirDocumentedProfiles(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}

	apiResult, err := validatePublicFactoryForParity(t, factory)
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}

	saveErr := savePublicFactoryForParity(t, factory)
	var topologyErr *factorydefinitions.ValidationTopologyError
	if !errors.As(saveErr, &topologyErr) {
		t.Fatalf("validateEditableFactoryTopology error = %v, want topology validation error", saveErr)
	}

	// HTTP validation uses Topology; save uses PrePersist. Both retain the
	// actionable structural findings; only Topology requires completion paths.
	wantSaveCodes := []string{"factory.duplicateIdentifier", "factory.duplicateIdentifier", "factory.route.danglingPlaceReference", "factory.worker.danglingReference"}
	wantAPICodes := append(append([]string(nil), wantSaveCodes...), "factory.workState.missingTerminalCompletionPath", "factory.workType.missingCompletionState", "factory.workType.missingFailureState", "factory.workstation.missingFailureRoute")
	assertPublicTargetCodes(t, topologyErr.Targets, wantSaveCodes)
	assertPublicTargetCodes(t, apiResult.BlockingTargets(), wantAPICodes)
	validationassert.HasDomainTargetCode(t, topologyErr.Targets, "factory.duplicateIdentifier")
}

func TestValidateEditableFactoryTopology_ReturnsTargetsForResourceSlotRoutes(t *testing.T) {
	t.Parallel()

	factory := factoryWithResourceSlotRoutes()

	saveErr := savePublicFactoryForParity(t, factory)
	var topologyErr *factorydefinitions.ValidationTopologyError
	if !errors.As(saveErr, &topologyErr) {
		t.Fatalf("validateEditableFactoryTopology error = %v, want topology validation error", saveErr)
	}

	validationassert.HasDomainTargetCode(t, topologyErr.Targets, "factory.route.danglingPlaceReference")
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

	if err := savePublicFactoryForParity(t, factory); err != nil {
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

	saveErr := savePublicFactoryForParity(t, factory)
	var topologyErr *factorydefinitions.ValidationTopologyError
	if !errors.As(saveErr, &topologyErr) {
		t.Fatalf("validateEditableFactoryTopology error = %v, want topology validation error", saveErr)
	}
	validationassert.HasDomainTargetCode(t, topologyErr.Targets, "work-type-handling-behavior-unique-default")
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

	if err := savePublicFactoryForParity(t, factory); err != nil {
		t.Fatalf("validateEditableFactoryTopology: %v", err)
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

	apiResult, err := validatePublicFactoryForParity(t, factory)
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}

	saveErr := savePublicFactoryForParity(t, factory)
	var topologyErr *factorydefinitions.ValidationTopologyError
	if !errors.As(saveErr, &topologyErr) {
		t.Fatalf("validateEditableFactoryTopology error = %v, want topology validation error", saveErr)
	}

	apiSignatures := publicTargetSignatures(apiResult.Targets)
	saveSignatures := publicTargetSignatures(topologyErr.Targets)
	if !reflect.DeepEqual(apiSignatures, saveSignatures) {
		t.Fatalf("ValidateFactoryAPI signatures = %#v, save signatures = %#v",
			apiSignatures, saveSignatures)
	}
	validationassert.HasDomainTargetCode(t, topologyErr.Targets, "factory.invocationReturn.unknownWorkTypeName")
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

	apiResult, err := validatePublicFactoryForParity(t, factory)
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}

	saveErr := savePublicFactoryForParity(t, factory)
	var topologyErr *factorydefinitions.ValidationTopologyError
	if !errors.As(saveErr, &topologyErr) {
		t.Fatalf("validateEditableFactoryTopology error = %v, want topology validation error", saveErr)
	}

	apiSignatures := publicTargetSignatures(apiResult.Targets)
	saveSignatures := publicTargetSignatures(topologyErr.Targets)
	if !reflect.DeepEqual(apiSignatures, saveSignatures) {
		t.Fatalf("ValidateFactoryAPI signatures = %#v, save signatures = %#v",
			apiSignatures, saveSignatures)
	}
	validationassert.HasDomainTargetCode(t, topologyErr.Targets, "factory.workstation.unsupportedWorkPropagationMode")
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
			Worker:      stringPtr("processor"),
		}},
	}
}

func TestValidateEditableFactoryTopology_RoutelessCronAndLogicalMove_InvalidFactoryTargets(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		decode      func() (factoryapi.Factory, error)
		workstation string
	}{
		{
			name:        "routeless_cron",
			decode:      factoryfixtures.DecodeRoutelessCronFactory,
			workstation: "cron",
		},
		{
			name:        "routeless_logical_move",
			decode:      factoryfixtures.DecodeRoutelessLogicalMoveFactory,
			workstation: "router",
		},
		{
			name:        "routeless_logical_move_cron",
			decode:      factoryfixtures.DecodeRoutelessLogicalMoveCronFactory,
			workstation: "trigger-monkey",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			factory, err := tc.decode()
			if err != nil {
				t.Fatalf("decode factory: %v", err)
			}

			saveErr := savePublicFactoryForParity(t, factory)
			var topologyErr *factorydefinitions.ValidationTopologyError
			if !errors.As(saveErr, &topologyErr) {
				t.Fatalf("validateEditableFactoryTopology error = %v, want topology validation error", saveErr)
			}

			validationassert.HasDomainTargetCode(t, topologyErr.Targets, "factory.workstation.missingOutputRoutes")
			validationassert.HasDomainTargetSubject(t, topologyErr.Targets, factorydefinitions.ValidationSubject{
				Type: factorydefinitions.ValidationSubjectTypeWorkstation, ID: tc.workstation,
				Location: factorydefinitions.ValidationSubjectLocationOutputs,
			})
			for _, target := range topologyErr.Targets {
				if target.Code == "factory.workstation.missingFailureRoute" &&
					target.Subject.Type == factorydefinitions.ValidationSubjectTypeWorkstation &&
					target.Subject.ID == tc.workstation &&
					target.Subject.Location == factorydefinitions.ValidationSubjectLocationOnFailure {
					t.Fatalf("targets = %#v, want no missingFailureRoute at ON_FAILURE for routeless workstation", topologyErr.Targets)
				}
			}
		})
	}
}

func TestValidateUpsertNamedFactoryRequest_RoutelessLogicalMove_InvalidFactoryTargets(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeRoutelessLogicalMoveFactory()
	if err != nil {
		t.Fatalf("DecodeRoutelessLogicalMoveFactory: %v", err)
	}

	saveErr := savePublicFactoryForParity(t, factory)
	var topologyErr *factorydefinitions.ValidationTopologyError
	if !errors.As(saveErr, &topologyErr) {
		t.Fatalf("validateUpsertNamedFactoryRequest error = %v, want topology validation error", saveErr)
	}

	validationassert.HasDomainTargetCode(t, topologyErr.Targets, "factory.workstation.missingOutputRoutes")
	validationassert.HasDomainTargetSubject(t, topologyErr.Targets, factorydefinitions.ValidationSubject{
		Type: factorydefinitions.ValidationSubjectTypeWorkstation, ID: "router",
		Location: factorydefinitions.ValidationSubjectLocationOutputs,
	})
}

func assertPublicTargetCodes(t *testing.T, targets []factorydefinitions.ValidationTarget, want []string) {
	t.Helper()
	got := make([]string, 0, len(targets))
	for _, target := range targets {
		got = append(got, target.Code)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diagnostic codes = %v, want %v", got, want)
	}
}
