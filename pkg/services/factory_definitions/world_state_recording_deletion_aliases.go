package factorydefinitions

import contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"

// Deletion-only aliases retain temporary Factory Definitions root symbols for
// world-state projection vocabulary rehomed to pkg/services/recordings in
// CLN-DEF-CONTRACTS story 004. Peers must import Recordings root world-state
// contracts instead; remove this file when downstream consumers finish cutover.

type (
	FactoryPlace                           = contracts.FactoryPlace
	FactoryPlaceOccupancy                  = contracts.FactoryPlaceOccupancy
	FactoryState                           = contracts.FactoryState
	FactoryStateDefinition                 = contracts.FactoryStateDefinition
	FactoryTerminalWork                    = contracts.FactoryTerminalWork
	FactoryWorkType                        = contracts.FactoryWorkType
	FactoryWorker                          = contracts.FactoryWorker
	FactoryWorkstation                     = contracts.FactoryWorkstation
	FactoryWorkstationRef                  = contracts.FactoryWorkstationRef
	FactoryWorldActiveExecution            = contracts.FactoryWorldActiveExecution
	FactoryWorldActivity                   = contracts.FactoryWorldActivity
	FactoryWorldAgentRunResponse           = contracts.FactoryWorldAgentRunResponse
	FactoryWorldDispatch                   = contracts.FactoryWorldDispatch
	FactoryWorldDispatchCompletion         = contracts.FactoryWorldDispatchCompletion
	FactoryWorldFailureDetail              = contracts.FactoryWorldFailureDetail
	FactoryWorldInferenceAttempt           = contracts.FactoryWorldInferenceAttempt
	FactoryWorldJavaScriptChildDispatchCounts = contracts.FactoryWorldJavaScriptChildDispatchCounts
	FactoryWorldJavaScriptProjection       = contracts.FactoryWorldJavaScriptProjection
	FactoryWorldPlaceRef                   = contracts.FactoryWorldPlaceRef
	FactoryWorldProviderSessionRecord      = contracts.FactoryWorldProviderSessionRecord
	FactoryWorldRuntimeView                = contracts.FactoryWorldRuntimeView
	FactoryWorldScriptRequest              = contracts.FactoryWorldScriptRequest
	FactoryWorldScriptResponse             = contracts.FactoryWorldScriptResponse
	FactoryWorldSessionBracketProjection   = contracts.FactoryWorldSessionBracketProjection
	FactoryWorldSessionBracketState        = contracts.FactoryWorldSessionBracketState
	FactoryWorldSessionRuntime             = contracts.FactoryWorldSessionRuntime
	FactoryWorldState                      = contracts.FactoryWorldState
	FactoryWorldSubmitWorkType             = contracts.FactoryWorldSubmitWorkType
	FactoryWorldThrottlePause              = contracts.FactoryWorldThrottlePause
	FactoryWorldTopologyView               = contracts.FactoryWorldTopologyView
	FactoryWorldTrace                      = contracts.FactoryWorldTrace
	FactoryWorldView                       = contracts.FactoryWorldView
	FactoryWorldWorkItemRef                = contracts.FactoryWorldWorkItemRef
	FactoryWorldWorkStateChangeRecord      = contracts.FactoryWorldWorkStateChangeRecord
	FactoryWorldWorkstationEdge            = contracts.FactoryWorldWorkstationEdge
	FactoryWorldWorkstationNode            = contracts.FactoryWorldWorkstationNode
	InitialStructurePayload                = contracts.InitialStructurePayload
)

const (
	FactoryStateCompleted = contracts.FactoryStateCompleted
	FactoryStateFailed    = contracts.FactoryStateFailed
	FactoryStateIdle      = contracts.FactoryStateIdle
	FactoryStatePaused    = contracts.FactoryStatePaused
	FactoryStateRunning   = contracts.FactoryStateRunning
	StateTypeFailed       = contracts.StateTypeFailed
	StateTypeInitial      = contracts.StateTypeInitial
	StateTypeProcessing   = contracts.StateTypeProcessing
	StateTypeTerminal     = contracts.StateTypeTerminal
)

var (
	CloneFactoryWorldDispatchCompletion            = contracts.CloneFactoryWorldDispatchCompletion
	CloneFactoryWorldInferenceAttemptsByDispatchID = contracts.CloneFactoryWorldInferenceAttemptsByDispatchID
	CloneFactoryWorldProviderSessionRecord         = contracts.CloneFactoryWorldProviderSessionRecord
	IsSystemTimePlace                              = contracts.IsSystemTimePlace
	IsSystemTimeWorkType                           = contracts.IsSystemTimeWorkType
)
