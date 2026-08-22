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

func TestExpectedArtifactTargets_ReportsActionableOwningDefinitionPaths(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "story",
			ExpectedArtifacts: []factorydefinitions.ExpectedArtifactConfig{{
				Name: "", Pattern: "artifacts/story.json",
			}},
		}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name:   "execute-story",
			Inputs: []factorydefinitions.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			ExpectedArtifacts: []factorydefinitions.ExpectedArtifactConfig{{
				Name: "manifest", Pattern: "artifacts/{{ (index .Inputs 1).Name }}/manifest.json",
			}},
		}},
	}

	targets := ExpectedArtifactTargets(cfg)
	if len(targets) != 2 {
		t.Fatalf("ExpectedArtifactTargets() returned %d targets, want 2: %#v", len(targets), targets)
	}
	workTypeTarget, workstationTarget := targets[0], targets[1]
	if workTypeTarget.Code != CodeWorkTypeInvalidExpectedArtifact ||
		workTypeTarget.Subject.Type != SubjectTypeWorkType || workTypeTarget.Subject.ID != "story" ||
		workTypeTarget.Path != "factory.workTypes[0](story).expectedArtifacts[0]" ||
		!strings.Contains(workTypeTarget.Message, "work type \"story\"") ||
		!strings.Contains(workTypeTarget.Message, "name is required") {
		t.Fatalf("work type target = %#v, want owning definition path and diagnostic", workTypeTarget)
	}
	if workstationTarget.Code != CodeWorkstationInvalidExpectedArtifact ||
		workstationTarget.Subject.Type != SubjectTypeWorkstation || workstationTarget.Subject.ID != "execute-story" ||
		workstationTarget.Path != "factory.workstations[0](execute-story).expectedArtifacts[0]" ||
		!strings.Contains(workstationTarget.Message, "workstation \"execute-story\"") ||
		!strings.Contains(workstationTarget.Message, "cannot be rendered") {
		t.Fatalf("workstation target = %#v, want owning definition path and diagnostic", workstationTarget)
	}
}

func TestWebhookTargetsAcceptsCanonicalFilterAndDeliveryPolicy(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Webhooks = []factorydefinitions.FactoryWebhookConfig{{
		Name:             "monitor",
		Enabled:          true,
		URL:              "https://hooks.example.test/factory",
		SigningSecretRef: "secrets/factory-monitor",
		Filter: factorydefinitions.FactoryWebhookFilterConfig{
			EventTypes:       []string{factorydefinitions.FactoryWebhookEventTypeWorkStateChange, factorydefinitions.FactoryWebhookEventTypeDispatchReconciled},
			DispatchStatuses: []string{factorydefinitions.FactoryWebhookDispatchStatusFailed},
		},
		DeliveryPolicy: &factorydefinitions.FactoryWebhookDeliveryPolicyConfig{
			RequestTimeout:    stringPointerForWebhookTest("15s"),
			MaxAttempts:       intPointerForWebhookTest(3),
			InitialBackoff:    stringPointerForWebhookTest("1s"),
			BackoffMultiplier: floatPointerForWebhookTest(1.5),
			MaxBackoff:        stringPointerForWebhookTest("5s"),
		},
	}}

	findings := WebhookTargets(cfg)
	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestWebhookTargetsRejectsDispatchFiltersWithNoCompatibleStatus(t *testing.T) {
	tests := []struct {
		name       string
		eventTypes []string
		statuses   []string
	}{
		{
			name:       "response cannot select interrupted",
			eventTypes: []string{factorydefinitions.FactoryWebhookEventTypeDispatchResponse},
			statuses:   []string{factorydefinitions.FactoryWebhookDispatchStatusInterrupted},
		},
		{
			name:       "interrupted cannot select failed",
			eventTypes: []string{factorydefinitions.FactoryWebhookEventTypeDispatchInterrupted},
			statuses:   []string{factorydefinitions.FactoryWebhookDispatchStatusFailed},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := testBaseConfig()
			cfg.Webhooks = []factorydefinitions.FactoryWebhookConfig{{
				Name:             "monitor",
				URL:              "https://hooks.example.test/factory",
				SigningSecretRef: "secrets/factory-monitor",
				Filter: factorydefinitions.FactoryWebhookFilterConfig{
					EventTypes:       test.eventTypes,
					DispatchStatuses: test.statuses,
				},
			}}

			findings := WebhookTargets(cfg)
			assertWebhookTargetMatch(
				t,
				findings,
				CodeWebhookDispatchStatusIncompatible,
				"factory.webhooks[0](monitor).filter.dispatchStatuses",
				"no status compatible with the configured dispatch event types",
			)
		})
	}
}

func TestWebhookTargetsAcceptsMixedDispatchFilterStatusCombinations(t *testing.T) {
	tests := []struct {
		name       string
		eventTypes []string
		statuses   []string
	}{
		{
			name: "failed response and interrupted dispatch",
			eventTypes: []string{
				factorydefinitions.FactoryWebhookEventTypeDispatchResponse,
				factorydefinitions.FactoryWebhookEventTypeDispatchInterrupted,
			},
			statuses: []string{
				factorydefinitions.FactoryWebhookDispatchStatusFailed,
				factorydefinitions.FactoryWebhookDispatchStatusInterrupted,
			},
		},
		{
			name: "failed response among mixed work and dispatch events",
			eventTypes: []string{
				factorydefinitions.FactoryWebhookEventTypeWorkStateChange,
				factorydefinitions.FactoryWebhookEventTypeDispatchResponse,
			},
			statuses: []string{factorydefinitions.FactoryWebhookDispatchStatusFailed},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := testBaseConfig()
			cfg.Webhooks = []factorydefinitions.FactoryWebhookConfig{{
				Name:             "monitor",
				URL:              "https://hooks.example.test/factory",
				SigningSecretRef: "secrets/factory-monitor",
				Filter: factorydefinitions.FactoryWebhookFilterConfig{
					EventTypes:       test.eventTypes,
					DispatchStatuses: test.statuses,
				},
			}}

			findings := WebhookTargets(cfg)
			if len(findings) != 0 {
				t.Fatalf("findings = %#v, want none", findings)
			}
		})
	}
}

func TestWebhookTargetsRejectsInvalidFieldsWithEndpointPaths(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Webhooks = []factorydefinitions.FactoryWebhookConfig{
		{
			Name:             "monitor",
			URL:              "ftp://hooks.example.test/factory",
			SigningSecretRef: " ",
			Filter: factorydefinitions.FactoryWebhookFilterConfig{
				EventTypes:       []string{factorydefinitions.FactoryWebhookEventTypeWorkStateChange, "NOT_CANONICAL"},
				DispatchStatuses: []string{factorydefinitions.FactoryWebhookDispatchStatusFailed},
			},
			DeliveryPolicy: &factorydefinitions.FactoryWebhookDeliveryPolicyConfig{
				MaxAttempts:       intPointerForWebhookTest(0),
				InitialBackoff:    stringPointerForWebhookTest("5s"),
				MaxBackoff:        stringPointerForWebhookTest("1s"),
				BackoffMultiplier: floatPointerForWebhookTest(0.5),
			},
		},
		{
			Name:             " monitor ",
			Enabled:          true,
			URL:              "https://hooks.example.test/other",
			SigningSecretRef: "secrets/other",
			Filter: factorydefinitions.FactoryWebhookFilterConfig{
				EventTypes: []string{factorydefinitions.FactoryWebhookEventTypeWorkStateChange},
			},
		},
	}

	findings := WebhookTargets(cfg)
	assertWebhookTargetMatch(t, findings, CodeWebhookURLInvalid, "factory.webhooks[0](monitor).url", "absolute http or https")
	assertWebhookTargetMatch(t, findings, CodeWebhookSecretReferenceRequired, "factory.webhooks[0](monitor).signingSecretRef", "non-empty")
	assertWebhookTargetMatch(t, findings, CodeWebhookEventTypeUnsupported, "factory.webhooks[0](monitor).filter.eventTypes[1]", "NOT_CANONICAL")
	assertWebhookTargetMatch(t, findings, CodeWebhookDeliveryPolicyInvalid, "factory.webhooks[0](monitor).deliveryPolicy.maxAttempts", "positive")
	assertWebhookTargetMatch(t, findings, CodeWebhookDeliveryPolicyInvalid, "factory.webhooks[0](monitor).deliveryPolicy.backoffMultiplier", "at least 1")
	assertWebhookTargetMatch(t, findings, CodeWebhookDeliveryPolicyInvalid, "factory.webhooks[0](monitor).deliveryPolicy.maxBackoff", "initialBackoff")
	assertWebhookTargetMatch(t, findings, CodeWebhookNameDuplicate, "factory.webhooks[1]( monitor ).name", "duplicates")
}

func assertWebhookTargetMatch(t *testing.T, targets []Target, code, pathSubstring, messageSubstring string) {
	t.Helper()
	for _, target := range targets {
		if target.Code != code || target.Severity != SeverityError || !strings.Contains(target.Path, pathSubstring) {
			continue
		}
		if !strings.Contains(target.Message, messageSubstring) {
			t.Fatalf("target message = %q, want substring %q", target.Message, messageSubstring)
		}
		return
	}
	t.Fatalf("expected error target with code %q, got %v", code, targets)
}

func stringPointerForWebhookTest(value string) *string { return &value }

func intPointerForWebhookTest(value int) *int { return &value }

func floatPointerForWebhookTest(value float64) *float64 { return &value }
