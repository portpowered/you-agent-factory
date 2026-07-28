package factorydefinitions_test

import (
	"context"
	"errors"
	"testing"

	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
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
	cfg.Workstations[0].Outputs = []factorycontracts.IOConfig{{
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
			finding.Severity == factorycontracts.ValidationSeverityError &&
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
	cfg.ResourceManifest = &factorycontracts.PortableResourceManifestConfig{
		RequiredTools: []factorycontracts.RequiredToolConfig{{
			Name:    "Missing helper",
			Command: "missing-tool",
		}},
	}
	checker := sealedValidationPathRequiredToolChecker{
		"missing-tool": {
			FailureKind: factorycontracts.RequiredToolFailureKindMissing,
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
			finding.Severity == factorycontracts.ValidationSeverityError &&
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
	cfg.Orchestrator = &factorycontracts.FactoryOrchestratorConfig{Kind: "LEGACY"}

	validator := factoryvalidation.New(nil)
	result := validator.Validate(context.Background(), cfg, nil)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking orchestrator strategy targets")
	}

	found := false
	for _, target := range result.Targets {
		assertValidationTargetUsesDefinitionsVocabulary(t, target)
		if target.Code == factoryvalidation.CodeOrchestratorUnsupportedKind &&
			target.Severity == factorycontracts.ValidationSeverityError &&
			target.Subject.Type == factorycontracts.ValidationSubjectTypeFactory &&
			target.Subject.Location == factorycontracts.ValidationSubjectLocationDefinition {
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
	validator := factoryvalidation.New(runtimeSemanticValidationStub{targets: []factorycontracts.ValidationTarget{{
		Code:     "workflow.source.syntaxError",
		Severity: factorycontracts.ValidationSeverityError,
		Message:  "unexpected token",
		Path:     "factory.orchestrator.javascript.inlineSource",
		Subject: factorycontracts.ValidationSubject{
			Type:     factorycontracts.ValidationSubjectTypeFactory,
			ID:       "factory",
			Location: factorycontracts.ValidationSubjectLocationDefinition,
		},
	}}})

	result := validator.Validate(context.Background(), cfg, nil)
	found := false
	for _, target := range result.Targets {
		assertValidationTargetUsesDefinitionsVocabulary(t, target)
		if target.Code == "workflow.source.syntaxError" &&
			target.Severity == factorycontracts.ValidationSeverityError &&
			target.Subject.Type == factorycontracts.ValidationSubjectTypeFactory &&
			target.Path == "factory.orchestrator.javascript.inlineSource" {
			found = true
		}
	}
	if !found {
		t.Fatalf("validation targets = %#v, want orchestration-invalid runtime semantic target through sealed port", result.Targets)
	}
}

func sealedValidationPathPetriFactoryConfig() *factorycontracts.FactoryConfig {
	return &factorycontracts.FactoryConfig{
		Name: "sealed-validation-path",
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

type sealedValidationPathRequiredToolChecker map[string]factorycontracts.RequiredToolCheckResult

func (s sealedValidationPathRequiredToolChecker) Check(
	tool factorycontracts.RequiredToolConfig,
) factorycontracts.RequiredToolCheckResult {
	if result, ok := s[tool.Command]; ok {
		return result
	}
	return factorycontracts.RequiredToolCheckResult{}
}
