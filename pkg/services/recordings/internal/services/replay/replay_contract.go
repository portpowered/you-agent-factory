package recordingsreplay

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

// Recordings-owned neutral replay plan vocabulary. Peers import these types
// from pkg/services/recordings rather than this private subservice package.
var (
	ErrReplayRecordingNotFound     = recordings.ErrReplayRecordingNotFound
	ErrReplayRecordingNotFinalized = recordings.ErrReplayRecordingNotFinalized
	ErrCorruptReplayInput          = recordings.ErrCorruptReplayInput
	ErrUnsupportedReplayPlan         = recordings.ErrUnsupportedReplayPlan
	ErrReplayPlanNotFound          = recordings.ErrReplayPlanNotFound
)

type (
	ReplayPlanSchemaVersion = recordings.ReplayPlanSchemaVersion
	ReplayTimingMode        = recordings.ReplayTimingMode
	ReplayRecordingFacts    = recordings.ReplayRecordingFacts
	LoadReplayRecordingRequest = recordings.LoadReplayRecordingRequest
	LoadReplayRecordingResult  = recordings.LoadReplayRecordingResult
	ReplayPlanHandle        = recordings.ReplayPlanHandle
	CreateReplayPlanRequest = recordings.CreateReplayPlanRequest
	ReplayPlanFacts         = recordings.ReplayPlanFacts
	CreateReplayPlanResult  = recordings.CreateReplayPlanResult
	ReplayObservationKind   = recordings.ReplayObservationKind
	ReplayDivergenceFacts   = recordings.ReplayDivergenceFacts
	ReplayObservation       = recordings.ReplayObservation
	ObserveReplayRequest    = recordings.ObserveReplayRequest
	ObserveReplayResult     = recordings.ObserveReplayResult
)

const (
	ReplayPlanSchemaV1  = recordings.ReplayPlanSchemaV1
	ReplayTimingOrderOnly = recordings.ReplayTimingOrderOnly
	ReplayProgress      = recordings.ReplayProgress
	ReplayCompleted     = recordings.ReplayCompleted
	ReplayDiverged      = recordings.ReplayDiverged
)

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
