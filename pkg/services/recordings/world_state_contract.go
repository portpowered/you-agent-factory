package recordings

import factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"

// Recordings-owned Factory world-state projection vocabulary. Peers import
// these aliases from pkg/services/recordings rather than treating the vocabulary
// as Factory Definitions-owned peer contract surface. Implementation debt
// remains in the contracts mega-barrel until CLN-DEF-CONTRACTS story 007
// deletes it.
type (
	ActiveThrottlePause                    = factorycontracts.ActiveThrottlePause
	FactoryPlace                           = factorycontracts.FactoryPlace
	FactoryPlaceOccupancy                  = factorycontracts.FactoryPlaceOccupancy
	FactoryState                           = factorycontracts.FactoryState
	FactoryStateDefinition                 = factorycontracts.FactoryStateDefinition
	FactoryTerminalWork                    = factorycontracts.FactoryTerminalWork
	FactoryWorkType                        = factorycontracts.FactoryWorkType
	FactoryWorker                          = factorycontracts.FactoryWorker
	FactoryWorkstation                     = factorycontracts.FactoryWorkstation
	FactoryWorkstationRef                  = factorycontracts.FactoryWorkstationRef
	FactoryWorldActiveExecution            = factorycontracts.FactoryWorldActiveExecution
	FactoryWorldActivity                   = factorycontracts.FactoryWorldActivity
	FactoryWorldAgentRunResponse           = factorycontracts.FactoryWorldAgentRunResponse
	FactoryWorldDispatch                   = factorycontracts.FactoryWorldDispatch
	FactoryWorldDispatchCompletion         = factorycontracts.FactoryWorldDispatchCompletion
	FactoryWorldFailureDetail              = factorycontracts.FactoryWorldFailureDetail
	FactoryWorldInferenceAttempt           = factorycontracts.FactoryWorldInferenceAttempt
	FactoryWorldJavaScriptChildDispatchCounts = factorycontracts.FactoryWorldJavaScriptChildDispatchCounts
	FactoryWorldJavaScriptProjection       = factorycontracts.FactoryWorldJavaScriptProjection
	FactoryWorldPlaceRef                   = factorycontracts.FactoryWorldPlaceRef
	FactoryWorldProviderSessionRecord      = factorycontracts.FactoryWorldProviderSessionRecord
	FactoryWorldRuntimeView                = factorycontracts.FactoryWorldRuntimeView
	FactoryWorldScriptRequest              = factorycontracts.FactoryWorldScriptRequest
	FactoryWorldScriptResponse             = factorycontracts.FactoryWorldScriptResponse
	FactoryWorldSessionBracketProjection   = factorycontracts.FactoryWorldSessionBracketProjection
	FactoryWorldSessionBracketState        = factorycontracts.FactoryWorldSessionBracketState
	FactoryWorldSessionRuntime             = factorycontracts.FactoryWorldSessionRuntime
	FactoryWorldState                      = factorycontracts.FactoryWorldState
	FactoryWorldSubmitWorkType             = factorycontracts.FactoryWorldSubmitWorkType
	FactoryWorldThrottlePause              = factorycontracts.FactoryWorldThrottlePause
	FactoryWorldTopologyView               = factorycontracts.FactoryWorldTopologyView
	FactoryWorldTrace                      = factorycontracts.FactoryWorldTrace
	FactoryWorldView                       = factorycontracts.FactoryWorldView
	FactoryWorldWorkItemRef                = factorycontracts.FactoryWorldWorkItemRef
	FactoryWorldWorkStateChangeRecord      = factorycontracts.FactoryWorldWorkStateChangeRecord
	FactoryWorldWorkstationEdge            = factorycontracts.FactoryWorldWorkstationEdge
	FactoryWorldWorkstationNode            = factorycontracts.FactoryWorldWorkstationNode
	InitialStructurePayload                = factorycontracts.InitialStructurePayload
)

const (
	FactoryStateCompleted = factorycontracts.FactoryStateCompleted
	FactoryStateFailed    = factorycontracts.FactoryStateFailed
	FactoryStateIdle      = factorycontracts.FactoryStateIdle
	FactoryStatePaused    = factorycontracts.FactoryStatePaused
	FactoryStateRunning   = factorycontracts.FactoryStateRunning
	StateTypeFailed       = factorycontracts.StateTypeFailed
	StateTypeInitial      = factorycontracts.StateTypeInitial
	StateTypeProcessing   = factorycontracts.StateTypeProcessing
	StateTypeTerminal     = factorycontracts.StateTypeTerminal
)

var (
	CloneFactoryWorldDispatchCompletion            = factorycontracts.CloneFactoryWorldDispatchCompletion
	CloneFactoryWorldInferenceAttemptsByDispatchID = factorycontracts.CloneFactoryWorldInferenceAttemptsByDispatchID
	CloneFactoryWorldProviderSessionRecord         = factorycontracts.CloneFactoryWorldProviderSessionRecord
	IsSystemTimePlace                              = factorycontracts.IsSystemTimePlace
	IsSystemTimeWorkType                           = factorycontracts.IsSystemTimeWorkType
)
