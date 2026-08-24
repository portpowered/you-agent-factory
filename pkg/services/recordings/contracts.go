// Package recordings exposes the single Recordings service seam and
// aliases the transport-neutral value vocabulary owned by its internals.
package recordings

import (
	"errors"
)

import recordingcontracts "github.com/portpowered/infinite-you/pkg/services/recordings/internal/contracts"

type (
	ActiveThrottlePause                                        = recordingcontracts.ActiveThrottlePause
	AppendRecordedEventRequest                                 = recordingcontracts.AppendRecordedEventRequest
	AppendRecordedEventResult                                  = recordingcontracts.AppendRecordedEventResult
	ArtifactCreatedEventPayload                                = recordingcontracts.ArtifactCreatedEventPayload
	BindRecordingRequest                                       = recordingcontracts.BindRecordingRequest
	BindRecordingResult                                        = recordingcontracts.BindRecordingResult
	BindReplayExecutionRequest                                 = recordingcontracts.BindReplayExecutionRequest
	BindReplayExecutionResult                                  = recordingcontracts.BindReplayExecutionResult
	BuildPortableArtifactRequest                               = recordingcontracts.BuildPortableArtifactRequest
	BuildPortableArtifactResult                                = recordingcontracts.BuildPortableArtifactResult
	CanonicalEvent                                             = recordingcontracts.CanonicalEvent
	CanonicalEventCursor                                       = recordingcontracts.CanonicalEventCursor
	CanonicalEventID                                           = recordingcontracts.CanonicalEventID
	CanonicalEventKind                                         = recordingcontracts.CanonicalEventKind
	CanonicalEventScope                                        = recordingcontracts.CanonicalEventScope
	CanonicalEventSequence                                     = recordingcontracts.CanonicalEventSequence
	CheckpointResumabilityStatus                               = recordingcontracts.CheckpointResumabilityStatus
	Clock                                                      = recordingcontracts.Clock
	CompletedDispatch                                          = recordingcontracts.CompletedDispatch
	CompletionDeliveryPlanner                                  = recordingcontracts.CompletionDeliveryPlanner
	ReplayWorkerSessionIDResolver                              = recordingcontracts.ReplayWorkerSessionIDResolver
	ReplayDispatchIDResolver                                   = recordingcontracts.ReplayDispatchIDResolver
	CreateReplayPlanRequest                                    = recordingcontracts.CreateReplayPlanRequest
	CreateReplayPlanResult                                     = recordingcontracts.CreateReplayPlanResult
	DecodePortableArtifactRequest                              = recordingcontracts.DecodePortableArtifactRequest
	DecodePortableArtifactResult                               = recordingcontracts.DecodePortableArtifactResult
	DispatchConsumedWorkRef                                    = recordingcontracts.DispatchConsumedWorkRef
	DispatchEntry                                              = recordingcontracts.DispatchEntry
	DispatchInterruptedEventPayload                            = recordingcontracts.DispatchInterruptedEventPayload
	DispatchQueuedEventPayload                                 = recordingcontracts.DispatchQueuedEventPayload
	DispatchReconciledEventPayload                             = recordingcontracts.DispatchReconciledEventPayload
	DispatchReconciliationSource                               = recordingcontracts.DispatchReconciliationSource
	DispatchRecord                                             = recordingcontracts.DispatchRecord
	DispatchRecorder                                           = recordingcontracts.DispatchRecorder
	DispatchRequestEventMetadata                               = recordingcontracts.DispatchRequestEventMetadata
	DispatchRequestEventPayload                                = recordingcontracts.DispatchRequestEventPayload
	DispatchResourceRef                                        = recordingcontracts.DispatchResourceRef
	EncodePortableArtifactRequest                              = recordingcontracts.EncodePortableArtifactRequest
	EncodePortableArtifactResult                               = recordingcontracts.EncodePortableArtifactResult
	EventReconnectCursor                                       = recordingcontracts.EventReconnectCursor
	EventReconnectScope                                        = recordingcontracts.EventReconnectScope
	EventSubscription                                          = recordingcontracts.EventSubscription
	ExportPortableArtifactRequest                              = recordingcontracts.ExportPortableArtifactRequest
	ExportPortableArtifactResult                               = recordingcontracts.ExportPortableArtifactResult
	FactoryChangeEventPayload                                  = recordingcontracts.FactoryChangeEventPayload
	FactoryChangeRequestEventPayload                           = recordingcontracts.FactoryChangeRequestEventPayload
	FactoryChangeFailedEventPayload                            = recordingcontracts.FactoryChangeFailedEventPayload
	FactoryDispatchKind                                        = recordingcontracts.FactoryDispatchKind
	FactoryDispatchRecord                                      = recordingcontracts.FactoryDispatchRecord
	FactoryDispatchStatus                                      = recordingcontracts.FactoryDispatchStatus
	FactoryDispatchUsage                                       = recordingcontracts.FactoryDispatchUsage
	FactoryDispatchWarning                                     = recordingcontracts.FactoryDispatchWarning
	FactoryEvent                                               = recordingcontracts.FactoryEvent
	FactoryEventContext                                        = recordingcontracts.FactoryEventContext
	FactoryEventReconnectCursor                                = recordingcontracts.FactoryEventReconnectCursor
	FactoryEventReconnectScope                                 = recordingcontracts.FactoryEventReconnectScope
	FactoryEventStream                                         = recordingcontracts.FactoryEventStream
	FactoryEventType                                           = recordingcontracts.FactoryEventType
	HumanApprovalDecision                                      = recordingcontracts.HumanApprovalDecision
	HumanApprovalRequestedEventPayload                         = recordingcontracts.HumanApprovalRequestedEventPayload
	HumanApprovalStatus                                        = recordingcontracts.HumanApprovalStatus
	FactoryPlace                                               = recordingcontracts.FactoryPlace
	FactoryPlaceOccupancy                                      = recordingcontracts.FactoryPlaceOccupancy
	FactorySessionChildDispatchCounts                          = recordingcontracts.FactorySessionChildDispatchCounts
	FactorySessionCompletedEventPayload                        = recordingcontracts.FactorySessionCompletedEventPayload
	FactorySessionDispatchFailureDetail                        = recordingcontracts.FactorySessionDispatchFailureDetail
	FactorySessionDispatchJavaScriptState                      = recordingcontracts.FactorySessionDispatchJavaScriptState
	FactorySessionDispatchPetriState                           = recordingcontracts.FactorySessionDispatchPetriState
	FactorySessionDispatchState                                = recordingcontracts.FactorySessionDispatchState
	FactorySessionDispatchUsage                                = recordingcontracts.FactorySessionDispatchUsage
	FactorySessionDispatchWarning                              = recordingcontracts.FactorySessionDispatchWarning
	FactorySessionLifecycleControlEventPayload                 = recordingcontracts.FactorySessionLifecycleControlEventPayload
	FactorySessionLogicalResolveHint                           = recordingcontracts.FactorySessionLogicalResolveHint
	FactorySessionPausedEventPayload                           = recordingcontracts.FactorySessionPausedEventPayload
	FactorySessionResultUpdatedEventPayload                    = recordingcontracts.FactorySessionResultUpdatedEventPayload
	FactorySessionResumedEventPayload                          = recordingcontracts.FactorySessionResumedEventPayload
	FactorySessionStartedEventPayload                          = recordingcontracts.FactorySessionStartedEventPayload
	FactorySessionSyncPreflightOptions                         = recordingcontracts.FactorySessionSyncPreflightOptions
	FactoryState                                               = recordingcontracts.FactoryState
	FactoryStateDefinition                                     = recordingcontracts.FactoryStateDefinition
	FactoryStateResponseEventPayload                           = recordingcontracts.FactoryStateResponseEventPayload
	FactoryTerminalWork                                        = recordingcontracts.FactoryTerminalWork
	FactoryWorker                                              = recordingcontracts.FactoryWorker
	FactoryWorkstation                                         = recordingcontracts.FactoryWorkstation
	FactoryWorkstationRef                                      = recordingcontracts.FactoryWorkstationRef
	FactoryWorkType                                            = recordingcontracts.FactoryWorkType
	FactoryWorldActiveExecution                                = recordingcontracts.FactoryWorldActiveExecution
	FactoryWorldActivity                                       = recordingcontracts.FactoryWorldActivity
	FactoryWorldAgentRunResponse                               = recordingcontracts.FactoryWorldAgentRunResponse
	FactoryWorldDispatch                                       = recordingcontracts.FactoryWorldDispatch
	FactoryWorldDispatchCompletion                             = recordingcontracts.FactoryWorldDispatchCompletion
	FactoryWorldHumanApproval                                  = recordingcontracts.FactoryWorldHumanApproval
	FactoryWorldFailureDetail                                  = recordingcontracts.FactoryWorldFailureDetail
	FactoryWorldInferenceAttempt                               = recordingcontracts.FactoryWorldInferenceAttempt
	FactoryWorldJavaScriptChildDispatchCounts                  = recordingcontracts.FactoryWorldJavaScriptChildDispatchCounts
	FactoryWorldJavaScriptProjection                           = recordingcontracts.FactoryWorldJavaScriptProjection
	FactoryWorldPlaceRef                                       = recordingcontracts.FactoryWorldPlaceRef
	FactoryWorldProviderSessionRecord                          = recordingcontracts.FactoryWorldProviderSessionRecord
	FactoryWorldRuntimeView                                    = recordingcontracts.FactoryWorldRuntimeView
	FactoryWorldScriptRequest                                  = recordingcontracts.FactoryWorldScriptRequest
	FactoryWorldScriptResponse                                 = recordingcontracts.FactoryWorldScriptResponse
	FactoryWorldSessionBracketProjection                       = recordingcontracts.FactoryWorldSessionBracketProjection
	FactoryWorldSessionBracketState                            = recordingcontracts.FactoryWorldSessionBracketState
	FactoryWorldSessionRuntime                                 = recordingcontracts.FactoryWorldSessionRuntime
	FactoryWorldState                                          = recordingcontracts.FactoryWorldState
	FactoryWorldSubmitWorkType                                 = recordingcontracts.FactoryWorldSubmitWorkType
	FactoryWorldThrottlePause                                  = recordingcontracts.FactoryWorldThrottlePause
	FactoryWorldTopologyView                                   = recordingcontracts.FactoryWorldTopologyView
	FactoryWorldTrace                                          = recordingcontracts.FactoryWorldTrace
	FactoryWorldView                                           = recordingcontracts.FactoryWorldView
	FactoryWorldWorkItemRef                                    = recordingcontracts.FactoryWorldWorkItemRef
	FactoryWorldWorkStateChangeRecord                          = recordingcontracts.FactoryWorldWorkStateChangeRecord
	FactoryWorldWorkstationEdge                                = recordingcontracts.FactoryWorldWorkstationEdge
	FactoryWorldWorkstationNode                                = recordingcontracts.FactoryWorldWorkstationNode
	FinishRecordingRequest                                     = recordingcontracts.FinishRecordingRequest
	FinishRecordingResult                                      = recordingcontracts.FinishRecordingResult
	FlushRecordingRequest                                      = recordingcontracts.FlushRecordingRequest
	FlushRecordingResult                                       = recordingcontracts.FlushRecordingResult
	InitialStructurePayload                                    = recordingcontracts.InitialStructurePayload
	InitialStructureRequestEventPayload                        = recordingcontracts.InitialStructureRequestEventPayload
	InitialStructureSource                                     = recordingcontracts.InitialStructureSource
	JavaScriptCheckpointRefEventPayload                        = recordingcontracts.JavaScriptCheckpointRefEventPayload
	JavaScriptPhaseChangeEventPayload                          = recordingcontracts.JavaScriptPhaseChangeEventPayload
	Ledger                                                     = recordingcontracts.Ledger
	LiveRecordingTarget                                        = recordingcontracts.LiveRecordingTarget
	LiveRecordingTargetPlanner                                 = recordingcontracts.LiveRecordingTargetPlanner
	LiveRecordingTargetPlannerFunc                             = recordingcontracts.LiveRecordingTargetPlannerFunc
	LiveRecordingTargetRequest                                 = recordingcontracts.LiveRecordingTargetRequest
	LoadReplayArtifactRequest                                  = recordingcontracts.LoadReplayArtifactRequest
	LoadReplayArtifactResult                                   = recordingcontracts.LoadReplayArtifactResult
	LoadReplayInputRequest                                     = recordingcontracts.LoadReplayInputRequest
	LoadReplayInputResult                                      = recordingcontracts.LoadReplayInputResult
	ReplayInputMetadata                                        = recordingcontracts.ReplayInputMetadata
	LoadResumeInputRequest                                     = recordingcontracts.LoadResumeInputRequest
	LoadResumeInputResult                                      = recordingcontracts.LoadResumeInputResult
	LoadReplayRecordingRequest                                 = recordingcontracts.LoadReplayRecordingRequest
	LoadReplayRecordingResult                                  = recordingcontracts.LoadReplayRecordingResult
	LoadReplayRecordingForResumeRequest                        = recordingcontracts.LoadReplayRecordingForResumeRequest
	LoadReplayRecordingForResumeResult                         = recordingcontracts.LoadReplayRecordingForResumeResult
	MetadataMismatchWarning                                    = recordingcontracts.MetadataMismatchWarning
	ObserveReplayRequest                                       = recordingcontracts.ObserveReplayRequest
	ObserveReplayResult                                        = recordingcontracts.ObserveReplayResult
	OrchestratorCheckpointWrittenEventPayload                  = recordingcontracts.OrchestratorCheckpointWrittenEventPayload
	OrchestratorPhaseChangedEventPayload                       = recordingcontracts.OrchestratorPhaseChangedEventPayload
	PortableArtifact                                           = recordingcontracts.PortableArtifact
	PortableArtifactIntegrity                                  = recordingcontracts.PortableArtifactIntegrity
	PortableArtifactSchemaVersion                              = recordingcontracts.PortableArtifactSchemaVersion
	PortableArtifactSummary                                    = recordingcontracts.PortableArtifactSummary
	PortableRecording                                          = recordingcontracts.PortableRecording
	PortableRecordingArtifactSummary                           = recordingcontracts.PortableRecordingArtifactSummary
	PortableRecordingAvailability                              = recordingcontracts.PortableRecordingAvailability
	PortableRecordingCanonicalArtifact                         = recordingcontracts.PortableRecordingCanonicalArtifact
	PortableRecordingCanonicalCheckpoint                       = recordingcontracts.PortableRecordingCanonicalCheckpoint
	PortableRecordingCanonicalFacts                            = recordingcontracts.PortableRecordingCanonicalFacts
	PortableRecordingCanonicalResult                           = recordingcontracts.PortableRecordingCanonicalResult
	PortableRecordingCheckpointSummary                         = recordingcontracts.PortableRecordingCheckpointSummary
	PortableRecordingCodec                                     = recordingcontracts.PortableRecordingCodec
	PortableRecordingCompatibilityPolicy                       = recordingcontracts.PortableRecordingCompatibilityPolicy
	PortableRecordingDecodeDiagnostics                         = recordingcontracts.PortableRecordingDecodeDiagnostics
	PortableRecordingDiagnostic                                = recordingcontracts.PortableRecordingDiagnostic
	PortableRecordingDiagnosticCode                            = recordingcontracts.PortableRecordingDiagnosticCode
	PortableRecordingEventSummary                              = recordingcontracts.PortableRecordingEventSummary
	PortableRecordingFailureSummary                            = recordingcontracts.PortableRecordingFailureSummary
	PortableRecordingRedactionMetadata                         = recordingcontracts.PortableRecordingRedactionMetadata
	PortableRecordingResult                                    = recordingcontracts.PortableRecordingResult
	ReplayInputDecodeDiagnostics                               = recordingcontracts.ReplayInputDecodeDiagnostics
	PortableRecordingSessionSummary                            = recordingcontracts.PortableRecordingSessionSummary
	PortableRecordingSourceSummary                             = recordingcontracts.PortableRecordingSourceSummary
	PortableRecordingWorkerHistory                             = recordingcontracts.PortableRecordingWorkerHistory
	PortableRecordingWorkerHistoryAvailability                 = recordingcontracts.PortableRecordingWorkerHistoryAvailability
	PortableRecordingWriter                                    = recordingcontracts.PortableRecordingWriter
	ProjectionService                                          = recordingcontracts.ProjectionService
	ReadPortableArtifactRequest                                = recordingcontracts.ReadPortableArtifactRequest
	ReadPortableArtifactResult                                 = recordingcontracts.ReadPortableArtifactResult
	ReconstructWorldStateRequest                               = recordingcontracts.ReconstructWorldStateRequest
	ReconstructWorldStateResult                                = recordingcontracts.ReconstructWorldStateResult
	RecordingArtifactReference                                 = recordingcontracts.RecordingArtifactReference
	RecordingRedactedValue                                     = recordingcontracts.RecordingRedactedValue
	RecordingRedactionRequest                                  = recordingcontracts.RecordingRedactionRequest
	RecordingRedactionResult                                   = recordingcontracts.RecordingRedactionResult
	RecordingSecret                                            = recordingcontracts.RecordingSecret
	RecordingSecretProvenance                                  = recordingcontracts.RecordingSecretProvenance
	RecordingClock                                             = recordingcontracts.RecordingClock
	RecordingCreateTemporaryFile                               = recordingcontracts.RecordingCreateTemporaryFile
	RecordingFailure                                           = recordingcontracts.RecordingFailure
	RecordingFlushTicker                                       = recordingcontracts.RecordingFlushTicker
	RecordingFlushTickerFactory                                = recordingcontracts.RecordingFlushTickerFactory
	RecordingID                                                = recordingcontracts.RecordingID
	RecordingNamedPathReserver                                 = recordingcontracts.RecordingNamedPathReserver
	RecordingPathReserver                                      = recordingcontracts.RecordingPathReserver
	RecordingLifecycleState                                    = recordingcontracts.RecordingLifecycleState
	RecordingMakeDirectories                                   = recordingcontracts.RecordingMakeDirectories
	RecordingPathJoiner                                        = recordingcontracts.RecordingPathJoiner
	RecordingReadFile                                          = recordingcontracts.RecordingReadFile
	RecordingOpenFile                                          = recordingcontracts.RecordingOpenFile
	ReplayArtifactMetadataLoader                               = recordingcontracts.ReplayArtifactMetadataLoader
	RecordingRemovePath                                        = recordingcontracts.RecordingRemovePath
	RecordingRenamePath                                        = recordingcontracts.RecordingRenamePath
	RecordingScopeRef                                          = recordingcontracts.RecordingScopeRef
	RecordingScopeService                                      = recordingcontracts.RecordingScopeService
	RecordingScopeStatus                                       = recordingcontracts.RecordingScopeStatus
	OpenRecordingScopeRequest                                  = recordingcontracts.OpenRecordingScopeRequest
	OpenRecordingScopeResult                                   = recordingcontracts.OpenRecordingScopeResult
	SubscribeRecordingScopeRequest                             = recordingcontracts.SubscribeRecordingScopeRequest
	SubscribeRecordingScopeResult                              = recordingcontracts.SubscribeRecordingScopeResult
	LoadReplayRecordingScopeRequest                            = recordingcontracts.LoadReplayRecordingScopeRequest
	LoadReplayRecordingScopeResult                             = recordingcontracts.LoadReplayRecordingScopeResult
	CreateReplayPlanScopeRequest                               = recordingcontracts.CreateReplayPlanScopeRequest
	CreateReplayPlanScopeResult                                = recordingcontracts.CreateReplayPlanScopeResult
	ObserveReplayScopeRequest                                  = recordingcontracts.ObserveReplayScopeRequest
	ObserveReplayScopeResult                                   = recordingcontracts.ObserveReplayScopeResult
	ReconstructRecordingScopeRequest                           = recordingcontracts.ReconstructRecordingScopeRequest
	ReconstructRecordingScopeResult                            = recordingcontracts.ReconstructRecordingScopeResult
	QuerySimpleDashboardScopeRequest                           = recordingcontracts.QuerySimpleDashboardScopeRequest
	QuerySimpleDashboardScopeResult                            = recordingcontracts.QuerySimpleDashboardScopeResult
	QueryWorkstationRequestsScopeRequest                       = recordingcontracts.QueryWorkstationRequestsScopeRequest
	QueryWorkstationRequestsScopeResult                        = recordingcontracts.QueryWorkstationRequestsScopeResult
	BuildPortableArtifactScopeRequest                          = recordingcontracts.BuildPortableArtifactScopeRequest
	BuildPortableArtifactScopeResult                           = recordingcontracts.BuildPortableArtifactScopeResult
	ExportPortableArtifactScopeRequest                         = recordingcontracts.ExportPortableArtifactScopeRequest
	ExportPortableArtifactScopeResult                          = recordingcontracts.ExportPortableArtifactScopeResult
	ReadPortableArtifactScopeRequest                           = recordingcontracts.ReadPortableArtifactScopeRequest
	ReadPortableArtifactScopeResult                            = recordingcontracts.ReadPortableArtifactScopeResult
	RecordingSnapshot                                          = recordingcontracts.RecordingSnapshot
	RecordingSnapshotWriter                                    = recordingcontracts.RecordingSnapshotWriter
	RecordingStatusFacts                                       = recordingcontracts.RecordingStatusFacts
	RecordingStatusRequest                                     = recordingcontracts.RecordingStatusRequest
	RecordingStatusResult                                      = recordingcontracts.RecordingStatusResult
	RecordingTargetRequest                                     = recordingcontracts.RecordingTargetRequest
	RecordingTemporaryFile                                     = recordingcontracts.RecordingTemporaryFile
	AppendRecordingScopeEventRequest                           = recordingcontracts.AppendRecordingScopeEventRequest
	AppendRecordingScopeEventResult                            = recordingcontracts.AppendRecordingScopeEventResult
	BeginRecordingScopeRequest                                 = recordingcontracts.BeginRecordingScopeRequest
	BeginRecordingScopeResult                                  = recordingcontracts.BeginRecordingScopeResult
	CloseRecordingScopeRequest                                 = recordingcontracts.CloseRecordingScopeRequest
	CloseRecordingScopeResult                                  = recordingcontracts.CloseRecordingScopeResult
	FinalizeRecordingScopeRequest                              = recordingcontracts.FinalizeRecordingScopeRequest
	FinalizeRecordingScopeResult                               = recordingcontracts.FinalizeRecordingScopeResult
	FlushRecordingScopeRequest                                 = recordingcontracts.FlushRecordingScopeRequest
	FlushRecordingScopeResult                                  = recordingcontracts.FlushRecordingScopeResult
	QueryRecordingScopeRequest                                 = recordingcontracts.QueryRecordingScopeRequest
	QueryRecordingScopeResult                                  = recordingcontracts.QueryRecordingScopeResult
	RecordRecordingErrorRequest                                = recordingcontracts.RecordRecordingErrorRequest
	RecordRecordingErrorResult                                 = recordingcontracts.RecordRecordingErrorResult
	RecordRecordingEventRequest                                = recordingcontracts.RecordRecordingEventRequest
	RecordRecordingEventResult                                 = recordingcontracts.RecordRecordingEventResult
	ReplayArtifact                                             = recordingcontracts.ReplayArtifact
	ReplayArtifactLoader                                       = recordingcontracts.ReplayArtifactLoader
	ReplayDiagnostics                                          = recordingcontracts.ReplayDiagnostics
	ReplayDivergenceFacts                                      = recordingcontracts.ReplayDivergenceFacts
	ReplayExecutionFactory                                     = recordingcontracts.ReplayExecutionFactory
	ReplayHook                                                 = recordingcontracts.ReplayHook
	ReplayHookResult                                           = recordingcontracts.ReplayHookResult
	ReplayObservation                                          = recordingcontracts.ReplayObservation
	ReplayObservationKind                                      = recordingcontracts.ReplayObservationKind
	ReplayPlanFacts                                            = recordingcontracts.ReplayPlanFacts
	ReplayPlanHandle                                           = recordingcontracts.ReplayPlanHandle
	ReplayPlanSchemaVersion                                    = recordingcontracts.ReplayPlanSchemaVersion
	ReplayRecordingFacts                                       = recordingcontracts.ReplayRecordingFacts
	ReplaySnapshot                                             = recordingcontracts.ReplaySnapshot
	ReplayTimingMode                                           = recordingcontracts.ReplayTimingMode
	ReplayWallClockMetadata                                    = recordingcontracts.ReplayWallClockMetadata
	ReplayWorkToken                                            = recordingcontracts.ReplayWorkToken
	RunEventWallClock                                          = recordingcontracts.RunEventWallClock
	RunRequestEventPayload                                     = recordingcontracts.RunRequestEventPayload
	RunResponseEventPayload                                    = recordingcontracts.RunResponseEventPayload
	RuntimeEventLedger                                         = recordingcontracts.RuntimeEventLedger
	RuntimeLedger                                              = recordingcontracts.RuntimeLedger
	SessionProjectionFacts                                     = recordingcontracts.SessionProjectionFacts
	SessionProjectionReader                                    = recordingcontracts.SessionProjectionReader
	DispatchWorkerSessionExecutionFacts                        = recordingcontracts.DispatchWorkerSessionExecutionFacts
	DispatchWorkerSessionAssociationRecorder                   = recordingcontracts.DispatchWorkerSessionAssociationRecorder
	HumanApprovalRequestRecorder                               = recordingcontracts.HumanApprovalRequestRecorder
	RuntimeOpeningRequest                                      = recordingcontracts.RuntimeOpeningRequest
	RuntimeScopeRequest                                        = recordingcontracts.RuntimeScopeRequest
	RuntimeScopeResult                                         = recordingcontracts.RuntimeScopeResult
	RuntimeRecorder                                            = recordingcontracts.RuntimeRecorder
	RuntimeRecorderWithProvenance                              = recordingcontracts.RuntimeRecorderWithProvenance
	RuntimeRecorderFactory                                     = recordingcontracts.RuntimeRecorderFactory
	SessionLifecycleControlInput                               = recordingcontracts.SessionLifecycleControlInput
	SimpleDashboardActiveExecution                             = recordingcontracts.SimpleDashboardActiveExecution
	SimpleDashboardQueryRequest                                = recordingcontracts.SimpleDashboardQueryRequest
	SimpleDashboardQueryResult                                 = recordingcontracts.SimpleDashboardQueryResult
	SimpleDashboardRenderData                                  = recordingcontracts.SimpleDashboardRenderData
	SimpleDashboardSessionData                                 = recordingcontracts.SimpleDashboardSessionData
	SimpleDashboardWorkstationActivity                         = recordingcontracts.SimpleDashboardWorkstationActivity
	StartRecordingRequest                                      = recordingcontracts.StartRecordingRequest
	StartRecordingResult                                       = recordingcontracts.StartRecordingResult
	StopRecordingRequest                                       = recordingcontracts.StopRecordingRequest
	StopRecordingResult                                        = recordingcontracts.StopRecordingResult
	SubmissionRecorder                                         = recordingcontracts.SubmissionRecorder
	SubscribeRequest                                           = recordingcontracts.SubscribeRequest
	SubscribeResult                                            = recordingcontracts.SubscribeResult
	SubscriptionGapCause                                       = recordingcontracts.SubscriptionGapCause
	SubscriptionGapFacts                                       = recordingcontracts.SubscriptionGapFacts
	SubscriptionOutcome                                        = recordingcontracts.SubscriptionOutcome
	SubscriptionOutcomeKind                                    = recordingcontracts.SubscriptionOutcomeKind
	SummarizePortableArtifactRequest                           = recordingcontracts.SummarizePortableArtifactRequest
	SummarizePortableArtifactResult                            = recordingcontracts.SummarizePortableArtifactResult
	ValidatePortableArtifactRequest                            = recordingcontracts.ValidatePortableArtifactRequest
	ValidatePortableArtifactResult                             = recordingcontracts.ValidatePortableArtifactResult
	ValidateReconnectReplayRequest                             = recordingcontracts.ValidateReconnectReplayRequest
	WorkerEventRecorder                                        = recordingcontracts.WorkerEventRecorder
	WorkStateChangeEventPayload                                = recordingcontracts.WorkStateChangeEventPayload
	WorkstationFactoryWorldMutationView                        = recordingcontracts.WorkstationFactoryWorldMutationView
	WorkstationFactoryWorldRunnerBaselineCapability            = recordingcontracts.WorkstationFactoryWorldRunnerBaselineCapability
	WorkstationFactoryWorldRunnerCapabilitiesView              = recordingcontracts.WorkstationFactoryWorldRunnerCapabilitiesView
	WorkstationFactoryWorldRunnerOptionalCapability            = recordingcontracts.WorkstationFactoryWorldRunnerOptionalCapability
	WorkstationFactoryWorldRunnerOptionalCapabilityStatus      = recordingcontracts.WorkstationFactoryWorldRunnerOptionalCapabilityStatus
	WorkstationFactoryWorldRunnerOptionalCapabilitySupportView = recordingcontracts.WorkstationFactoryWorldRunnerOptionalCapabilitySupportView
	WorkstationFactoryWorldScriptRequestView                   = recordingcontracts.WorkstationFactoryWorldScriptRequestView
	WorkstationFactoryWorldScriptResponseView                  = recordingcontracts.WorkstationFactoryWorldScriptResponseView
	WorkstationFactoryWorldSelectedRunnerView                  = recordingcontracts.WorkstationFactoryWorldSelectedRunnerView
	WorkstationFactoryWorldTokenView                           = recordingcontracts.WorkstationFactoryWorldTokenView
	WorkstationFactoryWorldWorkItemRef                         = recordingcontracts.WorkstationFactoryWorldWorkItemRef
	WorkstationFactoryWorldWorkItemRefLineageContinuity        = recordingcontracts.WorkstationFactoryWorldWorkItemRefLineageContinuity
	WorkstationFactoryWorldWorkItemRefLineageSourceKind        = recordingcontracts.WorkstationFactoryWorldWorkItemRefLineageSourceKind
	WorkstationFactoryWorldWorkItemRefPayloadStatus            = recordingcontracts.WorkstationFactoryWorldWorkItemRefPayloadStatus
	WorkstationFactoryWorldWorkstationRequestCountView         = recordingcontracts.WorkstationFactoryWorldWorkstationRequestCountView
	WorkstationFactoryWorldWorkstationRequestProjectionSlice   = recordingcontracts.WorkstationFactoryWorldWorkstationRequestProjectionSlice
	WorkstationFactoryWorldWorkstationRequestRequestView       = recordingcontracts.WorkstationFactoryWorldWorkstationRequestRequestView
	WorkstationFactoryWorldWorkstationRequestResponseView      = recordingcontracts.WorkstationFactoryWorldWorkstationRequestResponseView
	WorkstationFactoryWorldWorkstationRequestView              = recordingcontracts.WorkstationFactoryWorldWorkstationRequestView
	WorkstationRequestProjector                                = recordingcontracts.WorkstationRequestProjector
	WorkstationRequestsQueryRequest                            = recordingcontracts.WorkstationRequestsQueryRequest
	WorkstationRequestsQueryResult                             = recordingcontracts.WorkstationRequestsQueryResult
	WorkstationRunnerID                                        = recordingcontracts.WorkstationRunnerID
	WorkstationRunnerSelectionSource                           = recordingcontracts.WorkstationRunnerSelectionSource
	WorkstationStringMap                                       = recordingcontracts.WorkstationStringMap
	WorldStateReconstructor                                    = recordingcontracts.WorldStateReconstructor
	WorldStateView                                             = recordingcontracts.WorldStateView
	WorldStateViewSchemaVersion                                = recordingcontracts.WorldStateViewSchemaVersion
)

// ErrServiceUnavailable identifies a transport request that reached a
// Recordings-owned adapter before the process-scoped Recordings root was
// bound. The sentinel lives at the owner root so peer transports do not need
// to import another service's transport package to classify the failure.
var ErrServiceUnavailable = errors.New("recordings service is required")

// RuntimeOpening is the Recordings-owned capability used while Factory
// Runtime opens a private runtime scope. Replay input loading remains on the
// same process root so Factory Sessions cannot construct a second Recordings
// graph for historical replay.
type RuntimeOpening = recordingcontracts.RuntimeOpening

const (
	CheckpointResumabilityStatusResumable         = recordingcontracts.CheckpointResumabilityStatusResumable
	DefaultRecordingFlushInterval                 = recordingcontracts.DefaultRecordingFlushInterval
	DispatchReconciliationSourceProviderSession   = recordingcontracts.DispatchReconciliationSourceProviderSession
	DispatchReconciliationSourceStreamReplay      = recordingcontracts.DispatchReconciliationSourceStreamReplay
	DivergenceCategoryConfigMismatch              = recordingcontracts.DivergenceCategoryConfigMismatch
	FactoryDispatchKindJavaScriptAgent            = recordingcontracts.FactoryDispatchKindJavaScriptAgent
	FactoryDispatchKindJavaScriptScript           = recordingcontracts.FactoryDispatchKindJavaScriptScript
	FactoryDispatchKindJavaScriptSynthesize       = recordingcontracts.FactoryDispatchKindJavaScriptSynthesize
	FactoryDispatchKindJavaScriptSystem           = recordingcontracts.FactoryDispatchKindJavaScriptSystem
	FactoryDispatchKindJavaScriptTool             = recordingcontracts.FactoryDispatchKindJavaScriptTool
	FactoryDispatchKindJavaScriptVerify           = recordingcontracts.FactoryDispatchKindJavaScriptVerify
	FactoryDispatchKindPetriTransition            = recordingcontracts.FactoryDispatchKindPetriTransition
	FactoryDispatchStatusCompleted                = recordingcontracts.FactoryDispatchStatusCompleted
	FactoryDispatchStatusFailed                   = recordingcontracts.FactoryDispatchStatusFailed
	FactoryDispatchStatusInterrupted              = recordingcontracts.FactoryDispatchStatusInterrupted
	FactoryDispatchStatusQueued                   = recordingcontracts.FactoryDispatchStatusQueued
	FactoryDispatchStatusRunning                  = recordingcontracts.FactoryDispatchStatusRunning
	FactoryEventSchemaVersionV1                   = recordingcontracts.FactoryEventSchemaVersionV1
	FactoryEventTypeAgentRunResponse              = recordingcontracts.FactoryEventTypeAgentRunResponse
	FactoryEventTypeArtifactCreated               = recordingcontracts.FactoryEventTypeArtifactCreated
	FactoryEventTypeDispatchInterrupted           = recordingcontracts.FactoryEventTypeDispatchInterrupted
	FactoryEventTypeDispatchQueued                = recordingcontracts.FactoryEventTypeDispatchQueued
	FactoryEventTypeDispatchReconciled            = recordingcontracts.FactoryEventTypeDispatchReconciled
	FactoryEventTypeDispatchRequest               = recordingcontracts.FactoryEventTypeDispatchRequest
	FactoryEventTypeDispatchResponse              = recordingcontracts.FactoryEventTypeDispatchResponse
	FactoryEventTypeDispatchWorkerSessionAssoc    = recordingcontracts.FactoryEventTypeDispatchWorkerSessionAssoc
	FactoryEventTypeHumanApprovalRequested        = recordingcontracts.FactoryEventTypeHumanApprovalRequested
	HumanApprovalDecisionApprove                  = recordingcontracts.HumanApprovalDecisionApprove
	HumanApprovalDecisionReject                   = recordingcontracts.HumanApprovalDecisionReject
	HumanApprovalStatusPending                    = recordingcontracts.HumanApprovalStatusPending
	FactoryEventTypeFactoryChange                 = recordingcontracts.FactoryEventTypeFactoryChange
	FactoryEventTypeFactoryChangeRequest          = recordingcontracts.FactoryEventTypeFactoryChangeRequest
	FactoryEventTypeFactoryChangeFailed           = recordingcontracts.FactoryEventTypeFactoryChangeFailed
	FactoryEventTypeFactoryStateResponse          = recordingcontracts.FactoryEventTypeFactoryStateResponse
	FactoryEventTypeInferenceRequest              = recordingcontracts.FactoryEventTypeInferenceRequest
	FactoryEventTypeInferenceResponse             = recordingcontracts.FactoryEventTypeInferenceResponse
	FactoryEventTypeInitialStructureRequest       = recordingcontracts.FactoryEventTypeInitialStructureRequest
	FactoryEventTypeJavaScriptCheckpointRef       = recordingcontracts.FactoryEventTypeJavaScriptCheckpointRef
	FactoryEventTypeJavaScriptPhaseChange         = recordingcontracts.FactoryEventTypeJavaScriptPhaseChange
	FactoryEventTypeModelRequest                  = recordingcontracts.FactoryEventTypeModelRequest
	FactoryEventTypeModelResponse                 = recordingcontracts.FactoryEventTypeModelResponse
	FactoryEventTypeOrchestratorCheckpointWritten = recordingcontracts.FactoryEventTypeOrchestratorCheckpointWritten
	FactoryEventTypeOrchestratorPhaseChanged      = recordingcontracts.FactoryEventTypeOrchestratorPhaseChanged
	FactoryEventTypeRelationshipChangeRequest     = recordingcontracts.FactoryEventTypeRelationshipChangeRequest
	FactoryEventTypeRunRequest                    = recordingcontracts.FactoryEventTypeRunRequest
	FactoryEventTypeRunResponse                   = recordingcontracts.FactoryEventTypeRunResponse
	FactoryEventTypeScriptRequest                 = recordingcontracts.FactoryEventTypeScriptRequest
	FactoryEventTypeScriptResponse                = recordingcontracts.FactoryEventTypeScriptResponse
	FactoryEventTypeSessionCompleted              = recordingcontracts.FactoryEventTypeSessionCompleted
	FactoryEventTypeSessionLifecycleControl       = recordingcontracts.FactoryEventTypeSessionLifecycleControl
	FactoryEventTypeSessionPaused                 = recordingcontracts.FactoryEventTypeSessionPaused
	FactoryEventTypeSessionResultUpdated          = recordingcontracts.FactoryEventTypeSessionResultUpdated
	FactoryEventTypeSessionResumed                = recordingcontracts.FactoryEventTypeSessionResumed
	FactoryEventTypeSessionStarted                = recordingcontracts.FactoryEventTypeSessionStarted
	FactoryEventTypeWorkRequest                   = recordingcontracts.FactoryEventTypeWorkRequest
	FactoryEventTypeWorkStateChange               = recordingcontracts.FactoryEventTypeWorkStateChange
	FactoryStateCompleted                         = recordingcontracts.FactoryStateCompleted
	FactoryStateFailed                            = recordingcontracts.FactoryStateFailed
	FactoryStateIdle                              = recordingcontracts.FactoryStateIdle
	FactoryStatePaused                            = recordingcontracts.FactoryStatePaused
	FactoryStateRunning                           = recordingcontracts.FactoryStateRunning
	KindJavaScriptFactorySession                  = recordingcontracts.KindJavaScriptFactorySession
	PortableArtifactIntegritySHA256               = recordingcontracts.PortableArtifactIntegritySHA256
	PortableArtifactSchemaV1                      = recordingcontracts.PortableArtifactSchemaV1
	PortableRecordingCurrentSchemaVersion         = recordingcontracts.PortableRecordingCurrentSchemaVersion
	PortableRecordingCodeInvalidDigest            = recordingcontracts.PortableRecordingCodeInvalidDigest
	PortableRecordingCodeInvalidIdentity          = recordingcontracts.PortableRecordingCodeInvalidIdentity
	PortableRecordingCodeInvalidSummary           = recordingcontracts.PortableRecordingCodeInvalidSummary
	PortableRecordingCodeInvalidOrder             = recordingcontracts.PortableRecordingCodeInvalidOrder
	PortableRecordingCodeMalformedContract        = recordingcontracts.PortableRecordingCodeMalformedContract
	PortableRecordingCodeUnsupportedVersion       = recordingcontracts.PortableRecordingCodeUnsupportedVersion
	PortableRecordingReplayCompatibilityV1        = recordingcontracts.PortableRecordingReplayCompatibilityV1
	PortableRecordingSchemaV1                     = recordingcontracts.PortableRecordingSchemaV1
	PortableRecordingSchemaV2                     = recordingcontracts.PortableRecordingSchemaV2
	RecordingActive                               = recordingcontracts.RecordingActive
	RecordingFailed                               = recordingcontracts.RecordingFailed
	RecordingFinalized                            = recordingcontracts.RecordingFinalized
	RecordingSecretProvenanceDeclared             = recordingcontracts.RecordingSecretProvenanceDeclared
	ReplayCompleted                               = recordingcontracts.ReplayCompleted
	ReplayDiverged                                = recordingcontracts.ReplayDiverged
	ReplayPlanSchemaV1                            = recordingcontracts.ReplayPlanSchemaV1
	ReplayProgress                                = recordingcontracts.ReplayProgress
	ReplayTimingOrderOnly                         = recordingcontracts.ReplayTimingOrderOnly
	StateTypeFailed                               = recordingcontracts.StateTypeFailed
	StateTypeInitial                              = recordingcontracts.StateTypeInitial
	StateTypeProcessing                           = recordingcontracts.StateTypeProcessing
	StateTypeTerminal                             = recordingcontracts.StateTypeTerminal
	SubscriptionBackpressure                      = recordingcontracts.SubscriptionBackpressure
	SubscriptionClosed                            = recordingcontracts.SubscriptionClosed
	SubscriptionEvent                             = recordingcontracts.SubscriptionEvent
	SubscriptionGap                               = recordingcontracts.SubscriptionGap
	SubscriptionSequenceDiscontinuity             = recordingcontracts.SubscriptionSequenceDiscontinuity
	WorldStateViewSchemaV1                        = recordingcontracts.WorldStateViewSchemaV1
)

const (
	PortableRecordingCodeUnsupportedSchema          = recordingcontracts.PortableRecordingCodeUnsupportedSchema
	PortableRecordingCompatibilityAction            = recordingcontracts.PortableRecordingCompatibilityAction
	PortableRecordingSchemaV3                       = recordingcontracts.PortableRecordingSchemaV3
	PortableRecordingWorkerHistoryReasonNotCaptured = recordingcontracts.PortableRecordingWorkerHistoryReasonNotCaptured
)

const (
	PortableRecordingWorkerHistoryAvailable          = recordingcontracts.PortableRecordingWorkerHistoryAvailable
	PortableRecordingWorkerHistoryUnavailable        = recordingcontracts.PortableRecordingWorkerHistoryUnavailable
	PortableRecordingWorkerHistoryReasonLegacySchema = recordingcontracts.PortableRecordingWorkerHistoryReasonLegacySchema
	PortableRecordingWorkerHistoryUnavailableReason  = recordingcontracts.PortableRecordingWorkerHistoryUnavailableReason
)

var (
	CloneFactoryWorldDispatchCompletion                = recordingcontracts.CloneFactoryWorldDispatchCompletion
	CloneFactoryWorldInferenceAttemptsByDispatchID     = recordingcontracts.CloneFactoryWorldInferenceAttemptsByDispatchID
	CloneFactoryWorldProviderSessionRecord             = recordingcontracts.CloneFactoryWorldProviderSessionRecord
	ErrCorruptReplayInput                              = recordingcontracts.ErrCorruptReplayInput
	ErrForeignPortableArtifact                         = recordingcontracts.ErrForeignPortableArtifact
	ErrInvalidAppendEvent                              = recordingcontracts.ErrInvalidAppendEvent
	ErrInvalidPortableArtifact                         = recordingcontracts.ErrInvalidPortableArtifact
	ErrInvalidPortableArtifactIntegrity                = recordingcontracts.ErrInvalidPortableArtifactIntegrity
	ErrInvalidPortableArtifactOrder                    = recordingcontracts.ErrInvalidPortableArtifactOrder
	ErrInvalidProjectionInput                          = recordingcontracts.ErrInvalidProjectionInput
	ErrInvalidProjectionScope                          = recordingcontracts.ErrInvalidProjectionScope
	ErrInvalidReconnectCursor                          = recordingcontracts.ErrInvalidReconnectCursor
	ErrInvalidRecordingEvent                           = recordingcontracts.ErrInvalidRecordingEvent
	ErrInvalidRecordingFailure                         = recordingcontracts.ErrInvalidRecordingFailure
	ErrInvalidRecordingRedactionRequest                = recordingcontracts.ErrInvalidRecordingRedactionRequest
	ErrInvalidRecordingSecretPath                      = recordingcontracts.ErrInvalidRecordingSecretPath
	ErrInvalidRecordingSecretProvenance                = recordingcontracts.ErrInvalidRecordingSecretProvenance
	ErrInvalidRecordingScope                           = recordingcontracts.ErrInvalidRecordingScope
	ErrInvalidRecordingTerminalMetadata                = recordingcontracts.ErrInvalidRecordingTerminalMetadata
	ErrInvalidReplayArtifact                           = recordingcontracts.ErrInvalidReplayArtifact
	ErrInvalidSubscribeScope                           = recordingcontracts.ErrInvalidSubscribeScope
	ErrMalformedProjectionOrder                        = recordingcontracts.ErrMalformedProjectionOrder
	ErrMissingRecordingTarget                          = recordingcontracts.ErrMissingRecordingTarget
	ErrMissingReplayArtifact                           = recordingcontracts.ErrMissingReplayArtifact
	ErrPortableArtifactCancelled                       = recordingcontracts.ErrPortableArtifactCancelled
	ErrPortableArtifactExportFailed                    = recordingcontracts.ErrPortableArtifactExportFailed
	ErrPortableArtifactUnavailable                     = recordingcontracts.ErrPortableArtifactUnavailable
	ErrReconnectCursorExpired                          = recordingcontracts.ErrReconnectCursorExpired
	ErrReconnectCursorNotFound                         = recordingcontracts.ErrReconnectCursorNotFound
	ErrReconnectCursorUnavailable                      = recordingcontracts.ErrReconnectCursorUnavailable
	ErrRecordingBindingConflict                        = recordingcontracts.ErrRecordingBindingConflict
	ErrRecordingSnapshotEncoding                       = recordingcontracts.ErrRecordingSnapshotEncoding
	ErrRecordingSnapshotWrite                          = recordingcontracts.ErrRecordingSnapshotWrite
	ErrRecordingScopeClosed                            = recordingcontracts.ErrRecordingScopeClosed
	ErrRecordingScopeFinalized                         = recordingcontracts.ErrRecordingScopeFinalized
	ErrRecordingScopeForeign                           = recordingcontracts.ErrRecordingScopeForeign
	ErrRecordingScopeInvalid                           = recordingcontracts.ErrRecordingScopeInvalid
	ErrRecordingScopeStale                             = recordingcontracts.ErrRecordingScopeStale
	ErrRecordingScopeUnknown                           = recordingcontracts.ErrRecordingScopeUnknown
	ErrRecordingSecretPathNotFound                     = recordingcontracts.ErrRecordingSecretPathNotFound
	ErrDuplicateRecordingSecretPath                    = recordingcontracts.ErrDuplicateRecordingSecretPath
	ErrRecordingWriteRejected                          = recordingcontracts.ErrRecordingWriteRejected
	ErrInvalidRecordingScopeRef                        = recordingcontracts.ErrInvalidRecordingScopeRef
	ErrReplayPlanNotFound                              = recordingcontracts.ErrReplayPlanNotFound
	ErrReplayRecordingNotFinalized                     = recordingcontracts.ErrReplayRecordingNotFinalized
	ErrReplayRecordingNotFound                         = recordingcontracts.ErrReplayRecordingNotFound
	ErrUnsupportedPortableArtifactSchema               = recordingcontracts.ErrUnsupportedPortableArtifactSchema
	ErrUnsupportedProjectionView                       = recordingcontracts.ErrUnsupportedProjectionView
	ErrUnsupportedReplayBinding                        = recordingcontracts.ErrUnsupportedReplayBinding
	ErrUnsupportedReplayPlan                           = recordingcontracts.ErrUnsupportedReplayPlan
	IsSystemTimePlace                                  = recordingcontracts.IsSystemTimePlace
	IsSystemTimeWorkType                               = recordingcontracts.IsSystemTimeWorkType
	NewFactoryEvent                                    = recordingcontracts.NewFactoryEvent
	BuildFactoryWorldWorkstationRequestProjectionSlice = recordingcontracts.BuildFactoryWorldWorkstationRequestProjectionSlice
	BuildPortableRecording                             = recordingcontracts.BuildPortableRecording
	CurrentPortableRecordingCodec                      = recordingcontracts.CurrentPortableRecordingCodec
	DecodePortableRecording                            = recordingcontracts.DecodePortableRecording
	DecodePortableRecordingWithDiagnostics             = recordingcontracts.DecodePortableRecordingWithDiagnostics
	DecodePortableRecordingMetadata                    = recordingcontracts.DecodePortableRecordingMetadata
	DecodePortableRecordingWithVersions                = recordingcontracts.DecodePortableRecordingWithVersions
	FactoryMetadataWarnings                            = recordingcontracts.FactoryMetadataWarnings
	NormalizePortableRecordingWorkerHistory            = recordingcontracts.NormalizePortableRecordingWorkerHistory
	NewPortableRecordingCodec                          = recordingcontracts.NewPortableRecordingCodec
	ValidatePortableRecording                          = recordingcontracts.ValidatePortableRecording
	ValidatePortableRecordingWithVersions              = recordingcontracts.ValidatePortableRecordingWithVersions
	RedactDeclaredSecrets                              = recordingcontracts.RedactDeclaredSecrets
	RedactCanonicalEvents                              = recordingcontracts.RedactCanonicalEvents
	RedactPortableArtifact                             = recordingcontracts.RedactPortableArtifact
	RedactPortableRecording                            = recordingcontracts.RedactPortableRecording
)

// Service is the singular cross-service Recordings authority. Historical
// queries are part of this same owner contract; runtime opening remains a
// separate capability so peers do not need to advertise process lifecycle
// operations just to read canonical history.
type Service interface {
	recordingcontracts.Service

	// QueryHistoricalRecording reconstructs one finalized recording from its
	// published artifact and returns the detached canonical history and
	// projections selected by that artifact.
	QueryHistoricalRecording(HistoricalRecordingQueryRequest) (HistoricalRecordingQueryResult, error)
}

// HistoricalRecordingIdentity identifies one durable recording and its
// requested Factory Session scope.
type HistoricalRecordingIdentity struct {
	RecordingID RecordingID
	Artifact    RecordingArtifactReference
	Scope       CanonicalEventScope
}

// HistoricalRecordingQueryRequest selects one immutable recording artifact.
type HistoricalRecordingQueryRequest struct {
	Recording HistoricalRecordingIdentity
}

// HistoricalDispatchWorkerSessionAssociation records the canonical event
// that associated a dispatch with a Worker Session.
type HistoricalDispatchWorkerSessionAssociation struct {
	ID              CanonicalEventID
	WorkerSessionID string
	RequestID       string
	Cursor          CanonicalEventCursor
}

// HistoricalDispatch is the detached latest lifecycle projection for one
// dispatch. The result preserves first-seen dispatch order.
type HistoricalDispatch struct {
	ID           string
	Status       FactoryDispatchStatus
	DispatchKind FactoryDispatchKind
	TransitionID string
	// Usage is retained only for Petri DISPATCH_RESPONSE facts. JavaScript
	// dispatch usage continues to follow its existing reconciliation path.
	Usage       *FactoryDispatchUsage
	FirstCursor CanonicalEventCursor
	LastCursor  CanonicalEventCursor
	Association *HistoricalDispatchWorkerSessionAssociation
}

// HistoricalRecordingQueryResult contains detached canonical history,
// selected-tick state, recording status, and dispatch facts.
type HistoricalRecordingQueryResult struct {
	Recording        HistoricalRecordingIdentity
	Status           RecordingStatusFacts
	Events           []CanonicalEvent
	IgnoredJSONPaths []string
	WorldState       WorldStateView
	// WorkstationRequests is the selected-tick workstation read model derived
	// from the same historical world state, keeping HTTP and MCP on one owner
	// projection result.
	WorkstationRequests WorkstationFactoryWorldWorkstationRequestProjectionSlice
	Dispatches          []HistoricalDispatch
}

// HistoricalRecordingQueryErrorKind classifies durable-history outcomes
// without requiring callers to parse diagnostic strings.
type HistoricalRecordingQueryErrorKind string

const (
	HistoricalRecordingQueryErrorInvalidRequest HistoricalRecordingQueryErrorKind = "INVALID_REQUEST"
	HistoricalRecordingQueryErrorMissingHistory HistoricalRecordingQueryErrorKind = "MISSING_HISTORY"
	HistoricalRecordingQueryErrorCorruptHistory HistoricalRecordingQueryErrorKind = "CORRUPT_HISTORY"
	HistoricalRecordingQueryErrorUnavailable    HistoricalRecordingQueryErrorKind = "UNAVAILABLE"
)

// HistoricalRecordingQueryError retains only safe recording and event identity
// in its public presentation. Cause remains available for errors.Is/errors.As.
type HistoricalRecordingQueryError struct {
	Kind        HistoricalRecordingQueryErrorKind
	RecordingID RecordingID
	EventID     CanonicalEventID
	Cause       error
}

func (e *HistoricalRecordingQueryError) Error() string {
	if e == nil {
		return ""
	}
	message := "historical recording query " + string(e.Kind)
	if e.RecordingID != "" {
		message += " recording=" + string(e.RecordingID)
	}
	if e.EventID != "" {
		message += " event=" + string(e.EventID)
	}
	return message
}

func (e *HistoricalRecordingQueryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// RuntimeReadMetric is the small observation vocabulary used by bounded live
// reads. Recordings owns the canonical-history counters; the process-level
// recorder is supplied by the composition root.
type RuntimeReadMetric struct {
	Name   string
	Labels map[string]string
}

// RuntimeReadMetricsRecorder forwards bounded runtime-read observations.
// A function type keeps the optional capability narrow without publishing a
// second service-root interface.
type RuntimeReadMetricsRecorder func(RuntimeReadMetric)

// CanonicalHistoryReadStats is a detached snapshot of canonical-history work
// observed by one runtime ledger.
type CanonicalHistoryReadStats struct {
	CanonicalEventsCalls  uint64
	CanonicalEventsCopied uint64
	FullHistoryReductions uint64
}
