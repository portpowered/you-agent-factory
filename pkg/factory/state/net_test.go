package state

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
)

func TestPlaceID(t *testing.T) {
	tests := []struct {
		workTypeID string
		stateValue string
		want       string
	}{
		{"code-change", "init", "code-change:init"},
		{"code-change", "complete", "code-change:complete"},
		{"design-doc", "in-review", "design-doc:in-review"},
		{"gpu", "available", "gpu:available"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := PlaceID(tt.workTypeID, tt.stateValue)
			if got != tt.want {
				t.Errorf("PlaceID(%q, %q) = %q, want %q", tt.workTypeID, tt.stateValue, got, tt.want)
			}
		})
	}
}

func TestWorkTypeGeneratePlaces(t *testing.T) {
	wt := &WorkType{
		ID:   "code-change",
		Name: "Code Change",
		States: []StateDefinition{
			{Value: "init", Category: StateCategoryInitial},
			{Value: "in-progress", Category: StateCategoryProcessing},
			{Value: "complete", Category: StateCategoryTerminal},
			{Value: "failed", Category: StateCategoryFailed},
		},
	}

	places := wt.GeneratePlaces()

	if len(places) != 4 {
		t.Fatalf("expected 4 places, got %d", len(places))
	}

	expected := []struct {
		id     string
		typeID string
		state  string
	}{
		{"code-change:init", "code-change", "init"},
		{"code-change:in-progress", "code-change", "in-progress"},
		{"code-change:complete", "code-change", "complete"},
		{"code-change:failed", "code-change", "failed"},
	}

	for i, e := range expected {
		p := places[i]
		if p.ID != e.id {
			t.Errorf("place[%d].ID = %q, want %q", i, p.ID, e.id)
		}
		if p.TypeID != e.typeID {
			t.Errorf("place[%d].TypeID = %q, want %q", i, p.TypeID, e.typeID)
		}
		if p.State != e.state {
			t.Errorf("place[%d].State = %q, want %q", i, p.State, e.state)
		}
	}
}

func TestGenerateResourcePlaces(t *testing.T) {
	def := &ResourceDef{
		ID:       "gpu",
		Name:     "GPU",
		Capacity: 3,
	}

	place, tokens := GenerateResourcePlaces(def)

	// Verify place
	if place.ID != "gpu:available" {
		t.Errorf("place.ID = %q, want %q", place.ID, "gpu:available")
	}
	if place.TypeID != "gpu" {
		t.Errorf("place.TypeID = %q, want %q", place.TypeID, "gpu")
	}
	if place.State != "available" {
		t.Errorf("place.State = %q, want %q", place.State, "available")
	}

	// Verify tokens
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d", len(tokens))
	}

	for i, tok := range tokens {
		if tok.PlaceID != "gpu:available" {
			t.Errorf("token[%d].PlaceID = %q, want %q", i, tok.PlaceID, "gpu:available")
		}
		if tok.Color.WorkTypeID != "gpu" {
			t.Errorf("token[%d].Color.WorkTypeID = %q, want %q", i, tok.Color.WorkTypeID, "gpu")
		}
		if tok.Color.DataType != interfaces.DataTypeResource {
			t.Errorf("token[%d].Color.DataType = %q, want %q", i, tok.Color.DataType, interfaces.DataTypeResource)
		}
		if tok.CreatedAt.IsZero() {
			t.Errorf("token[%d].CreatedAt should not be zero", i)
		}
	}
}

func TestGenerateResourcePlacesZeroCapacity(t *testing.T) {
	def := &ResourceDef{
		ID:       "scanner",
		Name:     "Scanner",
		Capacity: 0,
	}

	place, tokens := GenerateResourcePlaces(def)

	if place.ID != "scanner:available" {
		t.Errorf("place.ID = %q, want %q", place.ID, "scanner:available")
	}
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens for zero capacity, got %d", len(tokens))
	}
}

func TestNormalizeTransitionTopology_AddsRepeaterAndDefaultFailureArcs(t *testing.T) {
	net := &Net{
		Places: map[string]*petri.Place{
			"task:init":        {ID: "task:init", TypeID: "task", State: "init"},
			"task:complete":    {ID: "task:complete", TypeID: "task", State: "complete"},
			"task:failed":      {ID: "task:failed", TypeID: "task", State: "failed"},
			"worker:available": {ID: "worker:available", TypeID: "worker", State: "available"},
		},
		Transitions: map[string]*petri.Transition{
			"repeat": {
				ID: "repeat",
				InputArcs: []petri.Arc{
					{PlaceID: "task:init", Direction: petri.ArcInput},
					{PlaceID: "worker:available", Direction: petri.ArcInput},
				},
				OutputArcs: []petri.Arc{
					{PlaceID: "task:complete", Direction: petri.ArcOutput},
					{PlaceID: "worker:available", Direction: petri.ArcOutput},
				},
			},
		},
		WorkTypes: map[string]*WorkType{
			"task": {
				ID: "task",
				States: []StateDefinition{
					{Value: "init", Category: StateCategoryInitial},
					{Value: "complete", Category: StateCategoryTerminal},
					{Value: "failed", Category: StateCategoryFailed},
				},
			},
		},
	}

	NormalizeTransitionTopology(net, runtimefixtures.RuntimeWorkstationLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"repeat": {Name: "repeat", Kind: interfaces.WorkstationKindRepeater},
		},
	})

	transition := net.Transitions["repeat"]
	if len(transition.RejectionArcs) != 1 {
		t.Fatalf("expected 1 rejection arc, got %d", len(transition.RejectionArcs))
	}
	if transition.RejectionArcs[0].PlaceID != "task:init" {
		t.Fatalf("rejection arc PlaceID = %q, want %q", transition.RejectionArcs[0].PlaceID, "task:init")
	}
	if len(transition.FailureArcs) != 1 {
		t.Fatalf("expected 1 failure arc, got %d", len(transition.FailureArcs))
	}
	if transition.FailureArcs[0].PlaceID != "task:failed" {
		t.Fatalf("failure arc PlaceID = %q, want %q", transition.FailureArcs[0].PlaceID, "task:failed")
	}
}
