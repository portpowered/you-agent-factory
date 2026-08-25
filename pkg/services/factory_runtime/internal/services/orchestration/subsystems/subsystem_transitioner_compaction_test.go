package subsystems

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
)

func TestTerminalMutationFacts_UseTopologyReachabilityAndConsumeFacts(t *testing.T) {
	net := compactionTestNet()

	tests := []struct {
		name          string
		mutation      interfaces.MarkingMutation
		wantTerminal  bool
		wantReachable bool
	}{
		{
			name:         "unreferenced terminal destination",
			mutation:     interfaces.MarkingMutation{Type: interfaces.MutationMove, ToPlace: "task:done"},
			wantTerminal: true,
		},
		{
			name:          "observed terminal dependency",
			mutation:      interfaces.MarkingMutation{Type: interfaces.MutationMove, ToPlace: "task:observed"},
			wantTerminal:  true,
			wantReachable: true,
		},
		{
			name:         "processing destination",
			mutation:     interfaces.MarkingMutation{Type: interfaces.MutationMove, ToPlace: "task:processing"},
			wantTerminal: false,
		},
		{
			name:         "consumed terminal token",
			mutation:     interfaces.MarkingMutation{Type: interfaces.MutationConsume, FromPlace: "task:observed"},
			wantTerminal: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			terminal, reachable := terminalMutationFacts(net, test.mutation)
			if terminal != test.wantTerminal || reachable != test.wantReachable {
				t.Fatalf("terminalMutationFacts() = (%t, %t), want (%t, %t)", terminal, reachable, test.wantTerminal, test.wantReachable)
			}
		})
	}
}

func compactionTestNet() *state.Net {
	workType := &state.WorkType{
		ID: "task",
		States: []state.StateDefinition{
			{Value: "processing", Category: state.StateCategoryProcessing},
			{Value: "done", Category: state.StateCategoryTerminal},
			{Value: "observed", Category: state.StateCategoryTerminal},
		},
	}
	places := make(map[string]*petri.Place)
	for _, place := range workType.GeneratePlaces() {
		places[place.ID] = place
	}
	return &state.Net{
		Places:    places,
		WorkTypes: map[string]*state.WorkType{"task": workType},
		Transitions: map[string]*petri.Transition{
			"observe-terminal": {
				ID: "observe-terminal",
				InputArcs: []petri.Arc{{
					PlaceID: "task:observed", Direction: petri.ArcInput, Mode: interfaces.ArcModeObserve,
				}},
			},
		},
	}
}
