package validation_test

import (
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/validationassert"
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

func TestValidate_JavaScriptFactoryRejectsEmptyNamedAgentPreset(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Name: "invalid-named-agent",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
				SourceRef: "factory/workflows/review.js",
				Agents: map[string]interfaces.FactoryOrchestratorJavaScriptAgent{
					"reviewer": {Preset: "   "},
				},
			},
		},
	}

	result := factoryvalidation.Validate(cfg)
	assertValidationCode(t, result.Targets, factoryvalidation.CodeOrchestratorJavaScriptInvalidAgent)
	if !strings.Contains(result.Targets[0].Message, "reviewer") {
		t.Fatalf("diagnostic = %q, want agent id", result.Targets[0].Message)
	}
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

func workPropagationValidationBaseConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:   "execute-story",
			Inputs: []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			Outputs: []interfaces.IOConfig{
				{WorkTypeName: "story", StateName: "complete"},
			},
			OnFailure: []interfaces.IOConfig{{WorkTypeName: "story", StateName: "failed"}},
		}},
	}
}

func TestValidate_WorkPropagationSupportedModes(t *testing.T) {
	t.Parallel()

	for _, generatedMode := range []factoryapi.WorkPropagationMode{
		factoryapi.WorkPropagationModeOutputAsPayload,
		factoryapi.WorkPropagationModePreserveInput,
	} {
		mode := interfaces.WorkPropagationMode(generatedMode)
		t.Run(string(mode), func(t *testing.T) {
			cfg := workPropagationValidationBaseConfig()
			cfg.Workstations[0].WorkPropagation = &interfaces.WorkPropagationConfig{Mode: mode}

			result := factoryvalidation.Validate(cfg)
			for _, target := range result.Targets {
				if target.Code == factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode {
					t.Fatalf("targets = %#v, want supported mode %q to stay valid", result.Targets, mode)
				}
			}
		})
	}
}

func TestValidate_WorkPropagationOmitted(t *testing.T) {
	t.Parallel()

	result := factoryvalidation.Validate(workPropagationValidationBaseConfig())
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode {
			t.Fatalf("targets = %#v, want omitted workPropagation to stay valid", result.Targets)
		}
	}
}

func TestValidate_WorkPropagationUnsupportedMode(t *testing.T) {
	t.Parallel()

	cfg := workPropagationValidationBaseConfig()
	cfg.Workstations[0].WorkPropagation = &interfaces.WorkPropagationConfig{
		Mode: interfaces.WorkPropagationMode("MERGE_PAYLOAD"),
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode)

	var matched bool
	for _, target := range result.Targets {
		if target.Code != factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode {
			continue
		}
		matched = true
		if target.Subject.Type != factoryvalidation.SubjectTypeWorkstation || target.Subject.ID != "execute-story" {
			t.Fatalf("target subject = %#v, want workstation execute-story", target.Subject)
		}
		if target.Path != "factory.workstations[0](execute-story).workPropagation.mode" {
			t.Fatalf("target path = %q, want workstation workPropagation path", target.Path)
		}
		if !strings.Contains(target.Message, `unsupported workPropagation.mode "MERGE_PAYLOAD"`) {
			t.Fatalf("target message = %q, want unsupported mode detail", target.Message)
		}
	}
	if !matched {
		t.Fatal("expected unsupported work propagation target")
	}
}

func TestValidate_WorkPropagationEmptyMode(t *testing.T) {
	t.Parallel()

	cfg := workPropagationValidationBaseConfig()
	cfg.Workstations[0].WorkPropagation = &interfaces.WorkPropagationConfig{}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode)
}

func TestValidate_WorkPropagationUnsupportedModeUsesGeneratedEnumValues(t *testing.T) {
	t.Parallel()

	cfg := workPropagationValidationBaseConfig()
	cfg.Workstations[0].WorkPropagation = &interfaces.WorkPropagationConfig{
		Mode: interfaces.WorkPropagationMode(factoryapi.WorkPropagationMode("PRESERVE_OUTPUT")),
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode)
}
