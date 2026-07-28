package factorydefinitions_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
)

var orchestrationSemanticValidationPackages = []string{
	factoryDefinitionsRoot + "/validation",
	factoryDefinitionsRoot + "/internal/services/validation/internal/orchestrator",
}

// TestOrchestrationSemanticValidationPackagesImportNoRuntimeImplementation seals
// CUT-DEF-RUN story 002: orchestration-specific semantic validation under
// Factory Definitions may reach Factory Runtime only through the injected
// OrchestratorDefinitionValidator port, not nested Runtime implementation paths.
func TestOrchestrationSemanticValidationPackagesImportNoRuntimeImplementation(t *testing.T) {
	t.Parallel()

	for _, pkg := range orchestrationSemanticValidationPackages {
		pkg := pkg
		t.Run(shortFactoryDefinitionsPackageName(pkg), func(t *testing.T) {
			t.Parallel()
			assertProductionImportsUseRuntimeRootOnly(t, pkg)
		})
	}
}

// TestOrchestrationSemanticValidation_DefinitionsOwnedStrategyCheckWithoutRuntimePort
// proves Definitions-owned orchestrator/strategy checks remain on Definitions
// vocabulary and do not require a Runtime-backed validator.
func TestOrchestrationSemanticValidation_DefinitionsOwnedStrategyCheckWithoutRuntimePort(t *testing.T) {
	t.Parallel()

	validator := factoryvalidation.New(nil)
	cfg := &factorycontracts.FactoryConfig{
		Name: "unsupported-orchestrator",
		Orchestrator: &factorycontracts.FactoryOrchestratorConfig{
			Kind: "LEGACY",
		},
	}

	result := validator.Validate(context.Background(), cfg, nil)
	found := false
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeOrchestratorUnsupportedKind &&
			target.Severity == factorycontracts.ValidationSeverityError &&
			target.Subject.Type == factorycontracts.ValidationSubjectTypeFactory &&
			target.Subject.Location == factorycontracts.ValidationSubjectLocationDefinition {
			found = true
		}
	}
	if !found {
		t.Fatalf("validation targets = %#v, want Definitions-owned unsupported orchestrator kind target", result.Targets)
	}
}

// TestOrchestrationSemanticValidation_InvalidOrchestrationReturnsDefinitionsOwnedTargets
// proves orchestration-invalid definitions yield Definitions-owned validation
// targets (code, severity, subject) through the sealed Runtime semantic-validation
// port without Definitions importing Runtime implementation packages.
func TestOrchestrationSemanticValidation_InvalidOrchestrationReturnsDefinitionsOwnedTargets(t *testing.T) {
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
		if target.Code == "workflow.source.syntaxError" &&
			target.Severity == factorycontracts.ValidationSeverityError &&
			target.Subject.Type == factorycontracts.ValidationSubjectTypeFactory &&
			target.Subject.ID == "factory" &&
			target.Subject.Location == factorycontracts.ValidationSubjectLocationDefinition &&
			target.Path == "factory.orchestrator.javascript.inlineSource" {
			found = true
		}
	}
	if !found {
		t.Fatalf("validation targets = %#v, want orchestration-invalid runtime semantic target", result.Targets)
	}
}

// TestOrchestrationSemanticValidation_ValidOrchestrationProducesNoRuntimeTargets
// proves equivalent orchestration-valid inputs still produce no runtime-owned
// semantic validation findings when the sealed port reports success.
func TestOrchestrationSemanticValidation_ValidOrchestrationProducesNoRuntimeTargets(t *testing.T) {
	t.Parallel()

	cfg := validJavaScriptOrchestratorConfig()
	validator := factoryvalidation.New(runtimeSemanticValidationStub{})

	result := validator.Validate(context.Background(), cfg, nil)
	for _, target := range result.Targets {
		if strings.HasPrefix(target.Code, "workflow.") ||
			strings.HasPrefix(target.Code, "JAVASCRIPT_") {
			t.Fatalf("validation targets = %#v, want no runtime semantic validation findings", result.Targets)
		}
	}
}

// TestOrchestrationSemanticValidationPortContractUsesDefinitionsVocabularyOnly
// proves the OrchestratorDefinitionValidator port surface stays on
// Definitions-owned contracts and does not pull Runtime implementation types
// into the orchestration semantic validation packages.
func TestOrchestrationSemanticValidationPortContractUsesDefinitionsVocabularyOnly(t *testing.T) {
	t.Parallel()

	cmd := exec.Command(
		"go",
		"list",
		"-f",
		"{{join .Imports \"\\n\"}}",
		factoryDefinitionsRoot+"/contracts",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for contracts: %v\n%s", err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		if isForbiddenFactoryDefinitionsRuntimeImport(importPath) {
			t.Fatalf(
				"contracts import %s is forbidden on OrchestratorDefinitionValidator port; use Definitions vocabulary only",
				importPath,
			)
		}
	}
}

func validJavaScriptOrchestratorConfig() *factorycontracts.FactoryConfig {
	return &factorycontracts.FactoryConfig{
		Name: "javascript-orchestrator",
		Orchestrator: &factorycontracts.FactoryOrchestratorConfig{
			Kind: factorycontracts.OrchestratorKindJavaScript,
			JavaScript: &factorycontracts.FactoryOrchestratorJavaScriptConfig{
				SourceRef:  "factory/workflows/review.js",
				Entrypoint: "main",
			},
		},
	}
}

type runtimeSemanticValidationStub struct {
	targets []factorycontracts.ValidationTarget
}

func (s runtimeSemanticValidationStub) ValidateJavaScriptFactoryDefinition(
	_ context.Context,
	_ *factorycontracts.FactoryOrchestratorJavaScriptConfig,
	_ factorycontracts.WorkflowSourceReader,
) []factorycontracts.ValidationTarget {
	return append([]factorycontracts.ValidationTarget(nil), s.targets...)
}
