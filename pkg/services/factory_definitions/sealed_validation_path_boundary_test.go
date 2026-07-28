package factorydefinitions_test

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
)

var sealedValidationPathPackages = []string{
	factoryDefinitionsRoot + "/validation",
}

// TestSealedValidationPathPackagesImportNoRuntimeImplementation seals CUT-DEF-RUN
// story 004: the public validation service may reach Factory Runtime only through
// the injected OrchestratorDefinitionValidator port, not nested Runtime paths.
func TestSealedValidationPathPackagesImportNoRuntimeImplementation(t *testing.T) {
	t.Parallel()

	for _, pkg := range sealedValidationPathPackages {
		pkg := pkg
		t.Run(shortFactoryDefinitionsPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertProductionImportsUseRuntimeRootOnly(t, pkg)
		})
	}
}

// TestSealedValidationPath_InvalidTopologyReturnsTypedTarget proves an
// invalid-topology case through the public validation service boundary with
// observable Definitions-owned code, severity, and path fields.
func TestSealedValidationPath_InvalidTopologyReturnsTypedTarget(t *testing.T) {
	t.Parallel()

	cfg := sealedValidationPathPetriFactoryConfig()
	cfg.Workstations[0].Outputs = []factorydefinitions.IOConfig{{
		WorkTypeName: "task",
		StateName:    "bogus",
	}}

	validator := factoryvalidation.New(nil)
	result := validator.ValidateTopology(context.Background(), cfg, nil)
	if !result.HasErrors() {
		t.Fatal("expected blocking topology findings")
	}

	found := false
	for _, finding := range result.Findings {
		if finding.Rule == factoryvalidation.CodeDanglingPlaceReference &&
			finding.Severity == factorydefinitions.ValidationSeverityError &&
			finding.Path != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("topology findings = %#v, want typed dangling place topology finding", result.Findings)
	}
}

// TestSealedValidationPath_InvalidRequiredToolReturnsTypedTarget proves an
// invalid/missing required-tool case through the public validation service
// boundary with observable Definitions-owned rule, severity, and path fields.
func TestSealedValidationPath_InvalidRequiredToolReturnsTypedTarget(t *testing.T) {
	t.Parallel()

	cfg := sealedValidationPathPetriFactoryConfig()
	cfg.ResourceManifest = &factorydefinitions.PortableResourceManifestConfig{
		RequiredTools: []factorydefinitions.RequiredToolConfig{{
			Name:    "Missing helper",
			Command: "missing-tool",
		}},
	}
	checker := sealedValidationPathRequiredToolChecker{
		"missing-tool": {
			FailureKind: factorydefinitions.RequiredToolFailureKindMissing,
			Err:         errors.New(`required tool "Missing helper" command "missing-tool" was not found on PATH`),
		},
	}

	validator := factoryvalidation.New(nil)
	result := validator.ValidateTopology(context.Background(), cfg, checker)
	if !result.HasErrors() {
		t.Fatal("expected blocking required-tool findings")
	}

	found := false
	for _, finding := range result.Findings {
		if finding.Rule == "required-tool-missing" &&
			finding.Severity == factorydefinitions.ValidationSeverityError &&
			finding.Path == "resourceManifest.requiredTools[0].command" {
			found = true
		}
	}
	if !found {
		t.Fatalf("topology findings = %#v, want typed missing required-tool finding", result.Findings)
	}
}

// TestSealedValidationPath_InvalidOrchestratorStrategyReturnsTypedTarget proves
// an invalid orchestrator/strategy case through the public validation service
// boundary with observable Definitions-owned code, severity, and subject fields.
func TestSealedValidationPath_InvalidOrchestratorStrategyReturnsTypedTarget(t *testing.T) {
	t.Parallel()

	cfg := sealedValidationPathPetriFactoryConfig()
	cfg.Orchestrator = &factorydefinitions.FactoryOrchestratorConfig{Kind: "LEGACY"}

	validator := factoryvalidation.New(nil)
	result := validator.Validate(context.Background(), cfg, nil)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking orchestrator strategy targets")
	}

	found := false
	for _, target := range result.Targets {
		assertValidationTargetUsesDefinitionsVocabulary(t, target)
		if target.Code == factoryvalidation.CodeOrchestratorUnsupportedKind &&
			target.Severity == factorydefinitions.ValidationSeverityError &&
			target.Subject.Type == factorydefinitions.ValidationSubjectTypeFactory &&
			target.Subject.Location == factorydefinitions.ValidationSubjectLocationDefinition {
			found = true
		}
	}
	if !found {
		t.Fatalf("validation targets = %#v, want typed unsupported orchestrator strategy target", result.Targets)
	}
}

// TestSealedValidationPath_RuntimeSemanticValidationThroughSealedPort proves
// orchestration-invalid semantic validation still flows through the injected
// Runtime root port on the public validation service without Definitions
// importing Runtime implementation packages.
func TestSealedValidationPath_RuntimeSemanticValidationThroughSealedPort(t *testing.T) {
	t.Parallel()

	cfg := validJavaScriptOrchestratorConfig()
	validator := factoryvalidation.New(runtimeSemanticValidationStub{targets: []factorydefinitions.ValidationTarget{{
		Code:     "workflow.source.syntaxError",
		Severity: factorydefinitions.ValidationSeverityError,
		Message:  "unexpected token",
		Path:     "factory.orchestrator.javascript.inlineSource",
		Subject: factorydefinitions.ValidationSubject{
			Type:     factorydefinitions.ValidationSubjectTypeFactory,
			ID:       "factory",
			Location: factorydefinitions.ValidationSubjectLocationDefinition,
		},
	}}})

	result := validator.Validate(context.Background(), cfg, nil)
	found := false
	for _, target := range result.Targets {
		assertValidationTargetUsesDefinitionsVocabulary(t, target)
		if target.Code == "workflow.source.syntaxError" &&
			target.Severity == factorydefinitions.ValidationSeverityError &&
			target.Subject.Type == factorydefinitions.ValidationSubjectTypeFactory &&
			target.Path == "factory.orchestrator.javascript.inlineSource" {
			found = true
		}
	}
	if !found {
		t.Fatalf("validation targets = %#v, want orchestration-invalid runtime semantic target through sealed port", result.Targets)
	}
}

func sealedValidationPathPetriFactoryConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{
		Name: "sealed-validation-path",
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "done", Type: factorydefinitions.StateTypeTerminal},
				{Name: "failed", Type: factorydefinitions.StateTypeFailed},
			},
		}},
		Workers: []workerconfig.Config{{Name: "worker-a"}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name:           "process",
			WorkerTypeName: "worker-a",
			Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
			OnFailure:      []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
		}},
	}
}

type sealedValidationPathRequiredToolChecker map[string]factorydefinitions.RequiredToolCheckResult

func (s sealedValidationPathRequiredToolChecker) Check(
	tool factorydefinitions.RequiredToolConfig,
) factorydefinitions.RequiredToolCheckResult {
	if result, ok := s[tool.Command]; ok {
		return result
	}
	return factorydefinitions.RequiredToolCheckResult{}
}
