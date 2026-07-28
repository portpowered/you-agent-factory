package factorydefinition_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"
	validationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation"
	validationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/wire"
	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// newRootValidateServiceForPeer attaches private validation behind the public root
// Service. Construction may import owner-local wire; peer exercise below must
// not depend on validation or other Definitions internals beyond the root Service.
func newRootValidateServiceForPeer(t *testing.T) factoryroot.Service {
	t.Helper()

	fileSystem := platformfilesystem.Local{}
	paths, err := factorynamedpaths.New(fileSystem)
	if err != nil {
		t.Fatalf("namedpaths.New: %v", err)
	}
	catalogService, err := catalogwire.NewService(catalog.Dependencies{
		Paths:      paths,
		FileSystem: fileSystem,
	})
	if err != nil {
		t.Fatalf("catalogwire.NewService: %v", err)
	}

	validator := testFactoryDefinitionValidator()
	validationService, err := validationwire.NewService(validationservice.Dependencies{
		Operations:    validator,
		Effective:     validator,
		LoadCanonical: testCanonicalFactoryLoader,
	})
	if err != nil {
		t.Fatalf("validationwire.NewService: %v", err)
	}
	return factorydefinition.NewWithValidation(nil, factorydefinition.StubActivationGateway(), catalogService, validationService)
}

func crossPathValidAlphaEffectivePayload(t *testing.T) []byte {
	t.Helper()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	if factory.WorkTypes == nil || len(*factory.WorkTypes) < 1 {
		t.Fatal("expected alpha fixture work types")
	}
	defaultBehavior := factoryapi.WorkTypeHandlingBehaviorDefault
	(*factory.WorkTypes)[0].HandlingBehavior = &[]factoryapi.WorkTypeHandlingBehavior{defaultBehavior}

	payload, err := json.Marshal(factory)
	if err != nil {
		t.Fatalf("Marshal(alpha factory): %v", err)
	}
	return payload
}

// peerExerciseRootValidateSuccess proves a peer-shaped consumer can drive
// CTR-DEF validate success cases through the attached private implementation
// while depending only on the root Service vocabulary.
func peerExerciseRootValidateSuccess(
	t *testing.T,
	service factoryroot.Service,
	structuralPayload []byte,
	effectivePayload []byte,
) {
	t.Helper()
	ctx := context.Background()

	structural, err := service.ValidateStructuralFactoryDefinition(
		ctx,
		factoryroot.ValidateStructuralFactoryDefinitionRequest{
			Canonical: structuralPayload,
			Profile:   factoryroot.ValidationProfileTopology,
		},
	)
	if err != nil {
		t.Fatalf("ValidateStructuralFactoryDefinition: %v", err)
	}
	if structural.Validation.HasBlockingTargets() {
		t.Fatalf(
			"ValidateStructuralFactoryDefinition findings = %#v, want none",
			structural.Validation,
		)
	}

	effective, err := service.ValidateEffectiveFactoryDefinition(
		ctx,
		factoryroot.ValidateEffectiveFactoryDefinitionRequest{
			Canonical: effectivePayload,
			Effective: factoryroot.EffectiveFactorySource{
				FactoryDir:      "/factories/alpha",
				RuntimeBaseDir:  "/factories/alpha",
				ContentIdentity: string(effectivePayload),
			},
		},
	)
	if err != nil {
		t.Fatalf("ValidateEffectiveFactoryDefinition: %v", err)
	}
	if effective.Validation.HasBlockingTargets() {
		t.Fatalf(
			"ValidateEffectiveFactoryDefinition findings = %#v, want none",
			effective.Validation,
		)
	}
}

// peerExerciseRootValidateTypedFailures proves a peer-shaped consumer can
// distinguish CTR-DEF typed validate failures through the attached private
// implementation using only root vocabulary.
func peerExerciseRootValidateTypedFailures(t *testing.T, service factoryroot.Service, invalidPayload []byte) {
	t.Helper()
	ctx := context.Background()

	_, invalidErr := service.ValidateStructuralFactoryDefinition(
		ctx,
		factoryroot.ValidateStructuralFactoryDefinitionRequest{Canonical: []byte("{")},
	)
	if !errors.Is(invalidErr, factoryroot.ErrInvalidFactoryDefinitionPayload) {
		t.Fatalf(
			"ValidateStructuralFactoryDefinition invalid-payload error = %v, want %v",
			invalidErr,
			factoryroot.ErrInvalidFactoryDefinitionPayload,
		)
	}

	_, findingsErr := service.ValidateStructuralFactoryDefinition(
		ctx,
		factoryroot.ValidateStructuralFactoryDefinitionRequest{
			Canonical: invalidPayload,
			Profile:   factoryroot.ValidationProfileTopology,
		},
	)
	var validationFailure *factoryroot.FactoryDefinitionValidationFailure
	if !errors.As(findingsErr, &validationFailure) {
		t.Fatalf(
			"ValidateStructuralFactoryDefinition error = %v, want FactoryDefinitionValidationFailure",
			findingsErr,
		)
	}
	if !errors.Is(findingsErr, factoryroot.ErrFactoryDefinitionValidationFailed) {
		t.Fatalf(
			"ValidateStructuralFactoryDefinition error = %v, want %v",
			findingsErr,
			factoryroot.ErrFactoryDefinitionValidationFailed,
		)
	}
	if errors.Is(findingsErr, factoryroot.ErrInvalidFactoryDefinitionPayload) {
		t.Fatal("validation findings must not also match ErrInvalidFactoryDefinitionPayload")
	}
	if len(validationFailure.Validation.Targets) == 0 {
		t.Fatal("FactoryDefinitionValidationFailure must carry validation targets")
	}

	hasErrorFinding := false
	for _, target := range validationFailure.Validation.Targets {
		if target.Severity == factoryroot.ValidationSeverityError {
			hasErrorFinding = true
		}
		if strings.Contains(strings.ToLower(target.Code), "petri") ||
			strings.Contains(strings.ToLower(target.Message), "petri") {
			t.Fatalf("published validation findings must not use Petri vocabulary: %#v", target)
		}
	}
	if !hasErrorFinding {
		t.Fatal("FactoryDefinitionValidationFailure must carry at least one error-severity finding")
	}

	validationassert.HasDomainTargetCode(
		t,
		validationFailure.Validation.Targets,
		factoryvalidation.CodeDuplicateIdentifier,
	)
	validationassert.HasDomainTargetCode(
		t,
		validationFailure.Validation.Targets,
		factoryvalidation.CodeDanglingWorkerReference,
	)
	validationassert.HasDomainTargetCode(
		t,
		validationFailure.Validation.Targets,
		factoryvalidation.CodeDanglingPlaceReference,
	)
}

func TestRootValidateEquivalence_CTRDEFSuccessThroughPrivateImplementation(t *testing.T) {
	t.Parallel()

	structuralPayload := []byte(factoryfixtures.CrossPathValidAlphaFactoryJSON)
	effectivePayload := crossPathValidAlphaEffectivePayload(t)
	service := newRootValidateServiceForPeer(t)

	peerExerciseRootValidateSuccess(t, service, structuralPayload, effectivePayload)
}

func TestRootValidateEquivalence_CTRDEFTypedFailuresThroughPrivateImplementation(t *testing.T) {
	t.Parallel()

	invalidPayload := []byte(factoryfixtures.CrossPathInvalidFactoryJSON)
	service := newRootValidateServiceForPeer(t)

	peerExerciseRootValidateTypedFailures(t, service, invalidPayload)
}

func TestRootValidateEquivalence_PeerExercisesRootWithoutValidationImport(t *testing.T) {
	t.Parallel()

	// Owner-local construction attaches private validation. The peer exercise
	// helpers accept only factoryroot.Service and root request/result/error
	// types, proving a peer can drive the slice end-to-end without importing
	// validation or other Definitions internals.
	validStructuralPayload := []byte(factoryfixtures.CrossPathValidAlphaFactoryJSON)
	validEffectivePayload := crossPathValidAlphaEffectivePayload(t)
	successService := newRootValidateServiceForPeer(t)
	peerExerciseRootValidateSuccess(t, successService, validStructuralPayload, validEffectivePayload)

	invalidPayload := []byte(factoryfixtures.CrossPathInvalidFactoryJSON)
	failureService := newRootValidateServiceForPeer(t)
	peerExerciseRootValidateTypedFailures(t, failureService, invalidPayload)
}
