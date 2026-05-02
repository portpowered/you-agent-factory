package state

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/petri"
)

// WorkType defines a type of work and its state machine.
type WorkType struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	States []StateDefinition `json:"states"`
}

// StateDefinition defines a single state within a work type.
type StateDefinition struct {
	Value    string        `json:"value"`
	Category StateCategory `json:"category"` // INITIAL, PROCESSING, TERMINAL, FAILED
}

// StateCategory classifies states within a work type.
type StateCategory string

const (
	StateCategoryInitial    StateCategory = "INITIAL"
	StateCategoryProcessing StateCategory = "PROCESSING"
	StateCategoryTerminal   StateCategory = "TERMINAL"
	StateCategoryFailed     StateCategory = "FAILED"
)

// PlaceID returns the derived place ID for a work type state.
// Convention: '{work_type_id}:{state_value}'.
func PlaceID(workTypeID, stateValue string) string {
	return fmt.Sprintf("%s:%s", workTypeID, stateValue)
}

// SplitPlaceID extracts work type and state from a place ID.
// Place IDs follow the convention '{work_type}:{state}'.
func SplitPlaceID(placeID string) (workType, stateValue string) {
	idx := strings.LastIndexByte(placeID, ':')
	if idx < 0 {
		return placeID, ""
	}
	return placeID[:idx], placeID[idx+1:]
}

// CategoryForState looks up the category of a work type state from a topology.
// Returns StateCategoryProcessing if the work type or state is unknown.
func CategoryForState(workTypes map[string]*WorkType, workTypeID, stateValue string) StateCategory {
	wt, ok := workTypes[workTypeID]
	if !ok {
		return StateCategoryProcessing
	}
	for _, sd := range wt.States {
		if sd.Value == stateValue {
			return sd.Category
		}
	}
	return StateCategoryProcessing
}

// ValidStatesByType returns the configured state names for each work type.
func ValidStatesByType(workTypes map[string]*WorkType) map[string]map[string]bool {
	out := make(map[string]map[string]bool, len(workTypes))
	for workTypeID, wt := range workTypes {
		states := make(map[string]bool, len(wt.States))
		for _, stateDef := range wt.States {
			states[stateDef.Value] = true
		}
		out[workTypeID] = states
	}
	return out
}

// GeneratePlaces produces a Place for each state in the work type.
// Place IDs follow the convention '{work_type_id}:{state_value}'.
func (wt *WorkType) GeneratePlaces() []*petri.Place {
	places := make([]*petri.Place, 0, len(wt.States))
	for _, s := range wt.States {
		places = append(places, &petri.Place{
			ID:     PlaceID(wt.ID, s.Value),
			TypeID: wt.ID,
			State:  s.Value,
		})
	}
	return places
}
