package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestRuleFactoryGuards_InferenceThrottleRequiresModelProvider(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Guards = []interfaces.FactoryGuardConfig{{
		Type:          interfaces.GuardTypeInferenceThrottle,
		RefreshWindow: "15m",
	}}

	findings := ruleFactoryGuards(cfg)
	assertFindingMatch(t, findings, "factory-guard-inference-throttle-model-provider", "guards[0](inference_throttle_guard).modelProvider", "modelProvider")
}

func TestRuleFactoryGuards_InferenceThrottleRejectsInvalidRefreshWindow(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Guards = []interfaces.FactoryGuardConfig{{
		Type:          interfaces.GuardTypeInferenceThrottle,
		ModelProvider: "claude",
		RefreshWindow: "tomorrow",
	}}

	findings := ruleFactoryGuards(cfg)
	assertFindingMatch(t, findings, "factory-guard-inference-throttle-refresh-window", "guards[0](inference_throttle_guard).refreshWindow", "positive duration")
}

func TestRuleFactoryGuards_InferenceThrottleRejectsNonPositiveRefreshWindow(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Guards = []interfaces.FactoryGuardConfig{{
		Type:          interfaces.GuardTypeInferenceThrottle,
		ModelProvider: "claude",
		RefreshWindow: "0s",
	}}

	findings := ruleFactoryGuards(cfg)
	assertFindingMatch(t, findings, "factory-guard-inference-throttle-refresh-window", "guards[0](inference_throttle_guard).refreshWindow", "positive duration")
}

func TestRuleFactoryGuards_RejectsUnsupportedType(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Guards = []interfaces.FactoryGuardConfig{{Type: interfaces.GuardTypeVisitCount}}

	findings := ruleFactoryGuards(cfg)
	assertFindingMatch(t, findings, "factory-guard-unknown-type", "guards[0](visit_count)", "factory guards support: inference_throttle_guard")
}

func TestRuleFactoryGuards_ValidInferenceThrottleGuard(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Guards = []interfaces.FactoryGuardConfig{{
		Type:          interfaces.GuardTypeInferenceThrottle,
		ModelProvider: "claude",
		Model:         "claude-sonnet-4-20250514",
		RefreshWindow: "15m",
	}}

	findings := ruleFactoryGuards(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %#v", findings)
	}
}

func TestRuleGuards_VisitCountMissingWorkstation(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:   "ws",
		Guards: []interfaces.GuardConfig{{Type: interfaces.GuardTypeVisitCount, MaxVisits: 3}},
	}}
	findings := ruleGuards(cfg)
	assertFindingExists(t, findings, "guard-visit-count-workstation")
}

func TestRuleGuards_VisitCountInvalidWorkstation(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:   "ws",
		Guards: []interfaces.GuardConfig{{Type: interfaces.GuardTypeVisitCount, Workstation: "nonexistent", MaxVisits: 3}},
	}}
	findings := ruleGuards(cfg)
	assertFindingExists(t, findings, "guard-visit-count-workstation")
}

func TestRuleGuards_VisitCountZeroMaxVisits(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{
		{Name: "ws", Guards: []interfaces.GuardConfig{{Type: interfaces.GuardTypeVisitCount, Workstation: "ws", MaxVisits: 0}}},
	}
	findings := ruleGuards(cfg)
	assertFindingExists(t, findings, "guard-visit-count-max-visits")
}

func TestRuleGuards_MatchesFieldsMissingMatchConfig(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:   "ws",
		Guards: []interfaces.GuardConfig{{Type: interfaces.GuardTypeMatchesFields}},
	}}

	findings := ruleGuards(cfg)
	if len(findings) != 1 || findings[0].Rule != "guard-matches-fields-input-key" {
		t.Fatalf("expected match_config.input_key finding, got %#v", findings)
	}
}

func TestRuleGuards_MatchesFieldsEmptyInputKey(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "ws",
		Guards: []interfaces.GuardConfig{{
			Type:        interfaces.GuardTypeMatchesFields,
			MatchConfig: &interfaces.GuardMatchConfig{InputKey: "   "},
		}},
	}}

	findings := ruleGuards(cfg)
	if len(findings) != 1 || findings[0].Rule != "guard-matches-fields-input-key" {
		t.Fatalf("expected match_config.input_key finding, got %#v", findings)
	}
}

func TestRuleGuards_ValidMatchesFieldsGuard(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "ws",
		Guards: []interfaces.GuardConfig{{
			Type:        interfaces.GuardTypeMatchesFields,
			MatchConfig: &interfaces.GuardMatchConfig{InputKey: `.Tags["_last_output"]`},
		}},
	}}

	findings := ruleGuards(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %#v", findings)
	}
}

func TestRuleGuards_UnknownType(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:   "ws",
		Guards: []interfaces.GuardConfig{{Type: "bogus"}},
	}}
	findings := ruleGuards(cfg)
	assertFindingExists(t, findings, "guard-unknown-type")
}

func TestRuleGuards_RejectsWorkstationLevelChildFanInTypes(t *testing.T) {
	tests := []struct {
		name      string
		guardType interfaces.GuardType
	}{
		{name: "all children complete", guardType: interfaces.GuardTypeAllChildrenComplete},
		{name: "any child failed", guardType: interfaces.GuardTypeAnyChildFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testBaseConfig()
			cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
				Name:   "ws",
				Guards: []interfaces.GuardConfig{{Type: tt.guardType}},
			}}
			findings := ruleGuards(cfg)
			assertFindingExists(t, findings, "guard-unknown-type")
			if !strings.Contains(findings[0].Message, "use per-input guards for child fan-in") {
				t.Fatalf("expected per-input guard guidance, got %q", findings[0].Message)
			}
		})
	}
}

func TestRuleGuards_ValidGuards(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "ws",
		Guards: []interfaces.GuardConfig{
			{Type: interfaces.GuardTypeVisitCount, Workstation: "ws", MaxVisits: 3},
		},
	}}
	findings := ruleGuards(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestRuleWorkstationKind_UnknownKind(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{Name: "ws", Kind: "bogus"}}
	findings := ruleWorkstationKind(cfg)
	assertFindingExists(t, findings, "workstation-kind")
}

func TestRuleClassifierWorkstations_RejectsMissingRoutesAndLegacySuccessPaths(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "classify-story",
		Type:           interfaces.WorkstationTypeClassify,
		WorkerTypeName: "w1",
		Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		OnContinue:     []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		OnRejection:    []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
	}}

	findings := ruleClassifierWorkstations(cfg)
	assertFindingExists(t, findings, "classifier-workstation-routes")
	assertFindingExists(t, findings, "classifier-workstation-outputs")
	assertFindingExists(t, findings, "classifier-workstation-on-continue")
	assertFindingExists(t, findings, "classifier-workstation-on-rejection")
}

func TestRuleClassifierWorkstations_RejectsDuplicateLabelsWhitespaceAndEmptyOutputs(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "classify-story",
		Type:           interfaces.WorkstationTypeClassify,
		WorkerTypeName: "w1",
		Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		ClassificationRoutes: []interfaces.ClassificationRouteConfig{
			{Label: "approved", Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}}},
			{Label: "approved"},
			{Label: " needs_review ", Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "review"}}},
			{Label: "   ", Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}}},
		},
	}}

	findings := ruleClassifierWorkstations(cfg)
	assertFindingExists(t, findings, "classifier-workstation-route-label")
	assertFindingExists(t, findings, "classifier-workstation-route-outputs")
}

func TestRuleClassifierWorkstations_AllowsValidClassifierTopology(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "classify-story",
		Type:           interfaces.WorkstationTypeClassify,
		WorkerTypeName: "w1",
		Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		ClassificationRoutes: []interfaces.ClassificationRouteConfig{
			{Label: "approved", Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}}},
			{Label: "needs_review", Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}}},
		},
		OnFailure: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
	}}

	findings := ruleClassifierWorkstations(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestRuleCronWorkstations_ValidScheduleCron(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "daily-refresh",
		Kind:           interfaces.WorkstationKindCron,
		WorkerTypeName: "w1",
		Cron: &interfaces.CronConfig{
			Schedule:       "0 * * * *",
			TriggerAtStart: true,
			Jitter:         "30s",
			ExpiryWindow:   "10m",
		},
		Outputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
	}}
	findings := ruleCronWorkstations(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestRuleCronWorkstations_ValidRequiredInputCron(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "refresh-ready-task",
		Kind:           interfaces.WorkstationKindCron,
		WorkerTypeName: "w1",
		Cron:           &interfaces.CronConfig{Schedule: "0 * * * *"},
		Inputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
		Outputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "done",
		}},
	}}
	findings := ruleCronWorkstations(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestRuleCronWorkstations_MissingCronConfig(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:    "daily-refresh",
		Kind:    interfaces.WorkstationKindCron,
		Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
	}}
	findings := ruleCronWorkstations(cfg)
	assertFindingExists(t, findings, "cron-config")
}

func TestRuleCronWorkstations_MissingSchedule(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:    "daily-refresh",
		Kind:    interfaces.WorkstationKindCron,
		Cron:    &interfaces.CronConfig{},
		Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
	}}
	findings := ruleCronWorkstations(cfg)
	assertFindingExists(t, findings, "cron-schedule")
}

func TestRuleCronWorkstations_InvalidScheduleNamesWorkstationAndValue(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:    "daily-refresh",
		Kind:    interfaces.WorkstationKindCron,
		Cron:    &interfaces.CronConfig{Schedule: "not a cron"},
		Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
	}}
	findings := ruleCronWorkstations(cfg)
	assertFindingExists(t, findings, "cron-schedule")
	if findings[0].Path != "workstations[0](daily-refresh).cron.schedule" {
		t.Fatalf("expected path to name cron workstation and schedule field, got %q", findings[0].Path)
	}
	if !strings.Contains(findings[0].Message, `"not a cron"`) {
		t.Fatalf("expected message to include bad schedule value, got %q", findings[0].Message)
	}
}

func TestRuleCronWorkstations_UnsupportedIntervalNamesWorkstationAndField(t *testing.T) {
	var cron interfaces.CronConfig
	if err := json.Unmarshal([]byte(`{"interval":"5m"}`), &cron); err != nil {
		t.Fatalf("unmarshal cron config: %v", err)
	}
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:    "daily-refresh",
		Kind:    interfaces.WorkstationKindCron,
		Cron:    &cron,
		Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
	}}
	findings := ruleCronWorkstations(cfg)
	assertFindingExists(t, findings, "cron-interval")
	if findings[0].Path != "workstations[0](daily-refresh).cron.interval" {
		t.Fatalf("expected path to name cron workstation and field, got %q", findings[0].Path)
	}
}

func TestRuleCronWorkstations_InvalidJitterNamesWorkstationAndField(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:    "daily-refresh",
		Kind:    interfaces.WorkstationKindCron,
		Cron:    &interfaces.CronConfig{Schedule: "0 * * * *", Jitter: "-1s"},
		Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
	}}
	findings := ruleCronWorkstations(cfg)
	assertFindingExists(t, findings, "cron-jitter")
	if findings[0].Path != "workstations[0](daily-refresh).cron.jitter" {
		t.Fatalf("expected path to name cron workstation and field, got %q", findings[0].Path)
	}
}

func TestRuleCronWorkstations_InvalidExpiryWindowNamesWorkstationAndField(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:    "daily-refresh",
		Kind:    interfaces.WorkstationKindCron,
		Cron:    &interfaces.CronConfig{Schedule: "0 * * * *", ExpiryWindow: "0s"},
		Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
	}}
	findings := ruleCronWorkstations(cfg)
	assertFindingExists(t, findings, "cron-expiry-window")
	if findings[0].Path != "workstations[0](daily-refresh).cron.expiry_window" {
		t.Fatalf("expected path to name cron workstation and field, got %q", findings[0].Path)
	}
}

func TestRuleCronWorkstations_MissingOutput(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "daily-refresh",
		Kind:           interfaces.WorkstationKindCron,
		WorkerTypeName: "w1",
		Cron:           &interfaces.CronConfig{Schedule: "0 * * * *"},
	}}
	findings := ruleCronWorkstations(cfg)
	assertFindingExists(t, findings, "cron-output")
}

func TestRuleCronWorkstations_MissingWorker(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "daily-refresh",
		Kind: interfaces.WorkstationKindCron,
		Cron: &interfaces.CronConfig{Schedule: "0 * * * *"},
		Outputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
	}}
	findings := ruleCronWorkstations(cfg)
	assertFindingExists(t, findings, "cron-worker")
	if findings[0].Path != "workstations[0](daily-refresh).worker" {
		t.Fatalf("expected path to name cron workstation and worker field, got %q", findings[0].Path)
	}
}

func TestRuleCronWorkstations_NonCronWithCronConfig(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "processor",
		Kind: interfaces.WorkstationKindStandard,
		Cron: &interfaces.CronConfig{Schedule: "0 * * * *"},
	}}
	findings := ruleCronWorkstations(cfg)
	assertFindingExists(t, findings, "cron-type")
}

func TestRuleWorkerReferences_NonexistentWorker(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{Name: "ws", WorkerTypeName: "nonexistent"}}
	findings := ruleWorkerReferences(cfg)
	assertFindingExists(t, findings, "workstation-worker-ref")
}

func TestRuleWorkstationKindAndWorker_ValidConfig(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "ws", Kind: interfaces.WorkstationKindRepeater, WorkerTypeName: "w1",
	}}
	f1 := ruleWorkstationKind(cfg)
	f2 := ruleWorkerReferences(cfg)
	if len(f1)+len(f2) != 0 {
		t.Fatalf("expected no findings, got kind=%v worker=%v", f1, f2)
	}
}

func TestRulePerInputGuards_MissingParentInput(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []interfaces.IOConfig{{
			WorkTypeName: "task", StateName: "init",
			Guard: &interfaces.InputGuardConfig{Type: interfaces.GuardTypeAllChildrenComplete},
		}},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-parent-input")
}

func TestRulePerInputGuards_ParentInputNotMatching(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []interfaces.IOConfig{{
			WorkTypeName: "task", StateName: "init",
			Guard: &interfaces.InputGuardConfig{Type: interfaces.GuardTypeAllChildrenComplete, ParentInput: "other"},
		}},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-parent-input")
}

func TestRulePerInputGuards_SelfReference(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []interfaces.IOConfig{{
			WorkTypeName: "task", StateName: "init",
			Guard: &interfaces.InputGuardConfig{Type: interfaces.GuardTypeAllChildrenComplete, ParentInput: "task"},
		}},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-self-ref")
}

func TestRulePerInputGuards_InvalidSpawnedBy(t *testing.T) {
	cfg := testBaseConfig()
	cfg.WorkTypes = append(cfg.WorkTypes, interfaces.WorkTypeConfig{
		Name: "parent", States: []interfaces.StateConfig{{Name: "init", Type: interfaces.StateTypeInitial}},
	})
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []interfaces.IOConfig{
			{WorkTypeName: "parent", StateName: "init"},
			{
				WorkTypeName: "task", StateName: "init",
				Guard: &interfaces.InputGuardConfig{Type: interfaces.GuardTypeAllChildrenComplete, ParentInput: "parent", SpawnedBy: "nonexistent"},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-spawned-by")
}

func TestRulePerInputGuards_UnsupportedType(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []interfaces.IOConfig{{
			WorkTypeName: "task", StateName: "init",
			Guard: &interfaces.InputGuardConfig{Type: interfaces.GuardTypeVisitCount},
		}},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-type")
}

func TestRulePerInputGuards_SameNameMissingMatchInput(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []interfaces.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard:        &interfaces.InputGuardConfig{Type: interfaces.GuardTypeSameName},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-match-input")
}

func TestRulePerInputGuards_SameNameMatchInputNotMatching(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []interfaces.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard: &interfaces.InputGuardConfig{
					Type:       interfaces.GuardTypeSameName,
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
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []interfaces.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard: &interfaces.InputGuardConfig{
					Type:       interfaces.GuardTypeSameName,
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
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []interfaces.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard: &interfaces.InputGuardConfig{
					Type:       interfaces.GuardTypeSameName,
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
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []interfaces.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard:        &interfaces.InputGuardConfig{Type: interfaces.GuardTypeSameTraceID},
			},
		},
	}}
	findings := rulePerInputGuards(cfg)
	assertFindingExists(t, findings, "per-input-guard-same-trace-match-input")
}

func TestRulePerInputGuards_SameTraceIDMatchInputNotMatching(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []interfaces.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard: &interfaces.InputGuardConfig{
					Type:       interfaces.GuardTypeSameTraceID,
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
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []interfaces.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard: &interfaces.InputGuardConfig{
					Type:       interfaces.GuardTypeSameTraceID,
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
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name: "ws",
		Inputs: []interfaces.IOConfig{
			{WorkTypeName: "plan", StateName: "init"},
			{
				WorkTypeName: "task",
				StateName:    "init",
				Guard: &interfaces.InputGuardConfig{
					Type:       interfaces.GuardTypeSameTraceID,
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
	cfg.WorkTypes = append(cfg.WorkTypes, interfaces.WorkTypeConfig{
		Name: "parent", States: []interfaces.StateConfig{{Name: "init", Type: interfaces.StateTypeInitial}},
	})
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{
		{Name: "spawner"},
		{
			Name: "ws",
			Inputs: []interfaces.IOConfig{
				{WorkTypeName: "parent", StateName: "init"},
				{
					WorkTypeName: "task", StateName: "init",
					Guard: &interfaces.InputGuardConfig{Type: interfaces.GuardTypeAllChildrenComplete, ParentInput: "parent", SpawnedBy: "spawner"},
				},
			},
		},
	}
	findings := rulePerInputGuards(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}
