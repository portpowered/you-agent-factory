package factorydefinitions

import contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"

// Deletion-only aliases retain temporary Factory Definitions root symbols for
// event and recording envelope vocabulary rehomed to pkg/services/recordings in
// CLN-DEF-CONTRACTS story 003. Peers must import Recordings root event contracts
// instead; remove this file when downstream consumers finish cutover.

type (
	ArtifactCreatedEventPayload                  = contracts.ArtifactCreatedEventPayload
	DispatchInterruptedEventPayload              = contracts.DispatchInterruptedEventPayload
	DispatchQueuedEventPayload                   = contracts.DispatchQueuedEventPayload
	DispatchReconciledEventPayload               = contracts.DispatchReconciledEventPayload
	DispatchResultIgnoredEventPayload            = contracts.DispatchResultIgnoredEventPayload
	ObservedWorkState                            = contracts.ObservedWorkState
	DispatchRequestEventPayload                  = contracts.DispatchRequestEventPayload
	HumanApprovalRequestedEventPayload           = contracts.HumanApprovalRequestedEventPayload
	HumanApprovalDecision                        = contracts.HumanApprovalDecision
	HumanApprovalStatus                          = contracts.HumanApprovalStatus
	DispatchWorkerSessionAssociationEventPayload = contracts.DispatchWorkerSessionAssociationEventPayload
	FactoryChangeEventPayload                    = contracts.FactoryChangeEventPayload
	FactoryChangeRequestEventPayload             = contracts.FactoryChangeRequestEventPayload
	FactoryChangeFailedEventPayload              = contracts.FactoryChangeFailedEventPayload
	FactoryResourceCapacityChange                = contracts.FactoryResourceCapacityChange
	FactoryEvent                                 = contracts.FactoryEvent
	FactoryEventContext                          = contracts.FactoryEventContext
	FactoryEventReconnectCursor                  = contracts.FactoryEventReconnectCursor
	FactoryEventReconnectScope                   = contracts.FactoryEventReconnectScope
	FactoryEventStream                           = contracts.FactoryEventStream
	FactoryEventType                             = contracts.FactoryEventType
	FactorySessionCompletedEventPayload          = contracts.FactorySessionCompletedEventPayload
	FactorySessionLifecycleControlEventPayload   = contracts.FactorySessionLifecycleControlEventPayload
	FactorySessionLogicalResolveHint             = contracts.FactorySessionLogicalResolveHint
	FactorySessionPausedEventPayload             = contracts.FactorySessionPausedEventPayload
	FactorySessionResultUpdatedEventPayload      = contracts.FactorySessionResultUpdatedEventPayload
	FactorySessionResumedEventPayload            = contracts.FactorySessionResumedEventPayload
	FactorySessionStartedEventPayload            = contracts.FactorySessionStartedEventPayload
	FactorySessionSyncPreflightOptions           = contracts.FactorySessionSyncPreflightOptions
	FactoryStateResponseEventPayload             = contracts.FactoryStateResponseEventPayload
	InitialStructureRequestEventPayload          = contracts.InitialStructureRequestEventPayload
	JavaScriptCheckpointRefEventPayload          = contracts.JavaScriptCheckpointRefEventPayload
	JavaScriptPhaseChangeEventPayload            = contracts.JavaScriptPhaseChangeEventPayload
	OrchestratorCheckpointWrittenEventPayload    = contracts.OrchestratorCheckpointWrittenEventPayload
	OrchestratorPhaseChangedEventPayload         = contracts.OrchestratorPhaseChangedEventPayload
	RunEventWallClock                            = contracts.RunEventWallClock
	RunRequestEventPayload                       = contracts.RunRequestEventPayload
	RunResponseEventPayload                      = contracts.RunResponseEventPayload
	WorkStateChangeEventPayload                  = contracts.WorkStateChangeEventPayload
)

const (
	FactoryEventSchemaVersionV1 = contracts.FactoryEventSchemaVersionV1

	FactoryEventTypeAgentRunResponse               = contracts.FactoryEventTypeAgentRunResponse
	FactoryEventTypeArtifactCreated                = contracts.FactoryEventTypeArtifactCreated
	FactoryEventTypeDispatchInterrupted            = contracts.FactoryEventTypeDispatchInterrupted
	FactoryEventTypeDispatchQueued                 = contracts.FactoryEventTypeDispatchQueued
	FactoryEventTypeDispatchReconciled             = contracts.FactoryEventTypeDispatchReconciled
	FactoryEventTypeDispatchResultIgnored          = contracts.FactoryEventTypeDispatchResultIgnored
	FactoryEventTypeDispatchRequest                = contracts.FactoryEventTypeDispatchRequest
	FactoryEventTypeDispatchResponse               = contracts.FactoryEventTypeDispatchResponse
	FactoryEventTypeDispatchWorkerSessionAssoc     = contracts.FactoryEventTypeDispatchWorkerSessionAssoc
	FactoryEventTypeHumanApprovalRequested         = contracts.FactoryEventTypeHumanApprovalRequested
	FactoryEventTypeFactoryChange                  = contracts.FactoryEventTypeFactoryChange
	FactoryEventTypeFactoryChangeRequest           = contracts.FactoryEventTypeFactoryChangeRequest
	FactoryEventTypeFactoryChangeFailed            = contracts.FactoryEventTypeFactoryChangeFailed
	FactoryEventTypeFactoryStateResponse           = contracts.FactoryEventTypeFactoryStateResponse
	FactoryEventTypeInferenceRequest               = contracts.FactoryEventTypeInferenceRequest
	FactoryEventTypeInferenceResponse              = contracts.FactoryEventTypeInferenceResponse
	FactoryEventTypeInitialStructureRequest        = contracts.FactoryEventTypeInitialStructureRequest
	FactoryEventTypeJavaScriptCheckpointRef        = contracts.FactoryEventTypeJavaScriptCheckpointRef
	FactoryEventTypeJavaScriptPhaseChange          = contracts.FactoryEventTypeJavaScriptPhaseChange
	FactoryEventTypeModelRequest                   = contracts.FactoryEventTypeModelRequest
	FactoryEventTypeModelResponse                  = contracts.FactoryEventTypeModelResponse
	FactoryEventTypeOrchestratorCheckpointWritten  = contracts.FactoryEventTypeOrchestratorCheckpointWritten
	FactoryEventTypeOrchestratorPhaseChanged       = contracts.FactoryEventTypeOrchestratorPhaseChanged
	FactoryEventTypeRelationshipChangeRequest      = contracts.FactoryEventTypeRelationshipChangeRequest
	FactoryEventTypeRunRequest                     = contracts.FactoryEventTypeRunRequest
	FactoryEventTypeRunResponse                    = contracts.FactoryEventTypeRunResponse
	FactoryEventTypeScriptRequest                  = contracts.FactoryEventTypeScriptRequest
	FactoryEventTypeScriptResponse                 = contracts.FactoryEventTypeScriptResponse
	FactoryEventTypeSessionCompleted               = contracts.FactoryEventTypeSessionCompleted
	FactoryEventTypeSessionLifecycleControl        = contracts.FactoryEventTypeSessionLifecycleControl
	FactoryEventTypeSessionPaused                  = contracts.FactoryEventTypeSessionPaused
	FactoryEventTypeSessionResultUpdated           = contracts.FactoryEventTypeSessionResultUpdated
	FactoryEventTypeSessionResumed                 = contracts.FactoryEventTypeSessionResumed
	FactoryEventTypeSessionStarted                 = contracts.FactoryEventTypeSessionStarted
	FactoryEventTypeWorkRequest                    = contracts.FactoryEventTypeWorkRequest
	FactoryEventTypeWorkStateChange                = contracts.FactoryEventTypeWorkStateChange
	DispatchResultIgnoredReasonWorkAlreadyTerminal = contracts.DispatchResultIgnoredReasonWorkAlreadyTerminal
	HumanApprovalDecisionApprove                   = contracts.HumanApprovalDecisionApprove
	HumanApprovalDecisionReject                    = contracts.HumanApprovalDecisionReject
	HumanApprovalStatusPending                     = contracts.HumanApprovalStatusPending
)

var NewFactoryEvent = contracts.NewFactoryEvent
