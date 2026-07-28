package recordings

import (
	"errors"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

var (
	// ErrReplayRecordingNotFound reports that a replay load could not select a
	// recording through its Recordings-owned identity.
	ErrReplayRecordingNotFound = errors.New("replay recording not found")
	// ErrReplayRecordingNotFinalized reports that a selected recording is not
	// yet stable enough to replay.
	ErrReplayRecordingNotFinalized = errors.New("replay recording is not finalized")
	// ErrCorruptReplayInput reports malformed scope, identity, or canonical
	// ordering in detached replay facts.
	ErrCorruptReplayInput = errors.New("corrupt replay input")
	// ErrUnsupportedReplayPlan reports an unknown plan schema, timing mode, or
	// other unsupported neutral replay option.
	ErrUnsupportedReplayPlan = errors.New("unsupported replay plan")
	// ErrReplayPlanNotFound reports an unknown opaque replay handle.
	ErrReplayPlanNotFound = errors.New("replay plan not found")
)

// ReplayPlanSchemaVersion identifies the neutral replay-plan vocabulary.
type ReplayPlanSchemaVersion string

const ReplayPlanSchemaV1 ReplayPlanSchemaVersion = "recordings.replay-plan.v1"

// ReplayTimingMode selects implementation-neutral timing behavior. Order-only
// replay preserves canonical ordering without exposing clocks or timers.
type ReplayTimingMode string

const ReplayTimingOrderOnly ReplayTimingMode = "ORDER_ONLY"

// ReplayRecordingFacts is a detached selection of one recording's canonical
// facts. Events is an independent slice and contains no decoder, store, or
// runtime handles.
type ReplayRecordingFacts struct {
	RecordingID RecordingID
	Scope       CanonicalEventScope
	Events      []CanonicalEvent
}

// LoadReplayRecordingRequest selects one recording by its opaque identity.
type LoadReplayRecordingRequest struct {
	RecordingID RecordingID
}

// LoadReplayRecordingResult returns detached canonical facts for replay.
type LoadReplayRecordingResult struct {
	Recording ReplayRecordingFacts
}

// ReplayPlanHandle is an opaque Recordings-owned replay identity.
type ReplayPlanHandle string

// CreateReplayPlanRequest asks Recordings to validate and retain a neutral
// replay plan. ExpectedThrough, when present, makes divergence observable
// without exposing a runtime engine.
type CreateReplayPlanRequest struct {
	SchemaVersion   ReplayPlanSchemaVersion
	Timing          ReplayTimingMode
	Recording       ReplayRecordingFacts
	ExpectedThrough *CanonicalEventCursor
	SelectedTick    int
}

// ReplayPlanFacts is the detached public description of an opaque plan.
type ReplayPlanFacts struct {
	Handle        ReplayPlanHandle
	RecordingID   RecordingID
	Scope         CanonicalEventScope
	TotalEvents   int
	SchemaVersion ReplayPlanSchemaVersion
	Timing        ReplayTimingMode
}

// CreateReplayPlanResult reports the accepted neutral plan.
type CreateReplayPlanResult struct {
	Plan ReplayPlanFacts
}

// ReplayObservationKind identifies one deterministic replay observation.
type ReplayObservationKind string

const (
	ReplayProgress  ReplayObservationKind = "PROGRESS"
	ReplayCompleted ReplayObservationKind = "COMPLETED"
	ReplayDiverged  ReplayObservationKind = "DIVERGED"
)

// ReplayDivergenceFacts contains safe expected and observed ordering facts.
type ReplayDivergenceFacts struct {
	Expected CanonicalEventCursor
	Observed CanonicalEventCursor
}

// ReplayObservation is one detached progress, completion, or divergence fact.
// WorldState is reduced from the canonical prefix reported by ProcessedEvents.
type ReplayObservation struct {
	Kind            ReplayObservationKind
	Plan            ReplayPlanHandle
	ProcessedEvents int
	TotalEvents     int
	Through         *CanonicalEventCursor
	WorldState      WorldStateView
	Divergence      *ReplayDivergenceFacts
}

// ObserveReplayRequest advances and observes one opaque replay plan.
type ObserveReplayRequest struct {
	Plan ReplayPlanHandle
}

// ObserveReplayResult returns one deterministic detached observation.
type ObserveReplayResult struct {
	Observation ReplayObservation
}

// Recordings-owned legacy replay artifact vocabulary. Peers import these
// aliases from pkg/services/recordings rather than treating the vocabulary as
// Factory Definitions-owned peer contract surface.
type (
	CheckpointResumabilityStatus = factorydefinitions.CheckpointResumabilityStatus
	ReplayArtifact               = factorydefinitions.ReplayArtifact
	ReplayDiagnostics            = factorydefinitions.ReplayDiagnostics
	ReplayWallClockMetadata      = factorydefinitions.ReplayWallClockMetadata
)

const (
	CheckpointResumabilityStatusResumable = factorydefinitions.CheckpointResumabilityStatusResumable
)
