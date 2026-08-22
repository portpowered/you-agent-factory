package impl

import (
	"fmt"
	"strings"
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
func TestWorkerModelProviderTargetsRequiresACPIntegrationIdentity(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{Workers: []factorydefinitions.FactoryWorkerConfig{{
		Name:             "implementer",
		Type:             factorydefinitions.WorkerTypeModel,
		ExecutorProvider: "ACP",
	}}}

	targets := workerModelProviderTargets(cfg)
	if len(targets) != 1 {
		t.Fatalf("targets = %#v, want one", targets)
	}
	if targets[0].Code != CodeWorkerACPModelProviderRequired || targets[0].Path != "factory.workers[0](implementer).modelProvider" {
		t.Fatalf("target = %#v, want ACP modelProvider diagnostic", targets[0])
	}
}

func TestWorkerModelProviderTargetsAcceptsACPIntegrationIdentity(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{Workers: []factorydefinitions.FactoryWorkerConfig{{
		Name:             "implementer",
		Type:             factorydefinitions.WorkerTypeModel,
		ExecutorProvider: "ACP",
		ModelProvider:    "cursor-acp",
	}}}

	if targets := workerModelProviderTargets(cfg); len(targets) != 0 {
		t.Fatalf("targets = %#v, want none", targets)
	}
}

func TestWorkerReasoningEffortTargetsAcceptsXHighAndRejectsUnknown(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{Workers: []factorydefinitions.FactoryWorkerConfig{{
		Name:            "implementer",
		Type:            factorydefinitions.WorkerTypeAgent,
		ReasoningEffort: " XHIGH ",
	}}}
	if targets := workerReasoningEffortTargets(cfg); len(targets) != 0 {
		t.Fatalf("xhigh targets = %#v, want none", targets)
	}

	cfg.Workers[0].ReasoningEffort = "extreme"
	targets := workerReasoningEffortTargets(cfg)
	if len(targets) != 1 ||
		targets[0].Code != CodeWorkerUnsupportedReasoningEffort ||
		targets[0].Path != "factory.workers[0](implementer).reasoningEffort" {
		t.Fatalf("invalid targets = %#v, want reasoning effort diagnostic", targets)
	}
}

type sameNameAllChildrenCompleteJoinArityTest struct {
	name         string
	inputs       []factorydefinitions.IOConfig
	wantFinding  bool
	wantArity    int
	messageParts []string
}

var sameNameAllChildrenCompleteJoinArityTests = []sameNameAllChildrenCompleteJoinArityTest{
	{
		name: "one input remains accepted",
		inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "parent", StateName: "ready"},
		},
	},
	{
		name: "two input same-name join remains accepted",
		inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "parent", StateName: "ready"},
			{
				WorkTypeName: "same",
				StateName:    "ready",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:       factorydefinitions.GuardTypeSameName,
					MatchInput: "parent",
				},
			},
		},
	},
	{
		name: "two input child-complete join remains accepted",
		inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "parent", StateName: "ready"},
			{
				WorkTypeName: "child",
				StateName:    "complete",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:        factorydefinitions.GuardTypeAllChildrenComplete,
					ParentInput: "parent",
				},
			},
		},
	},
	{
		name: "two input combined join remains accepted",
		inputs: []factorydefinitions.IOConfig{
			{
				WorkTypeName: "parent",
				StateName:    "ready",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:       factorydefinitions.GuardTypeSameName,
					MatchInput: "child",
				},
			},
			{
				WorkTypeName: "child",
				StateName:    "complete",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:        factorydefinitions.GuardTypeAllChildrenComplete,
					ParentInput: "parent",
				},
			},
		},
	},
	{
		name: "three input same-name plus child-complete join is rejected",
		inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "parent", StateName: "ready"},
			{
				WorkTypeName: "same",
				StateName:    "ready",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:       factorydefinitions.GuardTypeSameName,
					MatchInput: "parent",
				},
			},
			{
				WorkTypeName: "child",
				StateName:    "complete",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:        factorydefinitions.GuardTypeAllChildrenComplete,
					ParentInput: "parent",
				},
			},
		},
		wantFinding: true,
		wantArity:   3,
		messageParts: []string{
			`workstation "fan-in"`,
			"unsupported SAME_NAME plus ALL_CHILDREN_COMPLETE join arity",
			"observed arity is 3 inputs",
			"at most 2 inputs are supported",
			"Split the fan-in into supported two-input workstation stages",
			"reduce the joined inputs",
		},
	},
	{
		name: "four input same-name plus child-complete join is rejected",
		inputs: append([]factorydefinitions.IOConfig{
			{WorkTypeName: "parent", StateName: "ready"},
			{
				WorkTypeName: "same",
				StateName:    "ready",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:       factorydefinitions.GuardTypeSameName,
					MatchInput: "parent",
				},
			},
			{
				WorkTypeName: "child",
				StateName:    "complete",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:        factorydefinitions.GuardTypeAllChildrenComplete,
					ParentInput: "parent",
				},
			},
		}, factorydefinitions.IOConfig{WorkTypeName: "extra", StateName: "ready"}),
		wantFinding: true,
		wantArity:   4,
	},
	{
		name: "three input same-name without child-complete remains accepted",
		inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "parent", StateName: "ready"},
			{
				WorkTypeName: "same",
				StateName:    "ready",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:       factorydefinitions.GuardTypeSameName,
					MatchInput: "parent",
				},
			},
			{WorkTypeName: "extra", StateName: "ready"},
		},
	},
	{
		name: "three input child-complete without same-name remains accepted",
		inputs: []factorydefinitions.IOConfig{
			{WorkTypeName: "parent", StateName: "ready"},
			{
				WorkTypeName: "child",
				StateName:    "complete",
				Guard: &factorydefinitions.InputGuardConfig{
					Type:        factorydefinitions.GuardTypeAllChildrenComplete,
					ParentInput: "parent",
				},
			},
			{WorkTypeName: "extra", StateName: "ready"},
		},
	},
}

func TestConfigValidator_UnsupportedSameNameAllChildrenCompleteJoinArity(t *testing.T) {
	for _, tt := range sameNameAllChildrenCompleteJoinArityTests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testBaseConfig()
			cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
				Name:   "fan-in",
				Inputs: tt.inputs,
			}}

			result := NewConfigValidator(nil).Validate(cfg)
			var finding *Finding
			for index := range result.Findings {
				if result.Findings[index].Rule == "same-name-all-children-complete-join-arity" {
					finding = &result.Findings[index]
					break
				}
			}

			if !tt.wantFinding {
				if finding != nil {
					t.Fatalf("unexpected unsupported join arity finding: %#v", *finding)
				}
				return
			}
			if finding == nil {
				t.Fatalf("expected unsupported join arity finding, got %#v", result.Findings)
			}
			if finding.Severity != SeverityError {
				t.Fatalf("finding severity = %q, want %q", finding.Severity, SeverityError)
			}
			wantPath := "workstations[0](fan-in).inputs"
			if finding.Path != wantPath {
				t.Fatalf("finding path = %q, want %q", finding.Path, wantPath)
			}
			if tt.messageParts != nil && !containsAll(finding.Message, tt.messageParts...) {
				t.Fatalf("finding message = %q, want parts %v", finding.Message, tt.messageParts)
			}
			if !strings.Contains(finding.Message, fmt.Sprintf("observed arity is %d inputs", tt.wantArity)) {
				t.Fatalf("finding message = %q, want arity %d", finding.Message, tt.wantArity)
			}
		})
	}
}
