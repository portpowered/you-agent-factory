package state

import (
	"time"

	"github.com/portpowered/infinite-you/pkg/petri"
)

// Net is the complete CPN definition — places, transitions, arcs.
// This is loaded from the workflow config and validated at load time.
// The net topology is static; dynamism lives in the tokens.
type Net struct {
	ID          string                       `json:"id"`
	Places      map[string]*petri.Place      `json:"places"`
	Transitions map[string]*petri.Transition `json:"transitions"`
	Arcs        map[string]*petri.Arc        `json:"arcs"`
	WorkTypes   map[string]*WorkType         `json:"work_types"`
	Resources   map[string]*ResourceDef      `json:"resources"`
	InputTypes  map[string]*InputType        `json:"input_types,omitempty"`
	Limits      GlobalLimits                 `json:"limits"`

	// FanoutGroups maps a spawning transition ID to its fanout count place ID.
	// When the transitioner processes spawned work from a transition in this map,
	// it creates a guard token in the count place carrying the expected child count.
	FanoutGroups map[string]string `json:"fanout_groups,omitempty"`
}

// InputType defines a named input type available on the factory.
// The "default" type is always implicitly available and requires no structured payload.
type InputType struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // "default"
}

// StateCategoryForPlace returns the StateCategory for the place identified by placeID.
// It looks up the place's TypeID and State, then finds the matching StateDefinition
// in the work type. Returns StateCategoryProcessing if the place or work type is unknown.
func (n *Net) StateCategoryForPlace(placeID string) StateCategory {
	p, ok := n.Places[placeID]
	if !ok {
		return StateCategoryProcessing
	}
	wt, ok := n.WorkTypes[p.TypeID]
	if !ok {
		return StateCategoryProcessing
	}
	for _, sd := range wt.States {
		if sd.Value == p.State {
			return sd.Category
		}
	}
	return StateCategoryProcessing
}

// GlobalLimits defines net-wide execution constraints that apply to all tokens.
// These are the outermost safety net — individual transitions and cycles can
// have tighter limits, but these catch anything that slips through.
type GlobalLimits struct {
	MaxTokenAge    time.Duration `json:"max_token_age"`    // max time a token can exist before being force-failed (0 = no limit)
	MaxTotalVisits int           `json:"max_total_visits"` // max total transition visits for any single token (0 = no limit)
}
