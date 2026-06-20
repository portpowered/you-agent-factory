package subsystems

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestCalculateMutations_PreserveInput_KeepsConsumedPayloadForDownstreamWork(t *testing.T) {
	fixture := newCalculateMutationsFixture()
	fixture.consumed[0].Color.Payload = []byte("input-payload")
	fixture.consumed[0].Color.Content = []interfaces.WorkContentPart{{
		Type: interfaces.WorkContentPartTypeText,
		Text: "input-content",
	}}
	fixture.inputColors = tokenColorsFromTokens(fixture.consumed)

	mutations, err := fixture.calculateWithWorkstation(
		[]petri.Arc{{ID: "out", PlaceID: "wt-code:done"}},
		resolvedWorkResult{
			transitionID: "t1",
			outcome:      interfaces.OutcomeAccepted,
			output:       "worker-output",
		},
		&interfaces.FactoryWorkstationConfig{
			WorkPropagation: &interfaces.WorkPropagationConfig{
				Mode: interfaces.WorkPropagationModePreserveInput,
			},
		},
	)
	if err != nil {
		t.Fatalf("calculateMutations() error = %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("mutation count = %d, want 1", len(mutations))
	}
	if string(mutations[0].NewToken.Color.Payload) != "input-payload" {
		t.Fatalf("payload = %q, want input-payload", mutations[0].NewToken.Color.Payload)
	}
	if len(mutations[0].NewToken.Color.Content) != 1 || mutations[0].NewToken.Color.Content[0].Text != "input-content" {
		t.Fatalf("content = %#v, want preserved input content", mutations[0].NewToken.Color.Content)
	}
}

func TestCalculateMutations_OmittedWorkPropagation_UsesWorkerOutputPayload(t *testing.T) {
	fixture := newCalculateMutationsFixture()
	fixture.consumed[0].Color.Payload = []byte("input-payload")
	fixture.inputColors = tokenColorsFromTokens(fixture.consumed)

	mutations, err := fixture.calculate(
		[]petri.Arc{{ID: "out", PlaceID: "wt-code:done"}},
		resolvedWorkResult{
			transitionID: "t1",
			outcome:      interfaces.OutcomeAccepted,
			output:       "worker-output",
		},
	)
	if err != nil {
		t.Fatalf("calculateMutations() error = %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("mutation count = %d, want 1", len(mutations))
	}
	if string(mutations[0].NewToken.Color.Payload) != "worker-output" {
		t.Fatalf("payload = %q, want worker-output", mutations[0].NewToken.Color.Payload)
	}
}

func (f calculateMutationsFixture) calculateWithWorkstation(
	arcs []petri.Arc,
	result resolvedWorkResult,
	workstation *interfaces.FactoryWorkstationConfig,
) ([]interfaces.MarkingMutation, error) {
	return calculateMutations(mutationCalculationInput{
		transition:  f.transition,
		workstation: workstation,
		arcs:        arcs,
		consumed:    f.consumed,
		result:      result,
		now:         f.now,
		history:     f.baseHistory,
		inputColors: f.inputColors,
		transformer: f.transformer,
	})
}
