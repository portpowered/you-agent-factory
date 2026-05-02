package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type stubRequiredToolChecker map[string]RequiredToolCheckResult

func (s stubRequiredToolChecker) Check(tool interfaces.RequiredToolConfig) RequiredToolCheckResult {
	if result, ok := s[tool.Command]; ok {
		return result
	}
	return RequiredToolCheckResult{}
}

// --- US-001: ValidationResult and Finding types ---

func TestValidationResult_HasErrors_FalseWithOnlyWarningsAndHints(t *testing.T) {
	vr := &ValidationResult{
		Findings: []Finding{
			{Severity: SeverityWarning, Path: "a", Message: "warn", Rule: "r1"},
			{Severity: SeverityHint, Path: "b", Message: "hint", Rule: "r2"},
		},
	}
	if vr.HasErrors() {
		t.Fatal("HasErrors() should be false when only warnings and hints present")
	}
}

func TestValidationResult_HasErrors_TrueWithErrors(t *testing.T) {
	vr := &ValidationResult{
		Findings: []Finding{
			{Severity: SeverityWarning, Path: "a", Message: "warn", Rule: "r1"},
			{Severity: SeverityError, Path: "b", Message: "err", Rule: "r2"},
		},
	}
	if !vr.HasErrors() {
		t.Fatal("HasErrors() should be true when error findings present")
	}
}

func TestValidationResult_Errors_ReturnsOnlyErrors(t *testing.T) {
	vr := &ValidationResult{
		Findings: []Finding{
			{Severity: SeverityWarning, Path: "a", Message: "warn", Rule: "r1"},
			{Severity: SeverityError, Path: "b", Message: "err1", Rule: "r2"},
			{Severity: SeverityHint, Path: "c", Message: "hint", Rule: "r3"},
			{Severity: SeverityError, Path: "d", Message: "err2", Rule: "r4"},
		},
	}
	errs := vr.Errors()
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errs))
	}
	if errs[0].Path != "b" || errs[1].Path != "d" {
		t.Fatalf("unexpected error paths: %v", errs)
	}
}

// --- US-002: ConfigValidator runs all rules, no short-circuit ---

func TestConfigValidator_ReportsAllErrors(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		InputTypes: []interfaces.InputTypeConfig{
			{Name: "", Type: "default"}, // error: missing name
		},
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "task", States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
			}},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "ws1",
				Inputs: []interfaces.IOConfig{
					{WorkTypeName: "task", StateName: "init"},
				},
				Outputs: []interfaces.IOConfig{
					{WorkTypeName: "task", StateName: "nonexistent"}, // error: bad ref
				},
			},
		},
	}
	cv := NewConfigValidator()
	result := cv.Validate(cfg)
	if !result.HasErrors() {
		t.Fatal("expected errors")
	}
	errs := result.Errors()
	if len(errs) < 2 {
		t.Fatalf("expected at least 2 errors from independent rules, got %d: %v", len(errs), errs)
	}
}

// --- US-003: Input type validation rule ---

func TestRuleInputTypes_MissingName(t *testing.T) {
	cfg := &interfaces.FactoryConfig{InputTypes: []interfaces.InputTypeConfig{{Name: "", Type: "default"}}}
	findings := ruleInputTypes(cfg)
	assertFindingExists(t, findings, "input-type-name")
}

func TestRuleInputTypes_ReservedDefault(t *testing.T) {
	cfg := &interfaces.FactoryConfig{InputTypes: []interfaces.InputTypeConfig{{Name: "default", Type: "default"}}}
	findings := ruleInputTypes(cfg)
	assertFindingExists(t, findings, "input-type-reserved")
}

func TestRuleInputTypes_Duplicate(t *testing.T) {
	cfg := &interfaces.FactoryConfig{InputTypes: []interfaces.InputTypeConfig{
		{Name: "foo", Type: "default"},
		{Name: "foo", Type: "default"},
	}}
	findings := ruleInputTypes(cfg)
	assertFindingExists(t, findings, "input-type-duplicate")
}

func TestRuleInputTypes_MissingType(t *testing.T) {
	cfg := &interfaces.FactoryConfig{InputTypes: []interfaces.InputTypeConfig{{Name: "foo", Type: ""}}}
	findings := ruleInputTypes(cfg)
	assertFindingExists(t, findings, "input-type-type")
}

func TestRuleInputTypes_UnknownType(t *testing.T) {
	cfg := &interfaces.FactoryConfig{InputTypes: []interfaces.InputTypeConfig{{Name: "foo", Type: "bogus"}}}
	findings := ruleInputTypes(cfg)
	assertFindingExists(t, findings, "input-type-type")
}

func TestRuleInputTypes_ValidConfig(t *testing.T) {
	cfg := &interfaces.FactoryConfig{InputTypes: []interfaces.InputTypeConfig{
		{Name: "batch", Type: interfaces.InputKindDefault},
	}}
	findings := ruleInputTypes(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

// --- US-004: Place reference validation rule ---

func testBaseConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "task", States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			}},
		},
		Workers: []interfaces.WorkerConfig{{Name: "w1"}},
	}
}

func TestRulePlaceReferences_InvalidInput(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:   "ws",
		Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "bogus"}},
	}}
	findings := rulePlaceReferences(cfg)
	assertFindingExists(t, findings, "workstation-input-ref")
}

func TestRulePlaceReferences_InvalidOutput(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:    "ws",
		Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "bogus"}},
	}}
	findings := rulePlaceReferences(cfg)
	assertFindingExists(t, findings, "workstation-output-ref")
}

func TestRulePlaceReferences_InvalidOnRejection(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:        "ws",
		Inputs:      []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		OnRejection: &interfaces.IOConfig{WorkTypeName: "task", StateName: "bogus"},
	}}
	findings := rulePlaceReferences(cfg)
	assertFindingExists(t, findings, "workstation-on-rejection-ref")
}

func TestRulePlaceReferences_InvalidOnFailure(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:      "ws",
		Inputs:    []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		OnFailure: &interfaces.IOConfig{WorkTypeName: "task", StateName: "bogus"},
	}}
	findings := rulePlaceReferences(cfg)
	assertFindingExists(t, findings, "workstation-on-failure-ref")
}

func TestRulePlaceReferences_AllValid(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:        "ws",
		Inputs:      []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs:     []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		OnRejection: &interfaces.IOConfig{WorkTypeName: "task", StateName: "failed"},
		OnFailure:   &interfaces.IOConfig{WorkTypeName: "task", StateName: "failed"},
	}}
	findings := rulePlaceReferences(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

// --- US-005: Guard validation rule ---

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

// --- US-006: Workstation kind and worker reference validation ---

func TestRuleWorkstationKind_UnknownKind(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{Name: "ws", Kind: "bogus"}}
	findings := ruleWorkstationKind(cfg)
	assertFindingExists(t, findings, "workstation-kind")
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

// --- US-007: Per-input guard validation rule ---

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

// --- US-009: Resource usage validation ---

func TestRuleResourceUsage_NonexistentResource(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:      "ws",
		Resources: []interfaces.ResourceConfig{{Name: "bogus", Capacity: 1}},
	}}
	findings := ruleResourceUsage(cfg)
	assertFindingExists(t, findings, "resource-usage-ref")
}

func TestRuleResourceUsage_ZeroCapacity(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []interfaces.ResourceConfig{{Name: "gpu", Capacity: 4}}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:      "ws",
		Resources: []interfaces.ResourceConfig{{Name: "gpu", Capacity: 0}},
	}}
	findings := ruleResourceUsage(cfg)
	assertFindingExists(t, findings, "resource-usage-capacity")
}

func TestRuleResourceUsage_ValidConfig(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Resources = []interfaces.ResourceConfig{{Name: "gpu", Capacity: 4}}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:      "ws",
		Resources: []interfaces.ResourceConfig{{Name: "gpu", Capacity: 2}},
	}}
	findings := ruleResourceUsage(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

// --- US-010: Portable required-tool validation ---

func TestRuleRequiredTools_MissingNameAndCommand(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		RequiredTools: []interfaces.RequiredToolConfig{{}},
	}

	findings := ruleRequiredTools(nil)(cfg)
	assertFindingExists(t, findings, "required-tool-name")
	assertFindingExists(t, findings, "required-tool-command")
}

func TestConfigValidator_RequiredToolsReportsPresentAndMissingCommandsDeterministically(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		RequiredTools: []interfaces.RequiredToolConfig{
			{Name: "Go toolchain", Command: "go"},
			{Name: "Missing helper", Command: "missing-tool"},
		},
	}

	validator := NewConfigValidator(WithRequiredToolChecker(stubRequiredToolChecker{
		"go":           {ResolvedPath: "/usr/bin/go"},
		"missing-tool": {Err: assertErrString(`required tool "Missing helper" command "missing-tool" was not found on PATH`)},
	}))
	result := validator.Validate(cfg)
	if !result.HasErrors() {
		t.Fatal("expected missing required tool to produce an error")
	}
	if len(result.Errors()) != 1 {
		t.Fatalf("expected one required-tool error, got %#v", result.Errors())
	}
	finding := result.Errors()[0]
	if finding.Rule != "required-tool-missing" {
		t.Fatalf("expected required-tool-missing rule, got %#v", finding)
	}
	if finding.Path != "resourceManifest.requiredTools[1].command" {
		t.Fatalf("expected path-specific missing-tool finding, got %#v", finding)
	}
	if !strings.Contains(finding.Message, `"missing-tool" was not found on PATH`) {
		t.Fatalf("expected PATH lookup guidance, got %#v", finding)
	}
}

func TestRuleRequiredTools_InvalidVersionProbeUsesVersionArgsPath(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		RequiredTools: []interfaces.RequiredToolConfig{{
			Name:        "Python",
			Command:     "python",
			VersionArgs: []string{"--version"},
		}},
	}

	findings := ruleRequiredTools(stubRequiredToolChecker{
		"python": {
			FailureKind: RequiredToolFailureKindVersionProbe,
			Err:         assertErrString(`required tool "Python" command "python" failed version probe "--version": exit status 1`),
		},
	})(cfg)
	assertFindingExists(t, findings, "required-tool-version-probe")
	if findings[0].Path != "resourceManifest.requiredTools[0].versionArgs" {
		t.Fatalf("expected versionArgs path, got %#v", findings[0])
	}
}

func TestRuleRequiredTools_MissingCommandWithVersionArgsUsesCommandPath(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		RequiredTools: []interfaces.RequiredToolConfig{{
			Name:        "Portable helper",
			Command:     "missing-helper",
			VersionArgs: []string{"--version"},
		}},
	}

	findings := ruleRequiredTools(stubRequiredToolChecker{
		"missing-helper": {
			FailureKind: RequiredToolFailureKindMissing,
			Err:         assertErrString(`required tool "Portable helper" command "missing-helper" was not found on PATH`),
		},
	})(cfg)
	assertFindingExists(t, findings, "required-tool-missing")
	if findings[0].Path != "resourceManifest.requiredTools[0].command" {
		t.Fatalf("expected command path for missing tool, got %#v", findings[0])
	}
}

func TestRuleRequiredTools_RejectsBlankVersionArgsEntries(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		RequiredTools: []interfaces.RequiredToolConfig{{
			Name:        "Python",
			Command:     "python",
			VersionArgs: []string{"--version", ""},
		}},
	}

	findings := ruleRequiredTools(nil)(cfg)
	assertFindingExists(t, findings, "required-tool-version-args")
}

// --- US-003: Portable bundled-file validation ---

func TestRuleBundledFiles_RejectsUnsupportedTypeEncodingAndRoot(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       "BINARY",
			TargetPath: "factory/misc/helper.bin",
			Content: interfaces.BundledFileContentConfig{
				Encoding: "base64",
				Inline:   "AA==",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-type")
	assertFindingExists(t, findings, "bundled-file-content-encoding")
}

func TestRuleBundledFiles_RejectsUnsafeTargetPath(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       "SCRIPT",
			TargetPath: "../scripts/setup-workspace.py",
			Content: interfaces.BundledFileContentConfig{
				Encoding: "utf-8",
				Inline:   "print('portable')\n",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-path")
	if findings[0].Path != "resourceManifest.bundledFiles[0].targetPath" {
		t.Fatalf("expected targetPath-specific finding, got %#v", findings[0])
	}
}

func TestRuleBundledFiles_RejectsAbsoluteTargetPath(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       interfaces.BundledFileTypeScript,
			TargetPath: "/factory/scripts/setup-workspace.py",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "print('portable')\n",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-path")
	if !strings.Contains(findings[0].Message, "not absolute") {
		t.Fatalf("expected absolute-path guidance, got %#v", findings[0])
	}
}

func TestRuleBundledFiles_RejectsMissingInlineContent(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       interfaces.BundledFileTypeScript,
			TargetPath: "factory/scripts/setup-workspace.py",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-content-inline")
	if findings[0].Path != "resourceManifest.bundledFiles[0].content.inline" {
		t.Fatalf("expected inline-specific finding, got %#v", findings[0])
	}
}

func TestConfigValidator_BundledFilesAcceptCanonicalScriptAndDocTargets(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{
			{
				Type:       "SCRIPT",
				TargetPath: "factory/scripts/setup-workspace.py",
				Content: interfaces.BundledFileContentConfig{
					Encoding: "utf-8",
					Inline:   "print('portable')\n",
				},
			},
			{
				Type:       "DOC",
				TargetPath: "factory/docs/usage.md",
				Content: interfaces.BundledFileContentConfig{
					Encoding: "utf-8",
					Inline:   "# Usage\n",
				},
			},
			{
				Type:       interfaces.BundledFileTypeRootHelper,
				TargetPath: "Makefile",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
					Inline:   "test:\n\tgo test ./...\n",
				},
			},
		},
	}

	findings := ruleBundledFiles(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no bundled-file findings, got %#v", findings)
	}
}

func TestRuleBundledFiles_RejectsTargetOutsideCanonicalRootForType(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       "DOC",
			TargetPath: "factory/scripts/usage.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: "utf-8",
				Inline:   "# Usage\n",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-root")
}

func TestRuleBundledFiles_RejectsUnsupportedRootHelperTarget(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       interfaces.BundledFileTypeRootHelper,
			TargetPath: "README.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "outside allowlist\n",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-root-helper")
}

// --- Helper ---

func assertErrString(message string) error {
	return &staticErr{message: message}
}

type staticErr struct {
	message string
}

func (e *staticErr) Error() string {
	return e.message
}

func assertFindingExists(t *testing.T, findings []Finding, rule string) {
	t.Helper()
	for _, f := range findings {
		if f.Rule == rule && f.Severity == SeverityError {
			return
		}
	}
	t.Fatalf("expected error finding with rule %q, got %v", rule, findings)
}
