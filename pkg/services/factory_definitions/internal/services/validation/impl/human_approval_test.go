package impl

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestValidateHumanApprovalWorkstation(t *testing.T) {
	cfg := humanApprovalTestFactory()

	result := Validate(cfg)
	if len(result.Targets) != 0 {
		t.Fatalf("valid HUMAN_APPROVAL definition produced validation targets: %+v", result.Targets)
	}
}

func TestValidateHumanApprovalWorkstationReportsFieldSpecificDiagnostics(t *testing.T) {
	cfg := humanApprovalTestFactory()
	workstation := &cfg.Workstations[0]
	workstation.ID = ""
	workstation.Description = nil
	workstation.Inputs = nil
	workstation.Outputs = nil
	workstation.OnRejection = nil
	workstation.WorkerTypeName = "fake-worker"
	workstation.Runner = "codex"
	workstation.Operation = "TTS"
	workstation.PromptFile = "prompt.md"
	workstation.OutputSchema = `{ "type": "object" }`
	workstation.Timeout = "1m"
	workstation.Env = map[string]string{"SECRET": "value"}
	workstation.Worktree = "worktree"
	workstation.Resources = []factorydefinitions.ResourceConfig{{Name: "slot", Capacity: 1}}
	workstation.ClassificationRoutes = []factorydefinitions.ClassificationRouteConfig{{Label: "approve"}}
	workstation.OnContinue = []factorydefinitions.IOConfig{{WorkTypeName: "review", StateName: "pending"}}
	workstation.OnFailure = []factorydefinitions.IOConfig{{WorkTypeName: "review", StateName: "failed"}}

	targets := humanApprovalWorkstationTargets(cfg)
	wantPaths := []string{
		"factory.workstations[0](review).id",
		"factory.workstations[0](review).description",
		"factory.workstations[0](review).inputs",
		"factory.workstations[0](review).outputs",
		"factory.workstations[0](review).onRejection",
		"factory.workstations[0](review).worker",
		"factory.workstations[0](review).runner",
		"factory.workstations[0](review).operation",
		"factory.workstations[0](review).promptFile",
		"factory.workstations[0](review).outputSchema",
		"factory.workstations[0](review).timeout",
		"factory.workstations[0](review).env",
		"factory.workstations[0](review).worktree",
		"factory.workstations[0](review).resources",
		"factory.workstations[0](review).classification_routes",
		"factory.workstations[0](review).onContinue",
		"factory.workstations[0](review).onFailure",
	}
	got := make(map[string]bool, len(targets))
	for _, target := range targets {
		got[target.Path] = true
	}
	for _, path := range wantPaths {
		if !got[path] {
			t.Errorf("missing field-specific HUMAN_APPROVAL diagnostic for %q", path)
		}
	}
}

func TestValidateHumanApprovalWorkstationRejectsJavaScriptFactory(t *testing.T) {
	cfg := humanApprovalTestFactory()
	cfg.Orchestrator = &factorydefinitions.FactoryOrchestratorConfig{
		Kind: factorydefinitions.OrchestratorKindJavaScript,
		JavaScript: &factorydefinitions.FactoryOrchestratorJavaScriptConfig{
			InlineSource: &factorydefinitions.FactoryOrchestratorJavaScriptInlineSource{Inline: "export default {}"},
		},
	}

	targets := humanApprovalWorkstationTargets(cfg)
	for _, target := range targets {
		if target.Path == "factory.workstations[0](review).type" {
			return
		}
	}
	t.Fatal("expected JavaScript-specific HUMAN_APPROVAL diagnostic")
}

func humanApprovalTestFactory() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{
		Name: "approval-factory",
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name: "review",
				States: []factorydefinitions.StateConfig{
					{Name: "pending", Type: factorydefinitions.StateTypeInitial},
					{Name: "approved", Type: factorydefinitions.StateTypeTerminal},
					{Name: "rejected", Type: factorydefinitions.StateTypeProcessing},
					{Name: "failed", Type: factorydefinitions.StateTypeFailed},
				},
			},
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				ID:   "review-approval",
				Name: "review",
				Type: factorydefinitions.WorkstationTypeHumanApproval,
				Description: &factorydefinitions.NameValueConfig{
					Type:  factorydefinitions.NameValueTypeLocalizableAsset,
					Value: "A human reviews the work.",
				},
				Inputs:  []factorydefinitions.IOConfig{{WorkTypeName: "review", StateName: "pending"}},
				Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "review", StateName: "approved"}},
				OnRejection: []factorydefinitions.IOConfig{{
					WorkTypeName: "review",
					StateName:    "rejected",
				}},
			},
		},
	}
}
