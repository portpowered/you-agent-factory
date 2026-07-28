package canonicalledger

import factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

// Recordings-owned Factory Event envelope, type vocabulary, and detached event
// payloads. Peers import these aliases from pkg/services/recordings rather than
// treating the vocabulary as Factory Definitions-owned peer contract surface.
type (
	ArtifactCreatedEventPayload           = factorydefinitions.ArtifactCreatedEventPayload
	DispatchInterruptedEventPayload       = factorydefinitions.DispatchInterruptedEventPayload
	DispatchQueuedEventPayload            = factorydefinitions.DispatchQueuedEventPayload
	DispatchReconciledEventPayload        = factorydefinitions.DispatchReconciledEventPayload
	DispatchRequestEventPayload           = factorydefinitions.DispatchRequestEventPayload
	FactoryChangeEventPayload             = factorydefinitions.FactoryChangeEventPayload
	FactoryEvent                          = factorydefinitions.FactoryEvent
	FactoryEventContext                   = factorydefinitions.FactoryEventContext
	FactoryEventReconnectCursor           = factorydefinitions.FactoryEventReconnectCursor
	FactoryEventReconnectScope            = factorydefinitions.FactoryEventReconnectScope
	FactoryEventStream                    = factorydefinitions.FactoryEventStream
	FactoryEventType                      = factorydefinitions.FactoryEventType
	FactorySessionCompletedEventPayload   = factorydefinitions.FactorySessionCompletedEventPayload
	FactorySessionLifecycleControlEventPayload = factorydefinitions.FactorySessionLifecycleControlEventPayload
	FactorySessionLogicalResolveHint      = factorydefinitions.FactorySessionLogicalResolveHint
	FactorySessionPausedEventPayload      = factorydefinitions.FactorySessionPausedEventPayload
	FactorySessionResultUpdatedEventPayload = factorydefinitions.FactorySessionResultUpdatedEventPayload
	FactorySessionResumedEventPayload     = factorydefinitions.FactorySessionResumedEventPayload
	FactorySessionStartedEventPayload     = factorydefinitions.FactorySessionStartedEventPayload
	FactorySessionSyncPreflightOptions    = factorydefinitions.FactorySessionSyncPreflightOptions
	FactoryStateResponseEventPayload      = factorydefinitions.FactoryStateResponseEventPayload
	InitialStructureRequestEventPayload   = factorydefinitions.InitialStructureRequestEventPayload
	JavaScriptCheckpointRefEventPayload   = factorydefinitions.JavaScriptCheckpointRefEventPayload
	JavaScriptPhaseChangeEventPayload     = factorydefinitions.JavaScriptPhaseChangeEventPayload
	OrchestratorCheckpointWrittenEventPayload = factorydefinitions.OrchestratorCheckpointWrittenEventPayload
	OrchestratorPhaseChangedEventPayload  = factorydefinitions.OrchestratorPhaseChangedEventPayload
	RunEventWallClock                     = factorydefinitions.RunEventWallClock
	RunRequestEventPayload                = factorydefinitions.RunRequestEventPayload
	RunResponseEventPayload               = factorydefinitions.RunResponseEventPayload
	WorkStateChangeEventPayload           = factorydefinitions.WorkStateChangeEventPayload
)

const (
	FactoryEventSchemaVersionV1 = factorydefinitions.FactoryEventSchemaVersionV1

	FactoryEventTypeAgentRunResponse              = factorydefinitions.FactoryEventTypeAgentRunResponse
	FactoryEventTypeArtifactCreated               = factorydefinitions.FactoryEventTypeArtifactCreated
	FactoryEventTypeDispatchInterrupted           = factorydefinitions.FactoryEventTypeDispatchInterrupted
	FactoryEventTypeDispatchQueued                = factorydefinitions.FactoryEventTypeDispatchQueued
	FactoryEventTypeDispatchReconciled            = factorydefinitions.FactoryEventTypeDispatchReconciled
	FactoryEventTypeDispatchRequest               = factorydefinitions.FactoryEventTypeDispatchRequest
	FactoryEventTypeDispatchResponse              = factorydefinitions.FactoryEventTypeDispatchResponse
	FactoryEventTypeFactoryChange                 = factorydefinitions.FactoryEventTypeFactoryChange
	FactoryEventTypeFactoryStateResponse          = factorydefinitions.FactoryEventTypeFactoryStateResponse
	FactoryEventTypeInferenceRequest              = factorydefinitions.FactoryEventTypeInferenceRequest
	FactoryEventTypeInferenceResponse             = factorydefinitions.FactoryEventTypeInferenceResponse
	FactoryEventTypeInitialStructureRequest       = factorydefinitions.FactoryEventTypeInitialStructureRequest
	FactoryEventTypeJavaScriptCheckpointRef       = factorydefinitions.FactoryEventTypeJavaScriptCheckpointRef
	FactoryEventTypeJavaScriptPhaseChange         = factorydefinitions.FactoryEventTypeJavaScriptPhaseChange
	FactoryEventTypeModelRequest                  = factorydefinitions.FactoryEventTypeModelRequest
	FactoryEventTypeModelResponse                 = factorydefinitions.FactoryEventTypeModelResponse
	FactoryEventTypeOrchestratorCheckpointWritten = factorydefinitions.FactoryEventTypeOrchestratorCheckpointWritten
	FactoryEventTypeOrchestratorPhaseChanged      = factorydefinitions.FactoryEventTypeOrchestratorPhaseChanged
	FactoryEventTypeRelationshipChangeRequest     = factorydefinitions.FactoryEventTypeRelationshipChangeRequest
	FactoryEventTypeRunRequest                    = factorydefinitions.FactoryEventTypeRunRequest
	FactoryEventTypeRunResponse                   = factorydefinitions.FactoryEventTypeRunResponse
	FactoryEventTypeScriptRequest                 = factorydefinitions.FactoryEventTypeScriptRequest
	FactoryEventTypeScriptResponse                = factorydefinitions.FactoryEventTypeScriptResponse
	FactoryEventTypeSessionCompleted              = factorydefinitions.FactoryEventTypeSessionCompleted
	FactoryEventTypeSessionLifecycleControl       = factorydefinitions.FactoryEventTypeSessionLifecycleControl
	FactoryEventTypeSessionPaused                 = factorydefinitions.FactoryEventTypeSessionPaused
	FactoryEventTypeSessionResultUpdated          = factorydefinitions.FactoryEventTypeSessionResultUpdated
	FactoryEventTypeSessionResumed                = factorydefinitions.FactoryEventTypeSessionResumed
	FactoryEventTypeSessionStarted                = factorydefinitions.FactoryEventTypeSessionStarted
	FactoryEventTypeWorkRequest                   = factorydefinitions.FactoryEventTypeWorkRequest
	FactoryEventTypeWorkStateChange               = factorydefinitions.FactoryEventTypeWorkStateChange
)

var NewFactoryEvent = factorydefinitions.NewFactoryEvent
