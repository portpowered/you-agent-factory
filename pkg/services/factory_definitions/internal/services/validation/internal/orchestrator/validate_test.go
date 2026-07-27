package orchestrator_test

import (
	"context"
	"testing"

	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/internal/orchestrator"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
)

type stubOrchestratorValidator struct {
	targets []factorycontracts.ValidationTarget
}

func (s stubOrchestratorValidator) ValidateJavaScriptFactoryDefinition(
	_ context.Context,
	_ *factorycontracts.FactoryOrchestratorJavaScriptConfig,
	_ factorycontracts.WorkflowSourceReader,
) []factorycontracts.ValidationTarget {
	return append([]factorycontracts.ValidationTarget(nil), s.targets...)
}

func TestValidate_UnsupportedOrchestratorKindReturnsTypedTarget(t *testing.T) {
	t.Parallel()

	cfg := &factorycontracts.FactoryConfig{
		Name: "orchestrator-validation",
		Orchestrator: &factorycontracts.FactoryOrchestratorConfig{
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
			target.Severity == factorycontracts.ValidationSeverityError &&
			target.Subject.Type == factorycontracts.ValidationSubjectTypeFactory {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want unsupported orchestrator kind target", result.Targets)
	}
}

func TestValidate_JavaScriptMissingSourceReturnsTypedTarget(t *testing.T) {
	t.Parallel()

	cfg := &factorycontracts.FactoryConfig{
		Name: "javascript-orchestrator",
		Orchestrator: &factorycontracts.FactoryOrchestratorConfig{
			Kind:       factorycontracts.OrchestratorKindJavaScript,
			JavaScript: &factorycontracts.FactoryOrchestratorJavaScriptConfig{},
		},
	}

	result := orchestrator.Validate(context.Background(), cfg, nil, nil)
	if !result.HasBlockingTargets() {
		t.Fatal("expected blocking orchestrator targets")
	}
	found := false
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeOrchestratorJavaScriptMissingSource &&
			target.Severity == factorycontracts.ValidationSeverityError &&
			target.Subject.Type == factorycontracts.ValidationSubjectTypeFactory {
			found = true
		}
	}
	if !found {
		t.Fatalf("targets = %#v, want JavaScript missing source orchestrator target", result.Targets)
	}
}

func TestValidate_RuntimeValidatorTargetsAreMerged(t *testing.T) {
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
	validator := stubOrchestratorValidator{targets: []factorycontracts.ValidationTarget{{
		Code:     "factory.orchestrator.javascript.invalidPolicy",
		Severity: factorycontracts.ValidationSeverityError,
		Message:  "invalid default policy",
		Subject: factorycontracts.ValidationSubject{
			Type:     factorycontracts.ValidationSubjectTypeFactory,
			ID:       "factory",
			Location: factorycontracts.ValidationSubjectLocationDefinition,
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
