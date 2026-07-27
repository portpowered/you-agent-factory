package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	validationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation"
	validationserviceimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/service"
	validationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/wire"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
)

type stubLoadedSource struct {
	cfg *factorycontracts.FactoryConfig
}

func (s stubLoadedSource) FactoryConfig() *factorycontracts.FactoryConfig { return s.cfg }
func (s stubLoadedSource) FactoryDir() string                             { return "" }
func (s stubLoadedSource) RuntimeBaseDir() string                         { return "" }
func (s stubLoadedSource) SetRuntimeBaseDir(string)                       {}
func (s stubLoadedSource) PortableBundledFileReplacements() []factorycontracts.PortableBundledFileReplacement {
	return nil
}
func (s stubLoadedSource) MutateWorkers(func(*workerconfig.Config) error) error { return nil }
func (s stubLoadedSource) Workstation(string) (*factorycontracts.FactoryWorkstationConfig, bool) {
	return nil, false
}
func (s stubLoadedSource) Worker(string) (*workerconfig.Config, bool) { return nil, false }

func stubLoadCanonical(payload []byte, _ factorycontracts.WorkstationLoader) (factorycontracts.MutableLoadedFactorySource, error) {
	var cfg factorycontracts.FactoryConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return nil, factoryroot.ErrInvalidNamedFactory
	}
	return stubLoadedSource{cfg: &cfg}, nil
}

type stubOperations struct {
	structuralResult factorycontracts.ValidationResult
	effectiveResult  factorycontracts.ValidationResult
}

func (s stubOperations) ValidateDefinition(
	_ context.Context,
	_ factorycontracts.DefinitionValidationRequest,
) (factorycontracts.ValidationResult, error) {
	return s.structuralResult, nil
}

func (s stubOperations) ValidateSubmittedDefinition(
	ctx context.Context,
	request factorycontracts.SubmittedDefinitionValidationRequest,
) (factorycontracts.ValidationResult, error) {
	return s.ValidateDefinition(ctx, factorycontracts.DefinitionValidationRequest{
		Profile:              factorycontracts.ValidationProfileTopology,
		Config:               request.Config,
		WorkflowSourceReader: request.WorkflowSourceReader,
		SubmittedTaxonomy:    request.Taxonomy,
	})
}

func (s stubOperations) ValidateEffectiveDefinition(
	_ context.Context,
	_ factorycontracts.EffectiveDefinitionValidationRequest,
) (factorycontracts.ValidationResult, error) {
	return s.effectiveResult, nil
}

func newValidationService(t *testing.T, operations stubOperations) validationservice.Service {
	t.Helper()
	svc, err := validationwire.NewService(validationservice.Dependencies{
		Operations:    operations,
		Effective:     operations,
		LoadCanonical: stubLoadCanonical,
	})
	if err != nil {
		t.Fatalf("validationwire.NewService: %v", err)
	}
	return svc
}

func TestValidationService_RejectsMissingDependencies(t *testing.T) {
	t.Parallel()
	if svc := validationserviceimpl.New(nil, nil, nil); svc != nil {
		t.Fatal("expected nil service when dependencies are missing")
	}
	if _, err := validationwire.NewService(validationservice.Dependencies{}); err == nil {
		t.Fatal("expected dependency error")
	}
}

const minimalValidFactoryJSON = `{"name":"alpha"}`

func TestValidationService_ValidStructuralDefinitionSucceeds(t *testing.T) {
	t.Parallel()
	svc := newValidationService(t, stubOperations{})
	result, err := svc.ValidateStructuralFactoryDefinition(
		context.Background(),
		factoryroot.ValidateStructuralFactoryDefinitionRequest{
			Canonical: []byte(minimalValidFactoryJSON),
			Profile:   factoryroot.ValidationProfileTopology,
		},
	)
	if err != nil {
		t.Fatalf("ValidateStructuralFactoryDefinition: %v", err)
	}
	if result.Validation.HasBlockingTargets() {
		t.Fatalf("validation findings = %#v, want none", result.Validation)
	}
}

func TestValidationService_InvalidPayloadReturnsTypedError(t *testing.T) {
	t.Parallel()
	svc := newValidationService(t, stubOperations{})
	_, err := svc.ValidateStructuralFactoryDefinition(
		context.Background(),
		factoryroot.ValidateStructuralFactoryDefinitionRequest{Canonical: []byte("{")},
	)
	if !errors.Is(err, factoryroot.ErrInvalidFactoryDefinitionPayload) {
		t.Fatalf("error = %v, want %v", err, factoryroot.ErrInvalidFactoryDefinitionPayload)
	}
}

func TestValidationService_StructuralFindingsReturnValidationFailure(t *testing.T) {
	t.Parallel()
	svc := newValidationService(t, stubOperations{
		structuralResult: factorycontracts.ValidationResult{
			Targets: []factorycontracts.ValidationTarget{{
				Code:     factorycontracts.ValidationCodeFactoryPayloadInvalid,
				Severity: factorycontracts.ValidationSeverityError,
				Message:  "definition validation failed",
			}},
		},
	})
	_, err := svc.ValidateStructuralFactoryDefinition(
		context.Background(),
		factoryroot.ValidateStructuralFactoryDefinitionRequest{
			Canonical: []byte(minimalValidFactoryJSON),
			Profile:   factoryroot.ValidationProfileTopology,
		},
	)
	var validationFailure *factoryroot.FactoryDefinitionValidationFailure
	if !errors.As(err, &validationFailure) {
		t.Fatalf("error = %v, want FactoryDefinitionValidationFailure", err)
	}
	if !errors.Is(err, factoryroot.ErrFactoryDefinitionValidationFailed) {
		t.Fatalf("error = %v, want %v", err, factoryroot.ErrFactoryDefinitionValidationFailed)
	}
	if len(validationFailure.Validation.Targets) == 0 {
		t.Fatal("expected validation targets")
	}
}

func TestValidationService_ValidEffectiveDefinitionSucceeds(t *testing.T) {
	t.Parallel()
	svc := newValidationService(t, stubOperations{})
	payload := []byte(minimalValidFactoryJSON)
	result, err := svc.ValidateEffectiveFactoryDefinition(
		context.Background(),
		factoryroot.ValidateEffectiveFactoryDefinitionRequest{
			Canonical: payload,
			Effective: factoryroot.EffectiveFactorySource{
				ContentIdentity: string(payload),
			},
		},
	)
	if err != nil {
		t.Fatalf("ValidateEffectiveFactoryDefinition: %v", err)
	}
	if result.Validation.HasBlockingTargets() {
		t.Fatalf("validation findings = %#v, want none", result.Validation)
	}
}
