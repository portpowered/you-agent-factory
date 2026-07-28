package projectionquery

import factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

// Recordings-owned Factory world-state projection vocabulary. Peers import
// these aliases from pkg/services/recordings rather than treating the vocabulary
// as Factory Definitions-owned peer contract surface.
type (
	ActiveThrottlePause                    = factorydefinitions.ActiveThrottlePause
	FactoryPlace                           = factorydefinitions.FactoryPlace
	FactoryPlaceOccupancy                  = factorydefinitions.FactoryPlaceOccupancy
	FactoryState                           = factorydefinitions.FactoryState
	FactoryStateDefinition                 = factorydefinitions.FactoryStateDefinition
	FactoryTerminalWork                    = factorydefinitions.FactoryTerminalWork
	FactoryWorkType                        = factorydefinitions.FactoryWorkType
	FactoryWorker                          = factorydefinitions.FactoryWorker
	FactoryWorkstation                     = factorydefinitions.FactoryWorkstation
	FactoryWorkstationRef                  = factorydefinitions.FactoryWorkstationRef
	FactoryWorldActiveExecution            = factorydefinitions.FactoryWorldActiveExecution
	FactoryWorldActivity                   = factorydefinitions.FactoryWorldActivity
	FactoryWorldAgentRunResponse           = factorydefinitions.FactoryWorldAgentRunResponse
	FactoryWorldDispatch                   = factorydefinitions.FactoryWorldDispatch
	FactoryWorldDispatchCompletion         = factorydefinitions.FactoryWorldDispatchCompletion
	FactoryWorldFailureDetail              = factorydefinitions.FactoryWorldFailureDetail
	FactoryWorldInferenceAttempt           = factorydefinitions.FactoryWorldInferenceAttempt
	FactoryWorldJavaScriptChildDispatchCounts = factorydefinitions.FactoryWorldJavaScriptChildDispatchCounts
	FactoryWorldJavaScriptProjection       = factorydefinitions.FactoryWorldJavaScriptProjection
	FactoryWorldPlaceRef                   = factorydefinitions.FactoryWorldPlaceRef
	FactoryWorldProviderSessionRecord      = factorydefinitions.FactoryWorldProviderSessionRecord
	FactoryWorldRuntimeView                = factorydefinitions.FactoryWorldRuntimeView
	FactoryWorldScriptRequest              = factorydefinitions.FactoryWorldScriptRequest
	FactoryWorldScriptResponse             = factorydefinitions.FactoryWorldScriptResponse
	FactoryWorldSessionBracketProjection   = factorydefinitions.FactoryWorldSessionBracketProjection
	FactoryWorldSessionBracketState        = factorydefinitions.FactoryWorldSessionBracketState
	FactoryWorldSessionRuntime             = factorydefinitions.FactoryWorldSessionRuntime
	FactoryWorldState                      = factorydefinitions.FactoryWorldState
	FactoryWorldSubmitWorkType             = factorydefinitions.FactoryWorldSubmitWorkType
	FactoryWorldThrottlePause              = factorydefinitions.FactoryWorldThrottlePause
	FactoryWorldTopologyView               = factorydefinitions.FactoryWorldTopologyView
	FactoryWorldTrace                      = factorydefinitions.FactoryWorldTrace
	FactoryWorldView                       = factorydefinitions.FactoryWorldView
	FactoryWorldWorkItemRef                = factorydefinitions.FactoryWorldWorkItemRef
	FactoryWorldWorkStateChangeRecord      = factorydefinitions.FactoryWorldWorkStateChangeRecord
	FactoryWorldWorkstationEdge            = factorydefinitions.FactoryWorldWorkstationEdge
	FactoryWorldWorkstationNode            = factorydefinitions.FactoryWorldWorkstationNode
	InitialStructurePayload                = factorydefinitions.InitialStructurePayload
)

const (
	FactoryStateCompleted = factorydefinitions.FactoryStateCompleted
	FactoryStateFailed    = factorydefinitions.FactoryStateFailed
	FactoryStateIdle      = factorydefinitions.FactoryStateIdle
	FactoryStatePaused    = factorydefinitions.FactoryStatePaused
	FactoryStateRunning   = factorydefinitions.FactoryStateRunning
	StateTypeFailed       = factorydefinitions.StateTypeFailed
	StateTypeInitial      = factorydefinitions.StateTypeInitial
	StateTypeProcessing   = factorydefinitions.StateTypeProcessing
	StateTypeTerminal     = factorydefinitions.StateTypeTerminal
)

var (
	CloneFactoryWorldDispatchCompletion            = factorydefinitions.CloneFactoryWorldDispatchCompletion
	CloneFactoryWorldInferenceAttemptsByDispatchID = factorydefinitions.CloneFactoryWorldInferenceAttemptsByDispatchID
	CloneFactoryWorldProviderSessionRecord         = factorydefinitions.CloneFactoryWorldProviderSessionRecord
	IsSystemTimePlace                              = factorydefinitions.IsSystemTimePlace
	IsSystemTimeWorkType                           = factorydefinitions.IsSystemTimeWorkType
)
