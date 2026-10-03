package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	validationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/authoredmodel/workers"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	validationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/wire"
)

type stubLoadedSource struct {
	cfg *factoryroot.FactoryConfig
}

func (s stubLoadedSource) FactoryConfig() *factoryroot.FactoryConfig { return s.cfg }
func (s stubLoadedSource) FactoryDir() string                        { return "" }
func (s stubLoadedSource) RuntimeBaseDir() string                    { return "" }
func (s stubLoadedSource) SetRuntimeBaseDir(string)                  {}
func (s stubLoadedSource) PortableBundledFileReplacements() []factoryroot.PortableBundledFileReplacement {
	return nil
}
func (s stubLoadedSource) MutateWorkers(func(*workerconfig.Config) error) error { return nil }
func (s stubLoadedSource) Workstation(string) (*factoryroot.FactoryWorkstationConfig, bool) {
	return nil, false
}
func (s stubLoadedSource) Worker(string) (*workerconfig.Config, bool) { return nil, false }

func stubLoadCanonical(payload []byte, _ factoryroot.WorkstationLoader) (factoryroot.MutableLoadedFactorySource, error) {
	var cfg factoryroot.FactoryConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return nil, factoryroot.ErrInvalidNamedFactory
	}
	return stubLoadedSource{cfg: &cfg}, nil
}

type stubOperations struct {
	structuralResult factoryroot.ValidationResult
	effectiveResult  factoryroot.ValidationResult
}

func (s stubOperations) ValidateDefinition(
	_ context.Context,
	_ factoryroot.DefinitionValidationRequest,
) (factoryroot.ValidationResult, error) {
	return s.structuralResult, nil
}

func (s stubOperations) ValidateSubmittedDefinition(
	ctx context.Context,
	request factoryroot.SubmittedDefinitionValidationRequest,
) (factoryroot.ValidationResult, error) {
	return s.ValidateDefinition(ctx, factoryroot.DefinitionValidationRequest{
		Profile:              factoryroot.ValidationProfileTopology,
		Config:               request.Config,
		WorkflowSourceReader: request.WorkflowSourceReader,
		SubmittedTaxonomy:    request.Taxonomy,
	})
}

func (s stubOperations) ValidateEffectiveDefinition(
	_ context.Context,
	_ factoryroot.EffectiveDefinitionValidationRequest,
) (factoryroot.ValidationResult, error) {
	return s.effectiveResult, nil
}

func newValidationService(t *testing.T, operations stubOperations) validationservice.Service {
	t.Helper()
	svc := validationwire.NewService(operations, operations, stubLoadCanonical, nil, nil)
	return svc
}

func newValidationServiceWithConfig(
	t *testing.T,
	cfg *factoryroot.FactoryConfig,
	checker factoryroot.RequiredToolChecker,
	orchestratorValidator factoryroot.OrchestratorDefinitionValidator,
) validationservice.Service {
	t.Helper()
	validator := factoryvalidation.New(orchestratorValidator, stubLoadCanonicalForConfig(cfg))
	svc := validationwire.NewService(validator, validator, stubLoadCanonicalForConfig(cfg), checker, orchestratorValidator)
	return svc
}

func stubLoadCanonicalForConfig(
	cfg *factoryroot.FactoryConfig,
) factoryroot.CanonicalFactoryJSONLoader {
	return func(_ []byte, _ factoryroot.WorkstationLoader) (factoryroot.MutableLoadedFactorySource, error) {
		return stubLoadedSource{cfg: cfg}, nil
	}
}

func validPetriFactoryConfig() *factoryroot.FactoryConfig {
	return &factoryroot.FactoryConfig{
		Name: "structural-validation",
		WorkTypes: []factoryroot.WorkTypeConfig{{
			Name: "task",
			States: []factoryroot.StateConfig{
				{Name: "init", Type: factoryroot.StateTypeInitial},
				{Name: "done", Type: factoryroot.StateTypeTerminal},
				{Name: "failed", Type: factoryroot.StateTypeFailed},
			},
		}},
		Workers: []workerconfig.Config{{Name: "worker-a"}},
		Workstations: []factoryroot.FactoryWorkstationConfig{{
			Name:           "process",
			WorkerTypeName: "worker-a",
			Inputs:         []factoryroot.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []factoryroot.IOConfig{{WorkTypeName: "task", StateName: "done"}},
			OnFailure:      []factoryroot.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
		}},
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
		structuralResult: factoryroot.ValidationResult{
			Targets: []factoryroot.ValidationTarget{{
				Code:     factoryroot.ValidationCodeFactoryPayloadInvalid,
				Severity: factoryroot.ValidationSeverityError,
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
			target.Severity == factoryroot.ValidationSeverityError &&
			target.Subject.Type == factoryroot.ValidationSubjectTypeWorker {
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
	cfg.Workstations[0].Outputs = []factoryroot.IOConfig{{
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
			target.Severity == factoryroot.ValidationSeverityError &&
			target.Subject.Type == factoryroot.ValidationSubjectTypeRoute {
			found = true
		}
	}
	if !found {
		t.Fatalf("validation targets = %#v, want dangling place topology target", validationFailure.Validation.Targets)
	}
}

type wiredStubRequiredToolChecker map[string]factoryroot.RequiredToolCheckResult

func (s wiredStubRequiredToolChecker) Check(tool factoryroot.RequiredToolConfig) factoryroot.RequiredToolCheckResult {
	if result, ok := s[tool.Command]; ok {
		return result
	}
	return factoryroot.RequiredToolCheckResult{}
}

func TestValidationService_WiredRequiredToolValidationSucceedsWhenCheckerReportsPresent(t *testing.T) {
	t.Parallel()

	cfg := validPetriFactoryConfig()
	cfg.ResourceManifest = &factoryroot.PortableResourceManifestConfig{
		RequiredTools: []factoryroot.RequiredToolConfig{{
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
	cfg.ResourceManifest = &factoryroot.PortableResourceManifestConfig{
		RequiredTools: []factoryroot.RequiredToolConfig{{
			Name:    "Missing helper",
			Command: "missing-tool",
		}},
	}
	svc := newValidationServiceWithConfig(t, cfg, wiredStubRequiredToolChecker{
		"missing-tool": {
			FailureKind: factoryroot.RequiredToolFailureKindMissing,
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
			target.Severity == factoryroot.ValidationSeverityError &&
			target.Subject.Type == factoryroot.ValidationSubjectTypeFactory &&
			target.Subject.ID == "Missing helper" {
			found = true
		}
	}
	if !found {
		t.Fatalf("validation targets = %#v, want typed missing required-tool target", validationFailure.Validation.Targets)
	}
}

type wiredStubOrchestratorValidator struct {
	targets []factoryroot.ValidationTarget
}

func (s wiredStubOrchestratorValidator) ValidateJavaScriptFactoryDefinition(
	_ context.Context,
	_ *factoryroot.FactoryOrchestratorJavaScriptConfig,
	_ factoryroot.WorkflowSourceReader,
) []factoryroot.ValidationTarget {
	return append([]factoryroot.ValidationTarget(nil), s.targets...)
}

func TestValidationService_WiredOrchestratorValidationReturnsTypedUnsupportedKindTarget(t *testing.T) {
	t.Parallel()

	cfg := validPetriFactoryConfig()
	cfg.Orchestrator = &factoryroot.FactoryOrchestratorConfig{Kind: "LEGACY"}
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
			target.Severity == factoryroot.ValidationSeverityError &&
			target.Subject.Type == factoryroot.ValidationSubjectTypeFactory {
			found = true
		}
	}
	if !found {
		t.Fatalf("validation targets = %#v, want unsupported orchestrator kind target", validationFailure.Validation.Targets)
	}
}

func TestValidationService_WiredOrchestratorValidationMergesRuntimeValidatorTargets(t *testing.T) {
	t.Parallel()

	cfg := &factoryroot.FactoryConfig{
		Name: "javascript-orchestrator",
		Orchestrator: &factoryroot.FactoryOrchestratorConfig{
			Kind: factoryroot.OrchestratorKindJavaScript,
			JavaScript: &factoryroot.FactoryOrchestratorJavaScriptConfig{
				SourceRef:  "factory/workflows/review.js",
				Entrypoint: "main",
			},
		},
	}
	svc := newValidationServiceWithConfig(t, cfg, nil, wiredStubOrchestratorValidator{targets: []factoryroot.ValidationTarget{{
		Code:     "factory.orchestrator.javascript.invalidPolicy",
		Severity: factoryroot.ValidationSeverityError,
		Message:  "invalid default policy",
		Subject: factoryroot.ValidationSubject{
			Type:     factoryroot.ValidationSubjectTypeFactory,
			ID:       "factory",
			Location: factoryroot.ValidationSubjectLocationDefinition,
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

// validationObserver observes all supplied validation effect ports.
type validationObserver struct{ calls int }

func (o *validationObserver) ValidateDefinition(context.Context, factoryroot.DefinitionValidationRequest) (factoryroot.ValidationResult, error) {
	o.calls++
	return factoryroot.ValidationResult{}, nil
}
func (o *validationObserver) ValidateEffectiveDefinition(context.Context, factoryroot.EffectiveDefinitionValidationRequest) (factoryroot.ValidationResult, error) {
	o.calls++
	return factoryroot.ValidationResult{}, nil
}
func (o *validationObserver) Check(factoryroot.RequiredToolConfig) factoryroot.RequiredToolCheckResult {
	o.calls++
	return factoryroot.RequiredToolCheckResult{}
}
func (o *validationObserver) ValidateJavaScriptFactoryDefinition(context.Context, *factoryroot.FactoryOrchestratorJavaScriptConfig, factoryroot.WorkflowSourceReader) []factoryroot.ValidationTarget {
	o.calls++
	return nil
}

func TestValidationServiceConstructionAndCanceledRequestsHaveNoEffects(t *testing.T) {
	t.Parallel()
	observer := &validationObserver{}
	load := func([]byte, factoryroot.WorkstationLoader) (factoryroot.MutableLoadedFactorySource, error) {
		observer.calls++
		return stubLoadedSource{cfg: validPetriFactoryConfig()}, nil
	}
	svc := validationwire.NewService(observer, observer, load, observer, observer)
	if svc == nil || observer.calls != 0 {
		t.Fatalf("construction = %v, calls = %d", svc, observer.calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	structural, err := svc.ValidateStructuralFactoryDefinition(ctx, factoryroot.ValidateStructuralFactoryDefinitionRequest{Canonical: []byte(minimalValidFactoryJSON)})
	if !errors.Is(err, context.Canceled) || len(structural.Validation.Targets) != 0 {
		t.Fatalf("structural = %#v, error = %v", structural, err)
	}
	effective, err := svc.ValidateEffectiveFactoryDefinition(ctx, factoryroot.ValidateEffectiveFactoryDefinitionRequest{Canonical: []byte(minimalValidFactoryJSON)})
	if !errors.Is(err, context.Canceled) || len(effective.Validation.Targets) != 0 {
		t.Fatalf("effective = %#v, error = %v", effective, err)
	}
	if observer.calls != 0 {
		t.Fatalf("canceled requests called dependencies %d times", observer.calls)
	}
}
