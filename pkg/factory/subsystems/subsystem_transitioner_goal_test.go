package subsystems

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
)

func TestCalculateMutations_PackagedGoalReplacesTerminalContentWithSummary(t *testing.T) {
	fixture := newCalculateMutationsFixture()
	workstation := &interfaces.FactoryWorkstationConfig{
		Name:           goal.PackagedInvokeWorkstationName,
		Type:           interfaces.WorkstationTypeModel,
		WorkerTypeName: "goal-executor",
	}
	workerOutput := "Final goal summary.\nCOMPLETE"

	mutations, err := calculateMutations(mutationCalculationInput{
		transition:  fixture.transition,
		workstation: workstation,
		arcs: []petri.Arc{{
			PlaceID: "goal:complete",
		}},
		consumed:    fixture.consumed,
		result:      resolvedWorkResult{outcome: interfaces.OutcomeAccepted, output: workerOutput},
		now:         fixture.now,
		history:     fixture.baseHistory,
		inputColors: fixture.inputColors,
		transformer: fixture.transformer,
		runtimeConfig: runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers: map[string]*interfaces.WorkerConfig{
				"goal-executor": {
					Name:      "goal-executor",
					StopToken: "COMPLETE",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("calculateMutations: %v", err)
	}
	if len(mutations) != 1 {
		t.Fatalf("mutation count = %d, want 1", len(mutations))
	}

	token := mutations[0].NewToken
	if len(token.Color.Content) != 1 || token.Color.Content[0].Type != interfaces.WorkContentPartTypeText {
		t.Fatalf("terminal content = %#v, want one text summary part", token.Color.Content)
	}
	if token.Color.Content[0].Text != "Final goal summary." {
		t.Fatalf("terminal content = %q, want worker summary without stop token", token.Color.Content[0].Text)
	}
	if len(token.Color.Payload) != 0 {
		t.Fatalf("terminal payload = %q, want cleared so submitted input does not leak", string(token.Color.Payload))
	}
}
