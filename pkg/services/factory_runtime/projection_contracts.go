package factory

import (
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
)

type RuntimeNet = state.Net
type Net = state.Net
type RuntimeResourceDefinition = state.ResourceDef
type ResourceDef = state.ResourceDef
type RuntimeWorkType = state.WorkType
type WorkType = state.WorkType
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

// ObservationScope selects which live observation views peers request. Empty
// scope means ObservationScopeFull. Scopes stay orchestration-neutral and must
// not name Petri markings, nets, tokens, or JavaScript strategy records.
type ObservationScope string

const (
	// ObservationScopeFull returns status, progress, dispatches, results,
	// resources, and retained live health.
	ObservationScopeFull ObservationScope = "FULL"
	// ObservationScopeStatus returns live operational status.
	ObservationScopeStatus ObservationScope = "STATUS"
	// ObservationScopeProgress returns live progress counters.
	ObservationScopeProgress ObservationScope = "PROGRESS"
	// ObservationScopeDispatches returns in-flight dispatch summaries.
	ObservationScopeDispatches ObservationScope = "DISPATCHES"
	// ObservationScopeResults returns recent result views.
	ObservationScopeResults ObservationScope = "RESULTS"
	// ObservationScopeResources returns resource utilization views.
	ObservationScopeResources ObservationScope = "RESOURCES"
	// ObservationScopeHealth returns retained live health that cannot be
	// reconstructed solely as Recordings projections.
	ObservationScopeHealth ObservationScope = "HEALTH"
)

// ObservationStatus is the plain live operational status vocabulary.
type ObservationStatus string

const (
	// ObservationStatusActive indicates the instance is actively processing work.
	ObservationStatusActive ObservationStatus = "ACTIVE"
	// ObservationStatusIdle indicates the instance is available but not actively
	// processing work.
	ObservationStatusIdle ObservationStatus = "IDLE"
	// ObservationStatusFinished indicates the instance has finished all work.
	ObservationStatusFinished ObservationStatus = "FINISHED"
)

// ObserveRequest is the plain observation input published at the Runtime root.
type ObserveRequest struct {
	Scope ObservationScope
}

// ObservationProgress is the plain live progress view.
type ObservationProgress struct {
	InFlightDispatchCount int
	TickCount             int
	TotalWorkCount        int
	WorkCategories        ObservationWorkCategories
}

// ObservationWorkCategories counts customer Work by lifecycle category.
type ObservationWorkCategories struct {
	Failed     int
	Initial    int
	Processing int
	Terminal   int
}

// ObservationDispatchSummary is an orchestration-neutral in-flight dispatch view.
type ObservationDispatchSummary struct {
	DispatchID      string
	WorkIDs         []string
	WorkstationName string
	Status          string
}

// ObservationResultView is an orchestration-neutral result view peers can consume.
type ObservationResultView struct {
	DispatchID string
	WorkID     string
	Outcome    string
}

// ObservationResourceView is an orchestration-neutral resource utilization view.
type ObservationResourceView struct {
	ResourceID     string
	ResourceName   string
	ResourceType   string
	InUseCount     int
	AvailableCount int
}

// ObservationHealth retains live health that cannot be reconstructed solely as
// Recordings projections (stream generation, lifecycle control, uptime).
type ObservationHealth struct {
	FactoryState           string
	LifecycleControlStatus string
	StreamGenerationID     string
	Uptime                 time.Duration
}

// Observation is the detached orchestration-neutral observation value published
// at the Runtime root. It is the peer source of truth for live observation and
// must not carry EngineStateSnapshot[PetriMarkingSnapshot,*Net]-shaped aliases
// or JavaScript runtime-record types.
type Observation struct {
	Status             ObservationStatus
	Progress           ObservationProgress
	InFlightDispatches []ObservationDispatchSummary
	Results            []ObservationResultView
	Resources          []ObservationResourceView
	Health             ObservationHealth
}

// ObserveResult is the plain observation success shape published at the Runtime root.
type ObserveResult struct {
	Observation Observation
}
