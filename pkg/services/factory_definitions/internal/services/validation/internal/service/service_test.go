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
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
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

func newValidationServiceWithConfig(
	t *testing.T,
	cfg *factorycontracts.FactoryConfig,
	checker factorycontracts.RequiredToolChecker,
	orchestratorValidator factorycontracts.OrchestratorDefinitionValidator,
) validationservice.Service {
	t.Helper()
	validator := factoryvalidation.New(orchestratorValidator)
	svc, err := validationwire.NewService(validationservice.Dependencies{
		Operations:            validator,
		Effective:             validator,
		LoadCanonical:         stubLoadCanonicalForConfig(cfg),
		RequiredToolChecker:   checker,
		OrchestratorValidator: orchestratorValidator,
	})
	if err != nil {
		t.Fatalf("validationwire.NewService: %v", err)
	}
	return svc
}

func stubLoadCanonicalForConfig(
	cfg *factorycontracts.FactoryConfig,
) factorycontracts.CanonicalFactoryJSONLoader {
	return func(_ []byte, _ factorycontracts.WorkstationLoader) (factorycontracts.MutableLoadedFactorySource, error) {
		return stubLoadedSource{cfg: cfg}, nil
	}
}

func validPetriFactoryConfig() *factorycontracts.FactoryConfig {
	return &factorycontracts.FactoryConfig{
		Name: "structural-validation",
		WorkTypes: []factorycontracts.WorkTypeConfig{{
			Name: "task",
			States: []factorycontracts.StateConfig{
				{Name: "init", Type: factorycontracts.StateTypeInitial},
				{Name: "done", Type: factorycontracts.StateTypeTerminal},
				{Name: "failed", Type: factorycontracts.StateTypeFailed},
			},
		}},
		Workers: []workerconfig.Config{{Name: "worker-a"}},
		Workstations: []factorycontracts.FactoryWorkstationConfig{{
			Name:           "process",
			WorkerTypeName: "worker-a",
			Inputs:         []factorycontracts.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []factorycontracts.IOConfig{{WorkTypeName: "task", StateName: "done"}},
			OnFailure:      []factorycontracts.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
		}},
	}
}

func TestValidationService_RejectsMissingDependencies(t *testing.T) {
	t.Parallel()
	if svc := validationserviceimpl.New(nil, nil, nil, nil, nil); svc != nil {
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

func TestValidationService_WiredStructuralValidationSucceedsForValidPetriFactory(t *testing.T) {
	t.Parallel()

	svc := newValidationServiceWithConfig(t, validPetriFactoryConfig(), nil, nil)
	result, err := svc.ValidateStructuralFactoryDefinition(
		context.Background(),
		factoryroot.ValidateStructuralFactoryDefinitionRequest{
			Canonical: []byte(`{"name":"structural-validation"}`),
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

func TestValidationService_WiredStructuralValidationReturnsTypedDuplicateWorkerTarget(t *testing.T) {
	t.Parallel()

	cfg := validPetriFactoryConfig()
	cfg.Workers = append(cfg.Workers, workerconfig.Config{Name: "worker-a"})
	svc := newValidationServiceWithConfig(t, cfg, nil, nil)

	_, err := svc.ValidateStructuralFactoryDefinition(
		context.Background(),
		factoryroot.ValidateStructuralFactoryDefinitionRequest{
			Canonical: []byte(`{"name":"structural-validation"}`),
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
	found := false
	for _, target := range validationFailure.Validation.Targets {
		if target.Code == factoryvalidation.CodeDuplicateIdentifier &&
			target.Severity == factorycontracts.ValidationSeverityError &&
			target.Subject.Type == factorycontracts.ValidationSubjectTypeWorker {
			found = true
		}
	}
	if !found {
		t.Fatalf("validation targets = %#v, want duplicate worker structural target", validationFailure.Validation.Targets)
	}
}

func TestValidationService_WiredTopologyValidationReturnsTypedDanglingPlaceTarget(t *testing.T) {
	t.Parallel()

	cfg := validPetriFactoryConfig()
	cfg.Workstations[0].Outputs = []factorycontracts.IOConfig{{
		WorkTypeName: "task",
		StateName:    "bogus",
	}}
	svc := newValidationServiceWithConfig(t, cfg, nil, nil)

	_, err := svc.ValidateStructuralFactoryDefinition(
		context.Background(),
		factoryroot.ValidateStructuralFactoryDefinitionRequest{
			Canonical: []byte(`{"name":"structural-validation"}`),
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
	found := false
	for _, target := range validationFailure.Validation.Targets {
		if target.Code == factoryvalidation.CodeDanglingPlaceReference &&
			target.Severity == factorycontracts.ValidationSeverityError &&
			target.Subject.Type == factorycontracts.ValidationSubjectTypeRoute {
			found = true
		}
	}
	if !found {
		t.Fatalf("validation targets = %#v, want dangling place topology target", validationFailure.Validation.Targets)
	}
}

type wiredStubRequiredToolChecker map[string]factorycontracts.RequiredToolCheckResult

func (s wiredStubRequiredToolChecker) Check(tool factorycontracts.RequiredToolConfig) factorycontracts.RequiredToolCheckResult {
	if result, ok := s[tool.Command]; ok {
		return result
	}
	return factorycontracts.RequiredToolCheckResult{}
}

func TestValidationService_WiredRequiredToolValidationSucceedsWhenCheckerReportsPresent(t *testing.T) {
	t.Parallel()

	cfg := validPetriFactoryConfig()
	cfg.ResourceManifest = &factorycontracts.PortableResourceManifestConfig{
		RequiredTools: []factorycontracts.RequiredToolConfig{{
			Name:    "Portable helper",
			Command: "present-tool",
		}},
	}
	svc := newValidationServiceWithConfig(t, cfg, wiredStubRequiredToolChecker{
		"present-tool": {},
	}, nil)

	result, err := svc.ValidateStructuralFactoryDefinition(
		context.Background(),
		factoryroot.ValidateStructuralFactoryDefinitionRequest{
			Canonical: []byte(`{"name":"structural-validation"}`),
			Profile:   factoryroot.ValidationProfileTopology,
		},
	)
	if err != nil {
		t.Fatalf("ValidateStructuralFactoryDefinition: %v", err)
	}
	for _, target := range result.Validation.Targets {
		if target.Code == factoryvalidation.CodeRequiredToolMissing ||
			target.Code == factoryvalidation.CodeRequiredToolVersionProbe {
			t.Fatalf("validation findings = %#v, want no required-tool failures", result.Validation.Targets)
		}
	}
}

func TestValidationService_WiredRequiredToolValidationReturnsTypedMissingToolTarget(t *testing.T) {
	t.Parallel()

	cfg := validPetriFactoryConfig()
	cfg.ResourceManifest = &factorycontracts.PortableResourceManifestConfig{
		RequiredTools: []factorycontracts.RequiredToolConfig{{
			Name:    "Missing helper",
			Command: "missing-tool",
		}},
	}
	svc := newValidationServiceWithConfig(t, cfg, wiredStubRequiredToolChecker{
		"missing-tool": {
			FailureKind: factorycontracts.RequiredToolFailureKindMissing,
			Err:         errors.New(`required tool "Missing helper" command "missing-tool" was not found on PATH`),
		},
	}, nil)

	_, err := svc.ValidateStructuralFactoryDefinition(
		context.Background(),
		factoryroot.ValidateStructuralFactoryDefinitionRequest{
			Canonical: []byte(`{"name":"structural-validation"}`),
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
	found := false
	for _, target := range validationFailure.Validation.Targets {
		if target.Code == factoryvalidation.CodeRequiredToolMissing &&
			target.Severity == factorycontracts.ValidationSeverityError &&
			target.Subject.Type == factorycontracts.ValidationSubjectTypeFactory &&
			target.Subject.ID == "Missing helper" {
			found = true
		}
	}
	if !found {
		t.Fatalf("validation targets = %#v, want typed missing required-tool target", validationFailure.Validation.Targets)
	}
}

type wiredStubOrchestratorValidator struct {
	targets []factorycontracts.ValidationTarget
}

func (s wiredStubOrchestratorValidator) ValidateJavaScriptFactoryDefinition(
	_ context.Context,
	_ *factorycontracts.FactoryOrchestratorJavaScriptConfig,
	_ factorycontracts.WorkflowSourceReader,
) []factorycontracts.ValidationTarget {
	return append([]factorycontracts.ValidationTarget(nil), s.targets...)
}

func TestValidationService_WiredOrchestratorValidationReturnsTypedUnsupportedKindTarget(t *testing.T) {
	t.Parallel()

	cfg := validPetriFactoryConfig()
	cfg.Orchestrator = &factorycontracts.FactoryOrchestratorConfig{Kind: "LEGACY"}
	svc := newValidationServiceWithConfig(t, cfg, nil, nil)

	_, err := svc.ValidateStructuralFactoryDefinition(
		context.Background(),
		factoryroot.ValidateStructuralFactoryDefinitionRequest{
			Canonical: []byte(`{"name":"structural-validation"}`),
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
	found := false
	for _, target := range validationFailure.Validation.Targets {
		if target.Code == factoryvalidation.CodeOrchestratorUnsupportedKind &&
			target.Severity == factorycontracts.ValidationSeverityError &&
			target.Subject.Type == factorycontracts.ValidationSubjectTypeFactory {
			found = true
		}
	}
	if !found {
		t.Fatalf("validation targets = %#v, want unsupported orchestrator kind target", validationFailure.Validation.Targets)
	}
}

func TestValidationService_WiredOrchestratorValidationMergesRuntimeValidatorTargets(t *testing.T) {
	t.Parallel()

	cfg := &factorycontracts.FactoryConfig{
		Name: "javascript-orchestrator",
		Orchestrator: &factorycontracts.FactoryOrchestratorConfig{
			Kind: factorycontracts.OrchestratorKindJavaScript,
			JavaScript: &factorycontracts.FactoryOrchestratorJavaScriptConfig{
				SourceRef:  "factory/workflows/review.js",
				Entrypoint: "main",
			},
		},
	}
	svc := newValidationServiceWithConfig(t, cfg, nil, wiredStubOrchestratorValidator{targets: []factorycontracts.ValidationTarget{{
		Code:     "factory.orchestrator.javascript.invalidPolicy",
		Severity: factorycontracts.ValidationSeverityError,
		Message:  "invalid default policy",
		Subject: factorycontracts.ValidationSubject{
			Type:     factorycontracts.ValidationSubjectTypeFactory,
			ID:       "factory",
			Location: factorycontracts.ValidationSubjectLocationDefinition,
		},
		Path: "factory.orchestrator.javascript.defaultPolicy",
	}}})

	_, err := svc.ValidateStructuralFactoryDefinition(
		context.Background(),
		factoryroot.ValidateStructuralFactoryDefinitionRequest{
			Canonical: []byte(`{"name":"javascript-orchestrator"}`),
			Profile:   factoryroot.ValidationProfileTopology,
		},
	)
	var validationFailure *factoryroot.FactoryDefinitionValidationFailure
	if !errors.As(err, &validationFailure) {
		t.Fatalf("error = %v, want FactoryDefinitionValidationFailure", err)
	}
	found := false
	for _, target := range validationFailure.Validation.Targets {
		if target.Code == "factory.orchestrator.javascript.invalidPolicy" {
			found = true
		}
	}
	if !found {
		t.Fatalf("validation targets = %#v, want runtime orchestrator validator target", validationFailure.Validation.Targets)
	}
}
