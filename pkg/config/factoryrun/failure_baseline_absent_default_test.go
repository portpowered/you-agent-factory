package factoryrun

import (
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

// Hermetic S02 failure-baseline fixtures for one-shot you run --factory prompt
// submission when the factory lacks the required handlingBehavior DEFAULT work type.

func TestFailureBaseline_AbsentDefault_FactoryPromptRunRejectsMissingDefaultHandlingWorkType(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name:   "story",
			States: failureBaselineStoryStates(),
		}},
	}

	err := ValidateFactoryForPromptRun(cfg)
	if err == nil {
		t.Fatal("expected validation error for missing DEFAULT handling work type")
	}
	if !strings.Contains(err.Error(), "handlingBehavior DEFAULT") {
		t.Fatalf("error = %q, want DEFAULT handling guidance", err.Error())
	}
}

func failureBaselineStoryStates() []interfaces.StateConfig {
	return []interfaces.StateConfig{
		{Name: "init", Type: interfaces.StateTypeInitial},
		{Name: "complete", Type: interfaces.StateTypeTerminal},
	}
}
