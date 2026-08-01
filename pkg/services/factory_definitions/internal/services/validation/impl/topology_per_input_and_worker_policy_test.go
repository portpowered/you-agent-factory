package impl

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestRulePerInputGuards_MissingParentInput(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{{
			WorkTypeName: "task", StateName: "init",
			Guard: &factorydefinitions.InputGuardConfig{Type: factorydefinitions.GuardTypeAllChildrenComplete},
		}},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-parent-input")
}

func TestRulePerInputGuards_ParentInputNotMatching(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{{
			WorkTypeName: "task", StateName: "init",
			Guard: &factorydefinitions.InputGuardConfig{Type: factorydefinitions.GuardTypeAllChildrenComplete, ParentInput: "other"},
		}},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-parent-input")
}

func TestRulePerInputGuards_SelfReference(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{{
			WorkTypeName: "task", StateName: "init",
			Guard: &factorydefinitions.InputGuardConfig{Type: factorydefinitions.GuardTypeAllChildrenComplete, ParentInput: "task"},
		}},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-self-ref")
}

func TestRulePerInputGuards_InvalidSpawnedBy(t *testing.T) {
	cfg := testBaseConfig()
	cfg.WorkTypes = append(cfg.WorkTypes, factorydefinitions.WorkTypeConfig{
		Name: "parent", States: []factorydefinitions.StateConfig{{Name: "init", Type: factorydefinitions.StateTypeInitial}},
	})
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "parent", StateName: "init"},
			{
				WorkTypeName: "task", StateName: "init",
				Guard: &factorydefinitions.InputGuardConfig{Type: factorydefinitions.GuardTypeAllChildrenComplete, ParentInput: "parent", SpawnedBy: "nonexistent"},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-spawned-by")
}

func TestRulePerInputGuards_UnsupportedType(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{{
			WorkTypeName: "task", StateName: "init",
			Guard: &factorydefinitions.InputGuardConfig{Type: factorydefinitions.GuardTypeVisitCount},
		}},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-type")
}

func TestRulePerInputGuards_SameNameMissingMatchInput(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard:        &factorydefinitions.InputGuardConfig{Type: factorydefinitions.GuardTypeSameName},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-match-input")
}

func TestRulePerInputGuards_SameNameMatchInputNotMatching(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:       factorydefinitions.GuardTypeSameName,
					MatchInput: "other",
				},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-match-input")
}

func TestRulePerInputGuards_SameNameSelfReference(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:       factorydefinitions.GuardTypeSameName,
					MatchInput: "task",
				},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-self-ref")
}

func TestRulePerInputGuards_ValidSameNameGuard(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:       factorydefinitions.GuardTypeSameName,
					MatchInput: "plan",
				},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestRulePerInputGuards_SameTraceIDMissingMatchInput(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard:        &factorydefinitions.InputGuardConfig{Type: factorydefinitions.GuardTypeSameTraceID},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-same-trace-match-input")
}

func TestRulePerInputGuards_SameTraceIDMatchInputNotMatching(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:       factorydefinitions.GuardTypeSameTraceID,
					MatchInput: "other",
				},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-same-trace-match-input")
}

func TestRulePerInputGuards_SameTraceIDSelfReference(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:       factorydefinitions.GuardTypeSameTraceID,
					MatchInput: "task",
				},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-same-trace-self-ref")
}

func TestRulePerInputGuards_ValidSameTraceIDGuard(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:       factorydefinitions.GuardTypeSameTraceID,
					MatchInput: "plan",
				},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestRulePerInputGuards_ValidGuard(t *testing.T) {
	cfg := testBaseConfig()
	cfg.WorkTypes = append(cfg.WorkTypes, factorydefinitions.WorkTypeConfig{
		Name: "parent", States: []factorydefinitions.StateConfig{{Name: "init", Type: factorydefinitions.StateTypeInitial}},
	})
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{
		{Name: "spawner"},
		{
			Name: "ws",
			Inputs: []factorydefinitions.IOConfig{
				{WorkTypeName: "parent", StateName: "init"},
				{
					WorkTypeName: "task", StateName: "init",
					Guard: &factorydefinitions.InputGuardConfig{Type: factorydefinitions.GuardTypeAllChildrenComplete, ParentInput: "parent", SpawnedBy: "spawner"},
				},
			},
		},
	}
	findings := rulePerInputGuards(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestConfigValidator_WorkTypeHandlingBehavior_RejectsMultipleDefaultWorkTypes(t *testing.T) {
	cfg := testBaseConfig()
	cfg.WorkTypes = []factorydefinitions.WorkTypeConfig{
		{Name: "story", States: testStoryStates(), HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault}},
		{Name: "task", States: testStoryStates(), HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault}},
	}

	findings := ruleWorkTypeHandlingBehavior(cfg, false)
	assertFindingMatch(t, findings, "work-type-handling-behavior-unique-default", "factory.workTypes", "expected at most one work type with handlingBehavior DEFAULT")
}

func TestConfigValidator_WorkTypeHandlingBehavior_RequiresDefaultWhenConfigured(t *testing.T) {
	cfg := testBaseConfig()
	cfg.WorkTypes = []factorydefinitions.WorkTypeConfig{{Name: "story", States: testStoryStates()}}

	findings := ruleWorkTypeHandlingBehavior(cfg, true)
	assertFindingMatch(t, findings, "work-type-handling-behavior-required-default", "factory.workTypes", "expected exactly one work type with handlingBehavior DEFAULT")
}

func TestConfigValidator_WorkTypeHandlingBehavior_RejectsUnsupportedValues(t *testing.T) {
	cfg := testBaseConfig()
	cfg.WorkTypes = []factorydefinitions.WorkTypeConfig{{
		Name:             "story",
		States:           testStoryStates(),
		HandlingBehavior: []string{"PROMPT"},
	}}

	findings := ruleWorkTypeHandlingBehavior(cfg, false)
	assertFindingMatch(t, findings, "work-type-handling-behavior-value", `factory.workTypes[0].handlingBehavior[0]`, `unsupported handlingBehavior value "PROMPT"`)
}

func testStoryStates() []factorydefinitions.StateConfig {
	return []factorydefinitions.StateConfig{
		{Name: "init", Type: factorydefinitions.StateTypeInitial},
		{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
	}
}

func TestRuleAgentWorkerTools_RejectsMissingPolicy(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name:       "executor",
		Type:       factorydefinitions.WorkerTypeAgent,
		AgentTools: &factorydefinitions.AgentToolsConfig{},
	}}

	findings := ruleAgentWorkerTools(cfg)

	assertFindingMatch(t, findings, "agent-worker-tools-policy-required", "workers[0](executor).agentTools.policy", "requires an explicit policy")
}

func TestRuleAgentWorkerTools_RejectsUnsupportedPolicy(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name: "executor",
		Type: factorydefinitions.WorkerTypeAgent,
		AgentTools: &factorydefinitions.AgentToolsConfig{
			Policy: "FULL_SHELL",
		},
	}}

	findings := ruleAgentWorkerTools(cfg)

	assertFindingMatch(t, findings, "agent-worker-tools-policy-supported", "workers[0](executor).agentTools.policy", `unsupported agent tool policy "FULL_SHELL"`)
}

func TestRuleAgentWorkerTools_RejectsAgentToolsOnInferenceWorker(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name: "infer",
		Type: factorydefinitions.WorkerTypeInference,
		AgentTools: &factorydefinitions.AgentToolsConfig{
			Policy: factorydefinitions.AgentToolPolicyReadOnly,
		},
	}}

	findings := ruleAgentWorkerTools(cfg)

	assertFindingMatch(t, findings, "agent-worker-tools-worker-type", "workers[0](infer).agentTools", "only supported on AGENT_WORKER")
}

func TestRuleAgentWorkerTools_AllowsAgentWorkerPolicy(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name: "executor",
		Type: factorydefinitions.WorkerTypeAgent,
		AgentTools: &factorydefinitions.AgentToolsConfig{
			Policy: factorydefinitions.AgentToolPolicyEnabled,
		},
	}}

	findings := ruleAgentWorkerTools(cfg)
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}
