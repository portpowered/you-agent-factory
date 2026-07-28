package factorydefinitions

import contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"

// Deletion-only aliases retain temporary Factory Definitions root symbols for
// replay artifact vocabulary rehomed to pkg/services/recordings in CLN-DEF-CONTRACTS
// story 004. Peers must import Recordings root replay contracts instead; remove
// this file when downstream consumers finish cutover.

type (
	CheckpointResumabilityStatus = contracts.CheckpointResumabilityStatus
	ReplayArtifact               = contracts.ReplayArtifact
	ReplayDiagnostics            = contracts.ReplayDiagnostics
	ReplayWallClockMetadata      = contracts.ReplayWallClockMetadata
)

const (
	CheckpointResumabilityStatusResumable = contracts.CheckpointResumabilityStatusResumable
)
