package factory

import (
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
)

// RuntimeNet is the public topology observation consumed by Recordings
// projection implementations.
type RuntimeNet = state.Net

// Net is the public Factory Runtime topology read model.
type Net = state.Net

// RuntimeResourceDefinition is one runtime-projected resource definition.
type RuntimeResourceDefinition = state.ResourceDef
type ResourceDef = state.ResourceDef

// RuntimeWorkType is one runtime-projected Work type.
type RuntimeWorkType = state.WorkType

// WorkType is one public Factory Runtime work-type topology read model.
type WorkType = state.WorkType

// StateCategory is the runtime classification used by historical projections.
type StateCategory = state.StateCategory
type StateDefinition = state.StateDefinition
type GlobalLimits = state.GlobalLimits

const (
	StateCategoryInitial    = state.StateCategoryInitial
	StateCategoryProcessing = state.StateCategoryProcessing
	StateCategoryTerminal   = state.StateCategoryTerminal
	StateCategoryFailed     = state.StateCategoryFailed
)

func SplitPlaceID(placeID string) (string, string) {
	return state.SplitPlaceID(placeID)
}

func CategoryForState(workTypes map[string]*WorkType, workTypeID string, stateName string) StateCategory {
	return state.CategoryForState(workTypes, workTypeID, stateName)
}

func SnapshotHasActiveWork(snapshot *StateSnapshot) bool {
	return state.SnapshotHasActiveWork(snapshot)
}

var PlaceID = state.PlaceID
var ValidStatesByType = state.ValidStatesByType
var NormalizeTransitionTopology = state.NormalizeTransitionTopology
var NewEngineStateSnapshot = state.NewEngineStateSnapshot
var GenerateResourcePlaces = state.GenerateResourcePlaces
var ProjectInitialStructure = state.ProjectInitialStructure
