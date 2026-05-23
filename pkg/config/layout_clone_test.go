package config

import (
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestFactoryConfig_SharedSurfaceRetiresExhaustionRules(t *testing.T) {
	factoryType := reflect.TypeOf(interfaces.FactoryConfig{})
	if _, ok := factoryType.FieldByName("ExhaustionRules"); ok {
		t.Fatal("interfaces.FactoryConfig must not expose ExhaustionRules")
	}
}

func TestCloneFactoryConfig_PreservesGuardedLogicalMoveLoopBreakerWorkstations(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "review", Type: interfaces.StateTypeProcessing},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "review-story",
				WorkerTypeName: "reviewer",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "review"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "review"}},
				OnRejection:    []interfaces.IOConfig{{WorkTypeName: "story", StateName: "review"}},
			},
			{
				Name:    "review-loop-breaker",
				Type:    interfaces.WorkstationTypeLogical,
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "story", StateName: "review"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "story", StateName: "failed"}},
				Guards: []interfaces.GuardConfig{{
					Type:        interfaces.GuardTypeVisitCount,
					Workstation: "review-story",
					MaxVisits:   3,
				}},
			},
		},
	}

	cloned, err := CloneFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("CloneFactoryConfig: %v", err)
	}
	if len(cloned.Workstations) != 2 {
		t.Fatalf("expected 2 workstations, got %#v", cloned.Workstations)
	}
	loopBreaker := cloned.Workstations[1]
	if loopBreaker.Name != "review-loop-breaker" || loopBreaker.Type != interfaces.WorkstationTypeLogical {
		t.Fatalf("expected guarded logical move loop breaker to be preserved, got %#v", loopBreaker)
	}
	if len(loopBreaker.Guards) != 1 {
		t.Fatalf("expected one loop-breaker guard, got %#v", loopBreaker.Guards)
	}
	if guard := loopBreaker.Guards[0]; guard.Type != interfaces.GuardTypeVisitCount || guard.Workstation != "review-story" || guard.MaxVisits != 3 {
		t.Fatalf("expected visit_count guard details to survive clone, got %#v", guard)
	}

	cfg.Workstations[1].Guards[0].Workstation = "mutated"
	if cloned.Workstations[1].Guards[0].Workstation != "review-story" {
		t.Fatalf("expected cloned guard to be independent of source mutations, got %#v", cloned.Workstations[1].Guards[0])
	}
}

func TestCloneFactoryConfig_ClonesMatchesFieldsGuardMatchConfig(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "match-assets",
			Guards: []interfaces.GuardConfig{{
				Type:        interfaces.GuardTypeMatchesFields,
				MatchConfig: &interfaces.GuardMatchConfig{InputKey: ".Name"},
			}},
		}},
	}

	cloned, err := CloneFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("CloneFactoryConfig: %v", err)
	}
	cfg.Workstations[0].Guards[0].MatchConfig.InputKey = `.Tags["_last_output"]`
	if cloned.Workstations[0].Guards[0].MatchConfig == nil || cloned.Workstations[0].Guards[0].MatchConfig.InputKey != ".Name" {
		t.Fatalf("expected cloned matchConfig to be independent of source mutations, got %#v", cloned.Workstations[0].Guards[0].MatchConfig)
	}
}

func TestCloneFactoryConfig_PreservesFactoryGuards(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Guards: []interfaces.FactoryGuardConfig{{
			Type:          interfaces.GuardTypeInferenceThrottle,
			ModelProvider: "claude",
			Model:         "claude-sonnet",
			RefreshWindow: "15m",
		}},
	}

	cloned, err := CloneFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("CloneFactoryConfig: %v", err)
	}
	if len(cloned.Guards) != 1 {
		t.Fatalf("expected cloned factory guard, got %#v", cloned.Guards)
	}
	if cloned.Guards[0] != cfg.Guards[0] {
		t.Fatalf("cloned factory guard = %#v, want %#v", cloned.Guards[0], cfg.Guards[0])
	}

	cfg.Guards[0].ModelProvider = "codex"
	if cloned.Guards[0].ModelProvider != "claude" {
		t.Fatalf("expected cloned factory guard to be independent of source mutations, got %#v", cloned.Guards[0])
	}
}

func TestCloneFactoryConfig_ClonesModelOperationBindingWorkContent(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:      "writer",
			Operation: "MODEL_INVOKE",
			OperationBindings: []interfaces.ModelOperationBinding{{
				Slot: "draft",
				Config: []interfaces.WorkContentPart{{
					Type: interfaces.WorkContentPartTypeText,
					Text: "configured prompt",
				}},
				DefaultContent: []interfaces.WorkContentPart{{
					Type: interfaces.WorkContentPartTypeText,
					Text: "fallback prompt",
				}},
			}, {
				Slot:           "empty",
				Config:         []interfaces.WorkContentPart{},
				DefaultContent: []interfaces.WorkContentPart{},
			}},
		}},
	}

	cloned, err := CloneFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("CloneFactoryConfig: %v", err)
	}
	bindings := cloned.Workstations[0].OperationBindings
	if len(bindings) != 2 {
		t.Fatalf("cloned bindings = %#v, want two bindings", bindings)
	}
	if bindings[0].Config == nil || len(bindings[0].Config) != 1 || bindings[0].Config[0].Text != "configured prompt" {
		t.Fatalf("cloned config content = %#v, want one detached configured part", bindings[0].Config)
	}
	if bindings[0].DefaultContent == nil || len(bindings[0].DefaultContent) != 1 || bindings[0].DefaultContent[0].Text != "fallback prompt" {
		t.Fatalf("cloned default content = %#v, want one detached default part", bindings[0].DefaultContent)
	}
	if bindings[1].Config != nil {
		t.Fatalf("empty config content = %#v, want nil", bindings[1].Config)
	}
	if bindings[1].DefaultContent != nil {
		t.Fatalf("empty default content = %#v, want nil", bindings[1].DefaultContent)
	}

	cfg.Workstations[0].OperationBindings[0].Config[0].Text = "mutated config"
	cfg.Workstations[0].OperationBindings[0].DefaultContent[0].Text = "mutated default"
	if bindings[0].Config[0].Text != "configured prompt" {
		t.Fatalf("cloned config content changed after source mutation: %#v", bindings[0].Config)
	}
	if bindings[0].DefaultContent[0].Text != "fallback prompt" {
		t.Fatalf("cloned default content changed after source mutation: %#v", bindings[0].DefaultContent)
	}
}
