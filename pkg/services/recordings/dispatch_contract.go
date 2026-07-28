package recordings

import factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

// Recordings-owned dispatch vocabulary used at the recording and projection
// boundary. Peers import these aliases from pkg/services/recordings rather than
// treating the vocabulary as Factory Definitions-owned peer contract surface.
type (
	CompletedDispatch                      = factorydefinitions.CompletedDispatch
	DispatchConsumedWorkRef                = factorydefinitions.DispatchConsumedWorkRef
	DispatchEntry                          = factorydefinitions.DispatchEntry
	DispatchReconciliationSource           = factorydefinitions.DispatchReconciliationSource
	DispatchRecord                         = factorydefinitions.DispatchRecord
	DispatchRequestEventMetadata           = factorydefinitions.DispatchRequestEventMetadata
	DispatchResourceRef                    = factorydefinitions.DispatchResourceRef
	FactoryDispatchKind                    = factorydefinitions.FactoryDispatchKind
	FactoryDispatchRecord                  = factorydefinitions.FactoryDispatchRecord
	FactoryDispatchStatus                  = factorydefinitions.FactoryDispatchStatus
	FactoryDispatchUsage                   = factorydefinitions.FactoryDispatchUsage
	FactoryDispatchWarning                 = factorydefinitions.FactoryDispatchWarning
	FactorySessionChildDispatchCounts      = factorydefinitions.FactorySessionChildDispatchCounts
	FactorySessionDispatchFailureDetail    = factorydefinitions.FactorySessionDispatchFailureDetail
	FactorySessionDispatchJavaScriptState  = factorydefinitions.FactorySessionDispatchJavaScriptState
	FactorySessionDispatchPetriState       = factorydefinitions.FactorySessionDispatchPetriState
	FactorySessionDispatchState            = factorydefinitions.FactorySessionDispatchState
	FactorySessionDispatchUsage            = factorydefinitions.FactorySessionDispatchUsage
	FactorySessionDispatchWarning          = factorydefinitions.FactorySessionDispatchWarning
)

const (
	DispatchReconciliationSourceProviderSession = factorydefinitions.DispatchReconciliationSourceProviderSession
	DispatchReconciliationSourceStreamReplay    = factorydefinitions.DispatchReconciliationSourceStreamReplay
	FactoryDispatchKindJavaScriptAgent          = factorydefinitions.FactoryDispatchKindJavaScriptAgent
	FactoryDispatchKindJavaScriptScript         = factorydefinitions.FactoryDispatchKindJavaScriptScript
	FactoryDispatchKindJavaScriptSynthesize     = factorydefinitions.FactoryDispatchKindJavaScriptSynthesize
	FactoryDispatchKindJavaScriptSystem         = factorydefinitions.FactoryDispatchKindJavaScriptSystem
	FactoryDispatchKindJavaScriptTool           = factorydefinitions.FactoryDispatchKindJavaScriptTool
	FactoryDispatchKindJavaScriptVerify         = factorydefinitions.FactoryDispatchKindJavaScriptVerify
	FactoryDispatchKindPetriTransition          = factorydefinitions.FactoryDispatchKindPetriTransition
	FactoryDispatchStatusCompleted              = factorydefinitions.FactoryDispatchStatusCompleted
	FactoryDispatchStatusFailed                 = factorydefinitions.FactoryDispatchStatusFailed
	FactoryDispatchStatusInterrupted            = factorydefinitions.FactoryDispatchStatusInterrupted
	FactoryDispatchStatusQueued                 = factorydefinitions.FactoryDispatchStatusQueued
	FactoryDispatchStatusRunning                = factorydefinitions.FactoryDispatchStatusRunning
)
