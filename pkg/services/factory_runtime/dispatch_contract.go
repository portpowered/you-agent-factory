package factory

import factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"

// Runtime-owned dispatch vocabulary published at the Factory Runtime root.
// Runtime and Recordings dispatch seams consume these aliases rather than
// treating the vocabulary as Factory Definitions-owned peer contract surface.
type (
	ActiveThrottlePause                   = factorycontracts.ActiveThrottlePause
	CompletedDispatch                     = factorycontracts.CompletedDispatch
	DispatchConsumedWorkRef               = factorycontracts.DispatchConsumedWorkRef
	DispatchEntry                         = factorycontracts.DispatchEntry
	DispatchReconciliationSource          = factorycontracts.DispatchReconciliationSource
	DispatchRecord                        = factorycontracts.DispatchRecord
	DispatchRequestEventMetadata          = factorycontracts.DispatchRequestEventMetadata
	DispatchResourceRef                   = factorycontracts.DispatchResourceRef
	FactoryDispatchKind                   = factorycontracts.FactoryDispatchKind
	FactoryDispatchRecord                 = factorycontracts.FactoryDispatchRecord
	FactoryDispatchStatus                 = factorycontracts.FactoryDispatchStatus
	FactoryDispatchUsage                  = factorycontracts.FactoryDispatchUsage
	FactoryDispatchWarning                = factorycontracts.FactoryDispatchWarning
	FactorySessionChildDispatchCounts     = factorycontracts.FactorySessionChildDispatchCounts
	FactorySessionDispatchFailureDetail   = factorycontracts.FactorySessionDispatchFailureDetail
	FactorySessionDispatchJavaScriptState = factorycontracts.FactorySessionDispatchJavaScriptState
	FactorySessionDispatchPetriState      = factorycontracts.FactorySessionDispatchPetriState
	FactorySessionDispatchState           = factorycontracts.FactorySessionDispatchState
	FactorySessionDispatchUsage           = factorycontracts.FactorySessionDispatchUsage
	FactorySessionDispatchWarning         = factorycontracts.FactorySessionDispatchWarning
)

const (
	DispatchReconciliationSourceProviderSession = factorycontracts.DispatchReconciliationSourceProviderSession
	DispatchReconciliationSourceStreamReplay    = factorycontracts.DispatchReconciliationSourceStreamReplay
	FactoryDispatchKindJavaScriptAgent          = factorycontracts.FactoryDispatchKindJavaScriptAgent
	FactoryDispatchKindJavaScriptScript         = factorycontracts.FactoryDispatchKindJavaScriptScript
	FactoryDispatchKindJavaScriptSynthesize     = factorycontracts.FactoryDispatchKindJavaScriptSynthesize
	FactoryDispatchKindJavaScriptSystem         = factorycontracts.FactoryDispatchKindJavaScriptSystem
	FactoryDispatchKindJavaScriptTool           = factorycontracts.FactoryDispatchKindJavaScriptTool
	FactoryDispatchKindJavaScriptVerify         = factorycontracts.FactoryDispatchKindJavaScriptVerify
	FactoryDispatchKindPetriTransition          = factorycontracts.FactoryDispatchKindPetriTransition
	FactoryDispatchStatusCompleted              = factorycontracts.FactoryDispatchStatusCompleted
	FactoryDispatchStatusFailed                 = factorycontracts.FactoryDispatchStatusFailed
	FactoryDispatchStatusInterrupted            = factorycontracts.FactoryDispatchStatusInterrupted
	FactoryDispatchStatusQueued                 = factorycontracts.FactoryDispatchStatusQueued
	FactoryDispatchStatusRunning                = factorycontracts.FactoryDispatchStatusRunning
)
