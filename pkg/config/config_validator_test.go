package config

import (
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"strings"
	"testing"
)

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

func TestConfigValidator_ReportsAllErrors(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		InputTypes: []interfaces.InputTypeConfig{
			{Name: "", Type: "default"},
		},
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{{
				Name: "init",
				Type: interfaces.StateTypeInitial,
			}},
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "ws1",
			Inputs: []interfaces.IOConfig{{
				WorkTypeName: "task",
				StateName:    "init",
			}},
			Outputs: []interfaces.IOConfig{{
				WorkTypeName: "task",
				StateName:    "nonexistent",
			}},
		}},
	}
	result := NewConfigValidator().Validate(cfg)
	if !result.HasErrors() {
		t.Fatal("expected errors")
	}
	errs := result.Errors()
	if len(errs) < 2 {
		t.Fatalf("expected at least 2 errors from independent rules, got %d: %v", len(errs), errs)
	}
}

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
	cfg := &interfaces.FactoryConfig{InputTypes: []interfaces.InputTypeConfig{{
		Name: "batch",
		Type: interfaces.InputKindDefault,
	}}}
	findings := ruleInputTypes(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
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
		OnRejection: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "bogus"}},
	}}
	findings := rulePlaceReferences(cfg)
	assertFindingExists(t, findings, "workstation-on-rejection-ref")
}

func TestRulePlaceReferences_InvalidOnFailure(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:      "ws",
		Inputs:    []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		OnFailure: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "bogus"}},
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
		OnRejection: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
		OnFailure:   []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
	}}
	findings := rulePlaceReferences(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestRulePlaceReferences_InvalidClassificationRouteOutput(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "classifier",
		Type:           interfaces.WorkstationTypeClassify,
		WorkerTypeName: "w1",
		Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		ClassificationRoutes: []interfaces.ClassificationRouteConfig{{
			Label:   "approved",
			Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "bogus"}},
		}},
	}}

	findings := rulePlaceReferences(cfg)
	assertFindingExists(t, findings, "workstation-classification-route-ref")
}

type stubRequiredToolChecker map[string]RequiredToolCheckResult

func (s stubRequiredToolChecker) Check(tool interfaces.RequiredToolConfig) RequiredToolCheckResult {
	if result, ok := s[tool.Command]; ok {
		return result
	}
	return RequiredToolCheckResult{}
}

func testBaseConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "w1"}},
	}
}

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

func assertFindingMatch(t *testing.T, findings []Finding, rule string, pathSubstring string, messageSubstring string) {
	t.Helper()
	for _, f := range findings {
		if f.Rule != rule || f.Severity != SeverityError {
			continue
		}
		if !strings.Contains(f.Path, pathSubstring) {
			t.Fatalf("finding path = %q, want substring %q", f.Path, pathSubstring)
		}
		if !strings.Contains(f.Message, messageSubstring) {
			t.Fatalf("finding message = %q, want substring %q", f.Message, messageSubstring)
		}
		return
	}
	t.Fatalf("expected error finding with rule %q, got %v", rule, findings)
}
