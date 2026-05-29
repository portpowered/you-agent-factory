package config

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestConfigValidator_WorkTypeHandlingBehavior_RejectsMultipleDefaultWorkTypes(t *testing.T) {
	cfg := testBaseConfig()
	cfg.WorkTypes = []interfaces.WorkTypeConfig{
		{Name: "story", States: testStoryStates(), HandlingBehavior: []string{interfaces.WorkTypeHandlingBehaviorDefault}},
		{Name: "task", States: testStoryStates(), HandlingBehavior: []string{interfaces.WorkTypeHandlingBehaviorDefault}},
	}

	findings := NewConfigValidator().Validate(cfg).Findings
	assertFindingMatch(t, findings, "work-type-handling-behavior-unique-default", "workTypes", "expected at most one work type with handlingBehavior DEFAULT")
}

func TestConfigValidator_WorkTypeHandlingBehavior_RequiresDefaultWhenConfigured(t *testing.T) {
	cfg := testBaseConfig()
	cfg.WorkTypes = []interfaces.WorkTypeConfig{{Name: "story", States: testStoryStates()}}

	findings := NewConfigValidator(WithRequireDefaultHandlingWorkType()).Validate(cfg).Findings
	assertFindingMatch(t, findings, "work-type-handling-behavior-required-default", "workTypes", "expected exactly one work type with handlingBehavior DEFAULT")
}

func TestConfigValidator_WorkTypeHandlingBehavior_RejectsUnsupportedValues(t *testing.T) {
	cfg := testBaseConfig()
	cfg.WorkTypes = []interfaces.WorkTypeConfig{{
		Name:             "story",
		States:           testStoryStates(),
		HandlingBehavior: []string{"PROMPT"},
	}}

	findings := NewConfigValidator().Validate(cfg).Findings
	assertFindingMatch(t, findings, "work-type-handling-behavior-value", `workTypes[0](story).handlingBehavior[0]`, `unsupported handlingBehavior value "PROMPT"`)
}

func testStoryStates() []interfaces.StateConfig {
	return []interfaces.StateConfig{
		{Name: "init", Type: interfaces.StateTypeInitial},
		{Name: "complete", Type: interfaces.StateTypeTerminal},
	}
}
