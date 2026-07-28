package recordings

import factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"

// Recordings-owned Factory Event envelope, type vocabulary, and detached event
// payloads. Peers import these aliases from pkg/services/recordings rather than
// treating the vocabulary as Factory Definitions-owned peer contract surface.
// Implementation debt remains in the contracts mega-barrel until CLN-DEF-CONTRACTS
// story 007 deletes it.
type (
	ArtifactCreatedEventPayload           = factorycontracts.ArtifactCreatedEventPayload
	DispatchInterruptedEventPayload       = factorycontracts.DispatchInterruptedEventPayload
	DispatchQueuedEventPayload            = factorycontracts.DispatchQueuedEventPayload
	DispatchReconciledEventPayload        = factorycontracts.DispatchReconciledEventPayload
	DispatchRequestEventPayload           = factorycontracts.DispatchRequestEventPayload
	FactoryChangeEventPayload             = factorycontracts.FactoryChangeEventPayload
	FactoryEvent                          = factorycontracts.FactoryEvent
	FactoryEventContext                   = factorycontracts.FactoryEventContext
	FactoryEventReconnectCursor           = factorycontracts.FactoryEventReconnectCursor
	FactoryEventReconnectScope            = factorycontracts.FactoryEventReconnectScope
	FactoryEventStream                    = factorycontracts.FactoryEventStream
	FactoryEventType                      = factorycontracts.FactoryEventType
	FactorySessionCompletedEventPayload   = factorycontracts.FactorySessionCompletedEventPayload
	FactorySessionLifecycleControlEventPayload = factorycontracts.FactorySessionLifecycleControlEventPayload
	FactorySessionLogicalResolveHint      = factorycontracts.FactorySessionLogicalResolveHint
	FactorySessionPausedEventPayload      = factorycontracts.FactorySessionPausedEventPayload
	FactorySessionResultUpdatedEventPayload = factorycontracts.FactorySessionResultUpdatedEventPayload
	FactorySessionResumedEventPayload     = factorycontracts.FactorySessionResumedEventPayload
	FactorySessionStartedEventPayload     = factorycontracts.FactorySessionStartedEventPayload
	FactorySessionSyncPreflightOptions    = factorycontracts.FactorySessionSyncPreflightOptions
	FactoryStateResponseEventPayload      = factorycontracts.FactoryStateResponseEventPayload
	InitialStructureRequestEventPayload   = factorycontracts.InitialStructureRequestEventPayload
	JavaScriptCheckpointRefEventPayload   = factorycontracts.JavaScriptCheckpointRefEventPayload
	JavaScriptPhaseChangeEventPayload     = factorycontracts.JavaScriptPhaseChangeEventPayload
	OrchestratorCheckpointWrittenEventPayload = factorycontracts.OrchestratorCheckpointWrittenEventPayload
	OrchestratorPhaseChangedEventPayload  = factorycontracts.OrchestratorPhaseChangedEventPayload
	RunEventWallClock                     = factorycontracts.RunEventWallClock
	RunRequestEventPayload                = factorycontracts.RunRequestEventPayload
	RunResponseEventPayload               = factorycontracts.RunResponseEventPayload
	WorkStateChangeEventPayload           = factorycontracts.WorkStateChangeEventPayload
)

const (
	FactoryEventSchemaVersionV1 = factorycontracts.FactoryEventSchemaVersionV1

	FactoryEventTypeAgentRunResponse              = factorycontracts.FactoryEventTypeAgentRunResponse
	FactoryEventTypeArtifactCreated               = factorycontracts.FactoryEventTypeArtifactCreated
	FactoryEventTypeDispatchInterrupted           = factorycontracts.FactoryEventTypeDispatchInterrupted
	FactoryEventTypeDispatchQueued                = factorycontracts.FactoryEventTypeDispatchQueued
	FactoryEventTypeDispatchReconciled            = factorycontracts.FactoryEventTypeDispatchReconciled
	FactoryEventTypeDispatchRequest               = factorycontracts.FactoryEventTypeDispatchRequest
	FactoryEventTypeDispatchResponse              = factorycontracts.FactoryEventTypeDispatchResponse
	FactoryEventTypeFactoryChange                 = factorycontracts.FactoryEventTypeFactoryChange
	FactoryEventTypeFactoryStateResponse          = factorycontracts.FactoryEventTypeFactoryStateResponse
	FactoryEventTypeInferenceRequest              = factorycontracts.FactoryEventTypeInferenceRequest
	FactoryEventTypeInferenceResponse             = factorycontracts.FactoryEventTypeInferenceResponse
	FactoryEventTypeInitialStructureRequest       = factorycontracts.FactoryEventTypeInitialStructureRequest
	FactoryEventTypeJavaScriptCheckpointRef       = factorycontracts.FactoryEventTypeJavaScriptCheckpointRef
	FactoryEventTypeJavaScriptPhaseChange         = factorycontracts.FactoryEventTypeJavaScriptPhaseChange
	FactoryEventTypeModelRequest                  = factorycontracts.FactoryEventTypeModelRequest
	FactoryEventTypeModelResponse                 = factorycontracts.FactoryEventTypeModelResponse
	FactoryEventTypeOrchestratorCheckpointWritten = factorycontracts.FactoryEventTypeOrchestratorCheckpointWritten
	FactoryEventTypeOrchestratorPhaseChanged      = factorycontracts.FactoryEventTypeOrchestratorPhaseChanged
	FactoryEventTypeRelationshipChangeRequest     = factorycontracts.FactoryEventTypeRelationshipChangeRequest
	FactoryEventTypeRunRequest                    = factorycontracts.FactoryEventTypeRunRequest
	FactoryEventTypeRunResponse                   = factorycontracts.FactoryEventTypeRunResponse
	FactoryEventTypeScriptRequest                 = factorycontracts.FactoryEventTypeScriptRequest
	FactoryEventTypeScriptResponse                = factorycontracts.FactoryEventTypeScriptResponse
	FactoryEventTypeSessionCompleted              = factorycontracts.FactoryEventTypeSessionCompleted
	FactoryEventTypeSessionLifecycleControl       = factorycontracts.FactoryEventTypeSessionLifecycleControl
	FactoryEventTypeSessionPaused                 = factorycontracts.FactoryEventTypeSessionPaused
	FactoryEventTypeSessionResultUpdated          = factorycontracts.FactoryEventTypeSessionResultUpdated
	FactoryEventTypeSessionResumed                = factorycontracts.FactoryEventTypeSessionResumed
	FactoryEventTypeSessionStarted                = factorycontracts.FactoryEventTypeSessionStarted
	FactoryEventTypeWorkRequest                   = factorycontracts.FactoryEventTypeWorkRequest
	FactoryEventTypeWorkStateChange               = factorycontracts.FactoryEventTypeWorkStateChange
)

var NewFactoryEvent = factorycontracts.NewFactoryEvent
