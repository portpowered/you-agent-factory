package requiredtools_test

import (
	"errors"
	"testing"

	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/requiredtools"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
)

type stubRequiredToolChecker map[string]factorycontracts.RequiredToolCheckResult

func (s stubRequiredToolChecker) Check(tool factorycontracts.RequiredToolConfig) factorycontracts.RequiredToolCheckResult {
	if result, ok := s[tool.Command]; ok {
		return result
	}
	return factorycontracts.RequiredToolCheckResult{}
}

func factoryWithRequiredTools(
	tools ...factorycontracts.RequiredToolConfig,
) *factorycontracts.FactoryConfig {
	return &factorycontracts.FactoryConfig{
		Name: "required-tool-validation",
		ResourceManifest: &factorycontracts.PortableResourceManifestConfig{
			RequiredTools: tools,
		},
	}
}

func TestValidate_ValidRequiredToolHasNoBlockingTargets(t *testing.T) {
	t.Parallel()

	cfg := factoryWithRequiredTools(factorycontracts.RequiredToolConfig{
		Name:    "Portable helper",
		Command: "present-tool",
	})
	checker := stubRequiredToolChecker{"present-tool": {}}

	result := requiredtools.Validate(cfg, checker)
	if result.HasBlockingTargets() {
		t.Fatalf("required-tool targets = %#v, want none", result.Targets)
	}
}

func TestValidate_MissingRequiredToolReturnsTypedTarget(t *testing.T) {
	t.Parallel()

	cfg := factoryWithRequiredTools(factorycontracts.RequiredToolConfig{
		Name:    "Missing helper",
		Command: "missing-tool",
	})
	checker := stubRequiredToolChecker{
		"missing-tool": {
			FailureKind: factorycontracts.RequiredToolFailureKindMissing,
			Err:         errors.New(`required tool "Missing helper" command "missing-tool" was not found on PATH`),
		},
	}

	result := requiredtools.Validate(cfg, checker)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking required-tool targets")
	}
	found := false
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeRequiredToolMissing &&
			target.Severity == factorycontracts.ValidationSeverityError &&
			target.Subject.Type == factorycontracts.ValidationSubjectTypeFactory &&
			target.Subject.ID == "Missing helper" {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want typed missing required-tool target", result.Targets)
	}
}
