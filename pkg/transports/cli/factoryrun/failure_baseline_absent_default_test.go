package factoryrun

import (
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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

	result, err := promptRunValidationFailureValidator{}.ValidateEffectiveDefinition(
		t.Context(),
		interfaces.EffectiveDefinitionValidationRequest{Config: cfg},
	)
	if err != nil {
		t.Fatalf("ValidateEffectiveDefinition: %v", err)
	}
	if len(result.Targets) != 1 || !strings.Contains(result.Targets[0].Message, "handlingBehavior DEFAULT") {
		t.Fatalf("result = %#v, want DEFAULT handling guidance", result)
	}
}

func failureBaselineStoryStates() []interfaces.StateConfig {
	return []interfaces.StateConfig{
		{Name: "init", Type: interfaces.StateTypeInitial},
		{Name: "complete", Type: interfaces.StateTypeTerminal},
	}
}
