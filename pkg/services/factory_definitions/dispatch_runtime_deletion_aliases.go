package factorydefinitions

import contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"

// Deletion-only aliases retain temporary Factory Definitions root symbols for
// dispatch vocabulary rehomed to pkg/services/factory_runtime and
// pkg/services/recordings in CLN-DEF-CONTRACTS story 004. Peers must import
// Runtime or Recordings root dispatch contracts instead; remove this file when
// downstream consumers finish cutover.

type (
	ActiveThrottlePause                   = contracts.ActiveThrottlePause
	CompletedDispatch                     = contracts.CompletedDispatch
	DispatchConsumedWorkRef               = contracts.DispatchConsumedWorkRef
	DispatchEntry                         = contracts.DispatchEntry
	DispatchReconciliationSource          = contracts.DispatchReconciliationSource
	DispatchRecord                        = contracts.DispatchRecord
	DispatchRequestEventMetadata          = contracts.DispatchRequestEventMetadata
	DispatchResourceRef                   = contracts.DispatchResourceRef
	FactoryDispatchKind                   = contracts.FactoryDispatchKind
	FactoryDispatchRecord                 = contracts.FactoryDispatchRecord
	FactoryDispatchStatus                 = contracts.FactoryDispatchStatus
	FactoryDispatchUsage                  = contracts.FactoryDispatchUsage
	FactoryDispatchWarning                = contracts.FactoryDispatchWarning
	FactorySessionChildDispatchCounts     = contracts.FactorySessionChildDispatchCounts
	FactorySessionDispatchFailureDetail   = contracts.FactorySessionDispatchFailureDetail
	FactorySessionDispatchJavaScriptState = contracts.FactorySessionDispatchJavaScriptState
	FactorySessionDispatchPetriState      = contracts.FactorySessionDispatchPetriState
	FactorySessionDispatchState           = contracts.FactorySessionDispatchState
	FactorySessionDispatchUsage           = contracts.FactorySessionDispatchUsage
	FactorySessionDispatchWarning         = contracts.FactorySessionDispatchWarning
)

const (
	DispatchReconciliationSourceProviderSession = contracts.DispatchReconciliationSourceProviderSession
	DispatchReconciliationSourceStreamReplay    = contracts.DispatchReconciliationSourceStreamReplay
	FactoryDispatchKindJavaScriptAgent          = contracts.FactoryDispatchKindJavaScriptAgent
	FactoryDispatchKindJavaScriptScript         = contracts.FactoryDispatchKindJavaScriptScript
	FactoryDispatchKindJavaScriptSynthesize     = contracts.FactoryDispatchKindJavaScriptSynthesize
	FactoryDispatchKindJavaScriptSystem         = contracts.FactoryDispatchKindJavaScriptSystem
	FactoryDispatchKindJavaScriptTool           = contracts.FactoryDispatchKindJavaScriptTool
	FactoryDispatchKindJavaScriptVerify         = contracts.FactoryDispatchKindJavaScriptVerify
	FactoryDispatchKindPetriTransition          = contracts.FactoryDispatchKindPetriTransition
	FactoryDispatchStatusCompleted              = contracts.FactoryDispatchStatusCompleted
	FactoryDispatchStatusFailed                 = contracts.FactoryDispatchStatusFailed
	FactoryDispatchStatusInterrupted            = contracts.FactoryDispatchStatusInterrupted
	FactoryDispatchStatusQueued                 = contracts.FactoryDispatchStatusQueued
	FactoryDispatchStatusRunning                = contracts.FactoryDispatchStatusRunning
)
