package validation_test

import (
	"testing"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestValidate_LegacyPetriFactoryWithoutOrchestratorRemainsValid(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Name: "legacy-petri",
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name:   "task",
			States: []interfaces.StateConfig{{Name: "init", Type: interfaces.StateTypeInitial}, {Name: "done", Type: interfaces.StateTypeTerminal}},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "worker", Type: interfaces.WorkerTypeModel}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "execute",
			WorkerTypeName: "worker",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		}},
	}

	if got := interfaces.EffectiveOrchestratorKind(cfg); got != interfaces.OrchestratorKindPetri {
		t.Fatalf("EffectiveOrchestratorKind = %q, want PETRI", got)
	}
	if targets := factoryvalidation.OrchestratorTargets(cfg); len(targets) > 0 {
		t.Fatalf("orchestrator targets = %#v, want none for legacy Petri factory", targets)
	}
	if !factoryvalidation.IsPetriOrchestratorValidationScope(cfg) {
		t.Fatal("expected legacy factory to remain in Petri validation scope")
	}
}

func TestValidate_JavaScriptFactoryAcceptsSourceRefWithoutPetriGraph(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Name: "dynamic-workflow",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
				SourceRef:  "factory/workflows/review.js",
				Entrypoint: "main",
				Metadata:   map[string]string{"team": "platform"},
			},
		},
	}

	result := factoryvalidation.Validate(cfg)
	if result.HasTargets() {
		t.Fatalf("validation targets = %#v, want none for valid JavaScript factory", result.Targets)
	}
}

func TestValidate_JavaScriptFactoryRejectsMissingSource(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Name: "invalid-javascript",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind:       interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{},
		},
	}

	assertValidationCode(t, factoryvalidation.Validate(cfg).Targets, factoryvalidation.CodeOrchestratorJavaScriptMissingSource)
}

func TestValidate_JavaScriptFactoryRejectsPetriGraphFields(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Name: "invalid-javascript",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
				SourceRef: "factory/workflows/review.js",
			},
		},
		WorkTypes: []interfaces.WorkTypeConfig{{Name: "task", States: []interfaces.StateConfig{{Name: "init", Type: interfaces.StateTypeInitial}}}},
	}

	assertValidationCode(t, factoryvalidation.Validate(cfg).Targets, factoryvalidation.CodeOrchestratorIncompatiblePetriField)
}

func TestValidate_UnsupportedOrchestratorKindRejected(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Name: "invalid-kind",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: "STREAM",
		},
	}

	assertValidationCode(t, factoryvalidation.Validate(cfg).Targets, factoryvalidation.CodeOrchestratorUnsupportedKind)
}

func assertValidationCode(t *testing.T, targets []factoryvalidation.Target, code string) {
	t.Helper()
	for _, target := range targets {
		if target.Code == code {
			return
		}
	}
	t.Fatalf("validation targets = %#v, want code %q", targets, code)
}
