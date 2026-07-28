package requiredtools_test

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/requiredtools"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type stubRequiredToolChecker map[string]factorydefinitions.RequiredToolCheckResult

func (s stubRequiredToolChecker) Check(tool factorydefinitions.RequiredToolConfig) factorydefinitions.RequiredToolCheckResult {
	if result, ok := s[tool.Command]; ok {
		return result
	}
	return factorydefinitions.RequiredToolCheckResult{}
}

func factoryWithRequiredTools(
	tools ...factorydefinitions.RequiredToolConfig,
) *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{
		Name: "required-tool-validation",
		ResourceManifest: &factorydefinitions.PortableResourceManifestConfig{
			RequiredTools: tools,
		},
	}
}

func TestValidate_ValidRequiredToolHasNoBlockingTargets(t *testing.T) {
	t.Parallel()

	cfg := factoryWithRequiredTools(factorydefinitions.RequiredToolConfig{
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

	cfg := factoryWithRequiredTools(factorydefinitions.RequiredToolConfig{
		Name:    "Missing helper",
		Command: "missing-tool",
	})
	checker := stubRequiredToolChecker{
		"missing-tool": {
			FailureKind: factorydefinitions.RequiredToolFailureKindMissing,
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
			target.Severity == factorydefinitions.ValidationSeverityError &&
			target.Subject.Type == factorydefinitions.ValidationSubjectTypeFactory &&
			target.Subject.ID == "Missing helper" {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want typed missing required-tool target", result.Targets)
	}
}
