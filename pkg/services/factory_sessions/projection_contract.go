package factorysessions

import (
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// ProjectionContext carries the detached runtime inputs needed to project one
// live Factory Session.
type ProjectionContext struct {
	Session                *ScopedLiveSessionSummary
	FactorySessionID       string
	FactoryCfg             *interfaces.FactoryConfig
	Snapshot               *interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *factory.RuntimeNet]
	Observation            factory.Observation
	LifecycleControlStatus string
	BackendScopeID         string
	LogicalSessionKeyID    string
	NormalizedTarget       *RuntimeLogicalTarget
	RuntimeStartedAt       time.Time
	Enabled                []interfaces.EnabledTransition
	JavaScript             *interfaces.FactorySessionJavaScriptRuntimeState
	JavaScriptSession      *interfaces.FactoryWorldSessionBracketState
	JavaScriptCheckpoints  []interfaces.JavaScriptCheckpointRecord
	Now                    time.Time
}

// ReadProjection carries one live Factory Session read and records whether its
// runtime projection was available.
type ReadProjection struct {
	Context          ProjectionContext
	Runtime          RuntimeProjection
	RuntimeAvailable bool
}

// SessionProjection is the Factory Sessions-owned result for one live session
// detail read.
type SessionProjection struct {
	Context ProjectionContext
	Runtime RuntimeProjection
}
