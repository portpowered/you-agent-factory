package events

import (
	ledgerevents "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/events"
)

var (
	ErrReconnectCursorNotFound = ledgerevents.ErrReconnectCursorNotFound

	BuildCanonicalReconnectReplay      = ledgerevents.BuildCanonicalReconnectReplay
	FactoryStateToDurableLifecycleStatus = ledgerevents.FactoryStateToDurableLifecycleStatus
	NewRuntimeLedger                   = ledgerevents.NewRuntimeLedger
	NewFactoryEventHistory             = ledgerevents.NewFactoryEventHistory
)

type (
	ArtifactCreatedInput               = ledgerevents.ArtifactCreatedInput
	DispatchInterruptedInput           = ledgerevents.DispatchInterruptedInput
	DispatchQueuedInput                = ledgerevents.DispatchQueuedInput
	DispatchReconciledInput            = ledgerevents.DispatchReconciledInput
	FactoryEventHistory                = ledgerevents.FactoryEventHistory
	OrchestratorCheckpointWrittenInput = ledgerevents.OrchestratorCheckpointWrittenInput
	OrchestratorPhaseChangedInput      = ledgerevents.OrchestratorPhaseChangedInput
	RuntimeLedgerFactory               = ledgerevents.RuntimeLedgerFactory
	SessionLifecycleCompleteInput      = ledgerevents.SessionLifecycleCompleteInput
	SessionLifecycleControlInput       = ledgerevents.SessionLifecycleControlInput
	SessionLifecycleResultInput        = ledgerevents.SessionLifecycleResultInput
	SessionLifecycleStartInput         = ledgerevents.SessionLifecycleStartInput
)
