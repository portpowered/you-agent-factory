package sessionprojection

import (
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/legacysnapshot"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
)

// ProjectionBuildInput contains the owner-private mutable runtime facts needed
// to build one detached Factory Session projection context.
type ProjectionBuildInput struct {
	Session             *livesession.LiveSession
	RuntimeConfig       interfaces.RuntimeConfigLookup
	Snapshot            *legacysnapshot.Snapshot
	Observation         factoryruntime.Observation
	BackendScopeID      string
	LogicalSessionKey   string
	NormalizedTarget    *factorysessions.RuntimeLogicalTarget
	RuntimeStartedAt    time.Time
	CheckpointStore     factoryruntime.JavaScriptCheckpointStore
	Events              []interfaces.FactoryEvent
	WorldStateProjector factoryruntime.WorldStateProjector
	Now                 time.Time
}

type ProjectionContext = factorysessions.ProjectionContext
type RuntimeLogicalTarget = factorysessions.RuntimeLogicalTarget
type RuntimeProjection = factorysessions.RuntimeProjection
type RuntimeBudgets = factorysessions.RuntimeBudgets
type RuntimeLifecycle = factorysessions.RuntimeLifecycle
type RuntimeStreamIdentity = factorysessions.RuntimeStreamIdentity
type RuntimeProgress = factorysessions.RuntimeProgress
type RuntimeStatusCategories = factorysessions.RuntimeStatusCategories
type RuntimeUsage = factorysessions.RuntimeUsage
type RuntimeResourceUsage = factorysessions.RuntimeResourceUsage
type PetriRuntimeProjection = factorysessions.PetriRuntimeProjection
type PetriEnabledTransition = factorysessions.PetriEnabledTransition
type RuntimeToken = factorysessions.RuntimeToken
type RuntimeTokenHistory = factorysessions.RuntimeTokenHistory
type JavaScriptRuntimeProjection = factorysessions.JavaScriptRuntimeProjection
type StopSummary = factorysessions.StopSummary
type StopKind = factorysessions.StopKind
type StopDispatchStatus = factorysessions.StopDispatchStatus
type StopDispatchKind = factorysessions.StopDispatchKind
type StopFailureType = factorysessions.StopFailureType
type StopDispatchSummary = factorysessions.StopDispatchSummary
type StopFailureDetail = factorysessions.StopFailureDetail
type WorkStopSummaryRequest = factorysessions.WorkStopSummaryRequest
type WorkStopSummaryProjector = factorysessions.WorkStopSummaryProjector

const (
	StopKindPaused                  = factorysessions.StopKindPaused
	StopKindInterrupted             = factorysessions.StopKindInterrupted
	StopKindBlocked                 = factorysessions.StopKindBlocked
	StopKindNeedsHuman              = factorysessions.StopKindNeedsHuman
	StopDispatchStatusRunning       = factorysessions.StopDispatchStatusRunning
	StopDispatchStatusCompleted     = factorysessions.StopDispatchStatusCompleted
	StopDispatchStatusFailed        = factorysessions.StopDispatchStatusFailed
	StopDispatchStatusInterrupted   = factorysessions.StopDispatchStatusInterrupted
	StopDispatchKindPetriTransition = factorysessions.StopDispatchKindPetriTransition
	StopFailureTypeUnknown          = factorysessions.StopFailureTypeUnknown
)

func stringPointerOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
