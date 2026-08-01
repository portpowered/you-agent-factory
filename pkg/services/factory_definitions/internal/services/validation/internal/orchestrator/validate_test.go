package orchestrator_test

import (
	"context"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/impl"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/orchestrator"
)

type stubOrchestratorValidator struct {
	targets []factorydefinitions.ValidationTarget
}

func (s stubOrchestratorValidator) ValidateJavaScriptFactoryDefinition(
	_ context.Context,
	_ *factorydefinitions.FactoryOrchestratorJavaScriptConfig,
	_ factorydefinitions.WorkflowSourceReader,
) []factorydefinitions.ValidationTarget {
	return append([]factorydefinitions.ValidationTarget(nil), s.targets...)
}

func TestValidate_UnsupportedOrchestratorKindReturnsTypedTarget(t *testing.T) {
	t.Parallel()

	cfg := &factorydefinitions.FactoryConfig{
		Name: "orchestrator-validation",
		Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
			Kind: "LEGACY",
		},
	}

	result := orchestrator.Validate(context.Background(), cfg, nil, nil)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking orchestrator targets")
	}
	found := false
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeOrchestratorUnsupportedKind &&
			target.Severity == factorydefinitions.ValidationSeverityError &&
			target.Subject.Type == factorydefinitions.ValidationSubjectTypeFactory {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want unsupported orchestrator kind target", result.Targets)
	}
}

func TestValidate_JavaScriptMissingSourceReturnsTypedTarget(t *testing.T) {
	t.Parallel()

	cfg := &factorydefinitions.FactoryConfig{
		Name: "javascript-orchestrator",
		Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
			Kind:       factorydefinitions.OrchestratorKindJavaScript,
			JavaScript: &factorydefinitions.FactoryOrchestratorJavaScriptConfig{},
		},
	}

	result := orchestrator.Validate(context.Background(), cfg, nil, nil)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking orchestrator targets")
	}
	found := false
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeOrchestratorJavaScriptMissingSource &&
			target.Severity == factorydefinitions.ValidationSeverityError &&
			target.Subject.Type == factorydefinitions.ValidationSubjectTypeFactory {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want JavaScript missing source orchestrator target", result.Targets)
	}
}

func TestValidate_RuntimeValidatorTargetsAreMerged(t *testing.T) {
	t.Parallel()

	cfg := &factorydefinitions.FactoryConfig{
		Name: "javascript-orchestrator",
		Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
			Kind: factorydefinitions.OrchestratorKindJavaScript,
			JavaScript: &factorydefinitions.FactoryOrchestratorJavaScriptConfig{
				SourceRef:  "factory/workflows/review.js",
				Entrypoint: "main",
			},
		},
	}
	validator := stubOrchestratorValidator{targets: []factorydefinitions.ValidationTarget{{
		Code:     "factory.orchestrator.javascript.invalidPolicy",
		Severity: factorydefinitions.ValidationSeverityError,
		Message:  "invalid default policy",
		Subject: factorydefinitions.ValidationSubject{
			Type:     factorydefinitions.ValidationSubjectTypeFactory,
			ID:       "factory",
			Location: factorydefinitions.ValidationSubjectLocationDefinition,
		},
		Path: "factory.orchestrator.javascript.defaultPolicy",
	}}}

	result := orchestrator.Validate(context.Background(), cfg, validator, nil)
	found := false
	for _, target := range result.Targets {
		if target.Code == "factory.orchestrator.javascript.invalidPolicy" {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want runtime validator target", result.Targets)
	}
}
