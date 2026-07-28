// Package recordings defines the public Recordings service boundary.
package recordings

import (
	"context"
	"errors"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordingworkstation "github.com/portpowered/infinite-you/pkg/services/recordings/projections/workstation"
	recordingartifacts "github.com/portpowered/infinite-you/pkg/services/recordings/artifacts"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// ErrReconnectCursorNotFound reports that an acknowledged cursor does not
// identify an event in the selected ledger stream.
var ErrReconnectCursorNotFound = errors.New("reconnect cursor not found in event history")

// ErrInvalidSubscribeScope reports that a reconnect-aware subscribe request
// carries a malformed scope (for example a whitespace-only SessionID).
var ErrInvalidSubscribeScope = errors.New("invalid subscribe reconnect scope")

// ErrInvalidReconnectCursor reports a malformed cursor that cannot identify a
// canonical position.
var ErrInvalidReconnectCursor = errors.New("invalid reconnect cursor")

// ErrReconnectCursorExpired reports a cursor whose retained event is no longer
// available in the selected canonical history.
var ErrReconnectCursorExpired = errors.New("reconnect cursor expired")

// ErrReconnectCursorUnavailable reports a cursor for a different or otherwise
// unavailable stream generation.
var ErrReconnectCursorUnavailable = errors.New("reconnect cursor unavailable")

// ErrInvalidAppendEvent reports that an append request does not contain the
// identity, kind, timestamp, scope, or JSON payload required for a canonical
// Factory Event. Rejected events are never assigned ordering facts.
var ErrInvalidAppendEvent = errors.New("invalid canonical event append")

// ErrInvalidProjectionInput reports that a projection-query request carries
// empty or malformed inputs (for example a negative selected tick).
var ErrInvalidProjectionInput = errors.New("invalid projection query input")

// ErrInvalidProjectionScope reports a malformed or inconsistent Factory
// Session scope in a projection query.
var ErrInvalidProjectionScope = errors.New("invalid projection query scope")

// ErrMalformedProjectionOrder reports canonical facts that are duplicated,
// out of order, discontinuous, or inconsistent with the requested cursor.
var ErrMalformedProjectionOrder = errors.New("malformed canonical projection order")

// ErrUnsupportedProjectionView reports a detached read-model value whose
// schema is not supported by this Recordings implementation.
var ErrUnsupportedProjectionView = errors.New("unsupported projection view")

// ErrMissingRecordingTarget reports that a recording-lifecycle request lacks
// an opaque artifact reference or identifies an unknown recording.
var ErrMissingRecordingTarget = errors.New("missing recording target")

// ErrInvalidRecordingScope reports a malformed Factory Session scope on a
// recording binding.
var ErrInvalidRecordingScope = errors.New("invalid recording scope")

// ErrRecordingBindingConflict reports that a requested RecordingID is already
// bound to a different artifact reference or Factory Session scope.
var ErrRecordingBindingConflict = errors.New("recording binding conflict")

// ErrInvalidRecordingEvent reports that a canonical event cannot be associated
// with the selected recording because its scope or ordering facts are invalid.
var ErrInvalidRecordingEvent = errors.New("invalid recording event")

// ErrInvalidRecordingFailure reports a malformed detached recording failure.
var ErrInvalidRecordingFailure = errors.New("invalid recording failure")

// ErrInvalidRecordingTerminalMetadata reports terminal facts that cannot be applied.
var ErrInvalidRecordingTerminalMetadata = errors.New("invalid recording terminal metadata")

// ErrRecordingSnapshotEncoding identifies snapshot encoding failure.
var ErrRecordingSnapshotEncoding = errors.New("recording snapshot encoding failed")

// ErrRecordingSnapshotWrite identifies snapshot persistence failure.
var ErrRecordingSnapshotWrite = errors.New("recording snapshot write failed")

// ErrRecordingWriteRejected reports that a write was rejected after the
// recording finished.
var ErrRecordingWriteRejected = errors.New("recording write rejected after finish")

// DefaultRecordingFlushInterval is the cadence used when an enabled recording
// does not request a positive active-flush interval.
const DefaultRecordingFlushInterval = 250 * time.Millisecond

// ErrMissingReplayArtifact reports that a replay-load request lacks a usable
// artifact path or id.
var ErrMissingReplayArtifact = errors.New("missing replay artifact path or id")

// ErrInvalidReplayArtifact reports that a replay artifact path/id could not be
// loaded as a valid (non-corrupt) detached replay artifact.
var ErrInvalidReplayArtifact = errors.New("invalid or corrupt replay artifact")

// ErrUnsupportedReplayBinding reports that a replay-binding request carries
// unsupported inputs (for example a missing artifact or empty schema version).
var ErrUnsupportedReplayBinding = errors.New("unsupported replay binding input")

// RuntimeOpeningRequest contains Recordings-owned artifact selection for one
// runtime. Recording paths and flush policy do not leak into unrelated service
// requests.
type RuntimeOpeningRequest struct {
	RecordPath    string
	ReplayPath    string
	WorkflowID    string
	FlushInterval time.Duration
}

// CanonicalEventID is the Recordings-owned identity of one accepted Factory
// event.
type CanonicalEventID string

// CanonicalEventSequence is the Recordings-assigned global position of one
// event in canonical Factory-event order. A session-scoped selection can
// therefore contain increasing, non-contiguous values where other sessions'
// events occupy the intervening positions.
type CanonicalEventSequence int64

// CanonicalEventKind identifies the detached payload vocabulary without
// exposing a producer, reducer, or transport enum.
type CanonicalEventKind string

// CanonicalEventScope identifies the Factory Session whose canonical history
// contains an event. An empty FactorySessionID represents factory-wide scope.
type CanonicalEventScope struct {
	FactorySessionID string
}

// CanonicalEventCursor is a portable reconnect position in global canonical
// order. StreamGenerationID distinguishes histories whose numeric sequences
// may overlap; SubscribeRequest.Scope selects which event at that position may
// be acknowledged.
type CanonicalEventCursor struct {
	StreamGenerationID string
	Sequence           CanonicalEventSequence
}

// CanonicalEvent is a detached, Recordings-owned canonical fact. Its fields
// contain only value data: Payload is immutable JSON text rather than a shared
// byte slice, and no implementation, datastore, runtime, or transport handle
// can cross this boundary.
type CanonicalEvent struct {
	ID          CanonicalEventID
	Sequence    CanonicalEventSequence
	FactoryTick int
	Scope       CanonicalEventScope
	Cursor      CanonicalEventCursor
	RecordedAt  time.Time
	Kind        CanonicalEventKind
	Payload     string
	// SourceContext preserves detached producer correlation metadata.
	SourceContext string
}

// SubscriptionOutcomeKind identifies one deterministic observation from a
// subscription without prescribing its transport or buffering implementation.
type SubscriptionOutcomeKind string

const (
	SubscriptionEvent  SubscriptionOutcomeKind = "EVENT"
	SubscriptionGap    SubscriptionOutcomeKind = "GAP"
	SubscriptionClosed SubscriptionOutcomeKind = "CLOSED"
)

// SubscriptionGapCause identifies why ordered delivery cannot continue.
type SubscriptionGapCause string

const (
	SubscriptionSequenceDiscontinuity SubscriptionGapCause = "SEQUENCE_DISCONTINUITY"
	SubscriptionBackpressure          SubscriptionGapCause = "BACKPRESSURE"
)

// SubscriptionGapFacts describes an observed discontinuity and the cursor from
// which the peer can reconnect. ExpectedSequence and ObservedSequence identify
// positions in the requested delivery scope (session-local for a scoped
// subscription); equal positions identify the first event made unavailable by
// backpressure. ReconnectFrom remains the last delivered global canonical
// cursor. An unavailable event is not reported as delivered.
type SubscriptionGapFacts struct {
	Cause            SubscriptionGapCause
	ExpectedSequence CanonicalEventSequence
	ObservedSequence CanonicalEventSequence
	ReconnectFrom    CanonicalEventCursor
}

// SubscriptionOutcome is one explicit subscription observation. Event is set
// for SubscriptionEvent and Gap is set for SubscriptionGap.
type SubscriptionOutcome struct {
	Kind  SubscriptionOutcomeKind
	Event CanonicalEvent
	Gap   *SubscriptionGapFacts
}

// EventSubscription is the implementation-neutral ordered subscription
// operation. Next may block until an event, discontinuity, closure, or context
// cancellation; it does not prescribe channels, buffers, goroutines, retention,
// or transport framing.
type EventSubscription func(context.Context) SubscriptionOutcome

// Next observes the next ordered subscription outcome.
func (subscription EventSubscription) Next(ctx context.Context) SubscriptionOutcome {
	if subscription == nil {
		return SubscriptionOutcome{Kind: SubscriptionClosed}
	}
	return subscription(ctx)
}

// Factory Event envelope vocabulary is owned by canonical_ledger; peers import
// these aliases from the Recordings root package.
type (
	ArtifactCreatedEventPayload           = interfaces.ArtifactCreatedEventPayload
	DispatchInterruptedEventPayload       = interfaces.DispatchInterruptedEventPayload
	DispatchQueuedEventPayload            = interfaces.DispatchQueuedEventPayload
	DispatchReconciledEventPayload        = interfaces.DispatchReconciledEventPayload
	DispatchRequestEventPayload           = interfaces.DispatchRequestEventPayload
	FactoryChangeEventPayload             = interfaces.FactoryChangeEventPayload
	FactoryEvent                          = interfaces.FactoryEvent
	FactoryEventContext                   = interfaces.FactoryEventContext
	FactoryEventReconnectCursor           = interfaces.FactoryEventReconnectCursor
	FactoryEventReconnectScope            = interfaces.FactoryEventReconnectScope
	FactoryEventStream                    = interfaces.FactoryEventStream
	FactoryEventType                      = interfaces.FactoryEventType
	FactorySessionCompletedEventPayload   = interfaces.FactorySessionCompletedEventPayload
	FactorySessionLifecycleControlEventPayload = interfaces.FactorySessionLifecycleControlEventPayload
	FactorySessionLogicalResolveHint      = interfaces.FactorySessionLogicalResolveHint
	FactorySessionPausedEventPayload      = interfaces.FactorySessionPausedEventPayload
	FactorySessionResultUpdatedEventPayload = interfaces.FactorySessionResultUpdatedEventPayload
	FactorySessionResumedEventPayload     = interfaces.FactorySessionResumedEventPayload
	FactorySessionStartedEventPayload     = interfaces.FactorySessionStartedEventPayload
	FactorySessionSyncPreflightOptions    = interfaces.FactorySessionSyncPreflightOptions
	FactoryStateResponseEventPayload      = interfaces.FactoryStateResponseEventPayload
	InitialStructureRequestEventPayload   = interfaces.InitialStructureRequestEventPayload
	JavaScriptCheckpointRefEventPayload   = interfaces.JavaScriptCheckpointRefEventPayload
	JavaScriptPhaseChangeEventPayload     = interfaces.JavaScriptPhaseChangeEventPayload
	OrchestratorCheckpointWrittenEventPayload = interfaces.OrchestratorCheckpointWrittenEventPayload
	OrchestratorPhaseChangedEventPayload  = interfaces.OrchestratorPhaseChangedEventPayload
	RunEventWallClock                     = interfaces.RunEventWallClock
	RunRequestEventPayload                = interfaces.RunRequestEventPayload
	RunResponseEventPayload               = interfaces.RunResponseEventPayload
	WorkStateChangeEventPayload           = interfaces.WorkStateChangeEventPayload
)

const (
	FactoryEventSchemaVersionV1 = interfaces.FactoryEventSchemaVersionV1

	FactoryEventTypeAgentRunResponse              = interfaces.FactoryEventTypeAgentRunResponse
	FactoryEventTypeArtifactCreated               = interfaces.FactoryEventTypeArtifactCreated
	FactoryEventTypeDispatchInterrupted           = interfaces.FactoryEventTypeDispatchInterrupted
	FactoryEventTypeDispatchQueued                = interfaces.FactoryEventTypeDispatchQueued
	FactoryEventTypeDispatchReconciled            = interfaces.FactoryEventTypeDispatchReconciled
	FactoryEventTypeDispatchRequest               = interfaces.FactoryEventTypeDispatchRequest
	FactoryEventTypeDispatchResponse              = interfaces.FactoryEventTypeDispatchResponse
	FactoryEventTypeFactoryChange                 = interfaces.FactoryEventTypeFactoryChange
	FactoryEventTypeFactoryStateResponse          = interfaces.FactoryEventTypeFactoryStateResponse
	FactoryEventTypeInferenceRequest              = interfaces.FactoryEventTypeInferenceRequest
	FactoryEventTypeInferenceResponse             = interfaces.FactoryEventTypeInferenceResponse
	FactoryEventTypeInitialStructureRequest       = interfaces.FactoryEventTypeInitialStructureRequest
	FactoryEventTypeJavaScriptCheckpointRef       = interfaces.FactoryEventTypeJavaScriptCheckpointRef
	FactoryEventTypeJavaScriptPhaseChange         = interfaces.FactoryEventTypeJavaScriptPhaseChange
	FactoryEventTypeModelRequest                  = interfaces.FactoryEventTypeModelRequest
	FactoryEventTypeModelResponse                 = interfaces.FactoryEventTypeModelResponse
	FactoryEventTypeOrchestratorCheckpointWritten = interfaces.FactoryEventTypeOrchestratorCheckpointWritten
	FactoryEventTypeOrchestratorPhaseChanged      = interfaces.FactoryEventTypeOrchestratorPhaseChanged
	FactoryEventTypeRelationshipChangeRequest     = interfaces.FactoryEventTypeRelationshipChangeRequest
	FactoryEventTypeRunRequest                    = interfaces.FactoryEventTypeRunRequest
	FactoryEventTypeRunResponse                   = interfaces.FactoryEventTypeRunResponse
	FactoryEventTypeScriptRequest                 = interfaces.FactoryEventTypeScriptRequest
	FactoryEventTypeScriptResponse                = interfaces.FactoryEventTypeScriptResponse
	FactoryEventTypeSessionCompleted              = interfaces.FactoryEventTypeSessionCompleted
	FactoryEventTypeSessionLifecycleControl       = interfaces.FactoryEventTypeSessionLifecycleControl
	FactoryEventTypeSessionPaused                 = interfaces.FactoryEventTypeSessionPaused
	FactoryEventTypeSessionResultUpdated          = interfaces.FactoryEventTypeSessionResultUpdated
	FactoryEventTypeSessionResumed                = interfaces.FactoryEventTypeSessionResumed
	FactoryEventTypeSessionStarted                = interfaces.FactoryEventTypeSessionStarted
	FactoryEventTypeWorkRequest                   = interfaces.FactoryEventTypeWorkRequest
	FactoryEventTypeWorkStateChange               = interfaces.FactoryEventTypeWorkStateChange
)

var NewFactoryEvent = interfaces.NewFactoryEvent

// Factory world-state projection vocabulary is owned by projection_query; peers
// import these aliases from the Recordings root package.
type (
	ActiveThrottlePause                         = interfaces.ActiveThrottlePause
	FactoryPlace                                = interfaces.FactoryPlace
	FactoryPlaceOccupancy                       = interfaces.FactoryPlaceOccupancy
	FactoryState                                = interfaces.FactoryState
	FactoryStateDefinition                      = interfaces.FactoryStateDefinition
	FactoryTerminalWork                         = interfaces.FactoryTerminalWork
	FactoryWorkType                             = interfaces.FactoryWorkType
	FactoryWorker                               = interfaces.FactoryWorker
	FactoryWorkstation                          = interfaces.FactoryWorkstation
	FactoryWorkstationRef                       = interfaces.FactoryWorkstationRef
	FactoryWorldActiveExecution                 = interfaces.FactoryWorldActiveExecution
	FactoryWorldActivity                        = interfaces.FactoryWorldActivity
	FactoryWorldAgentRunResponse                = interfaces.FactoryWorldAgentRunResponse
	FactoryWorldDispatch                        = interfaces.FactoryWorldDispatch
	FactoryWorldDispatchCompletion              = interfaces.FactoryWorldDispatchCompletion
	FactoryWorldFailureDetail                   = interfaces.FactoryWorldFailureDetail
	FactoryWorldInferenceAttempt                = interfaces.FactoryWorldInferenceAttempt
	FactoryWorldJavaScriptChildDispatchCounts   = interfaces.FactoryWorldJavaScriptChildDispatchCounts
	FactoryWorldJavaScriptProjection            = interfaces.FactoryWorldJavaScriptProjection
	FactoryWorldPlaceRef                        = interfaces.FactoryWorldPlaceRef
	FactoryWorldProviderSessionRecord           = interfaces.FactoryWorldProviderSessionRecord
	FactoryWorldRuntimeView                     = interfaces.FactoryWorldRuntimeView
	FactoryWorldScriptRequest                   = interfaces.FactoryWorldScriptRequest
	FactoryWorldScriptResponse                  = interfaces.FactoryWorldScriptResponse
	FactoryWorldSessionBracketProjection        = interfaces.FactoryWorldSessionBracketProjection
	FactoryWorldSessionBracketState             = interfaces.FactoryWorldSessionBracketState
	FactoryWorldSessionRuntime                  = interfaces.FactoryWorldSessionRuntime
	FactoryWorldState                           = interfaces.FactoryWorldState
	FactoryWorldSubmitWorkType                  = interfaces.FactoryWorldSubmitWorkType
	FactoryWorldThrottlePause                   = interfaces.FactoryWorldThrottlePause
	FactoryWorldTopologyView                    = interfaces.FactoryWorldTopologyView
	FactoryWorldTrace                           = interfaces.FactoryWorldTrace
	FactoryWorldView                            = interfaces.FactoryWorldView
	FactoryWorldWorkItemRef                     = interfaces.FactoryWorldWorkItemRef
	FactoryWorldWorkStateChangeRecord           = interfaces.FactoryWorldWorkStateChangeRecord
	FactoryWorldWorkstationEdge                 = interfaces.FactoryWorldWorkstationEdge
	FactoryWorldWorkstationNode                 = interfaces.FactoryWorldWorkstationNode
	InitialStructurePayload                     = interfaces.InitialStructurePayload
)

const (
	FactoryStateCompleted = interfaces.FactoryStateCompleted
	FactoryStateFailed    = interfaces.FactoryStateFailed
	FactoryStateIdle      = interfaces.FactoryStateIdle
	FactoryStatePaused    = interfaces.FactoryStatePaused
	FactoryStateRunning   = interfaces.FactoryStateRunning
	StateTypeFailed       = interfaces.StateTypeFailed
	StateTypeInitial      = interfaces.StateTypeInitial
	StateTypeProcessing   = interfaces.StateTypeProcessing
	StateTypeTerminal     = interfaces.StateTypeTerminal
)

var (
	CloneFactoryWorldDispatchCompletion              = interfaces.CloneFactoryWorldDispatchCompletion
	CloneFactoryWorldInferenceAttemptsByDispatchID   = interfaces.CloneFactoryWorldInferenceAttemptsByDispatchID
	CloneFactoryWorldProviderSessionRecord           = interfaces.CloneFactoryWorldProviderSessionRecord
	IsSystemTimePlace                                = interfaces.IsSystemTimePlace
	IsSystemTimeWorkType                             = interfaces.IsSystemTimeWorkType
)

// Recordings-owned dispatch vocabulary is owned by projection_query; peers
// import these aliases from the Recordings root package.
type (
	CompletedDispatch                     = interfaces.CompletedDispatch
	DispatchConsumedWorkRef               = interfaces.DispatchConsumedWorkRef
	DispatchEntry                         = interfaces.DispatchEntry
	DispatchReconciliationSource          = interfaces.DispatchReconciliationSource
	DispatchRecord                        = interfaces.DispatchRecord
	DispatchRequestEventMetadata          = interfaces.DispatchRequestEventMetadata
	DispatchResourceRef                   = interfaces.DispatchResourceRef
	FactoryDispatchKind                   = interfaces.FactoryDispatchKind
	FactoryDispatchRecord                 = interfaces.FactoryDispatchRecord
	FactoryDispatchStatus                 = interfaces.FactoryDispatchStatus
	FactoryDispatchUsage                  = interfaces.FactoryDispatchUsage
	FactoryDispatchWarning                = interfaces.FactoryDispatchWarning
	FactorySessionChildDispatchCounts     = interfaces.FactorySessionChildDispatchCounts
	FactorySessionDispatchFailureDetail   = interfaces.FactorySessionDispatchFailureDetail
	FactorySessionDispatchJavaScriptState = interfaces.FactorySessionDispatchJavaScriptState
	FactorySessionDispatchPetriState      = interfaces.FactorySessionDispatchPetriState
	FactorySessionDispatchState           = interfaces.FactorySessionDispatchState
	FactorySessionDispatchUsage           = interfaces.FactorySessionDispatchUsage
	FactorySessionDispatchWarning         = interfaces.FactorySessionDispatchWarning
)

const (
	DispatchReconciliationSourceProviderSession = interfaces.DispatchReconciliationSourceProviderSession
	DispatchReconciliationSourceStreamReplay    = interfaces.DispatchReconciliationSourceStreamReplay
	FactoryDispatchKindJavaScriptAgent          = interfaces.FactoryDispatchKindJavaScriptAgent
	FactoryDispatchKindJavaScriptScript         = interfaces.FactoryDispatchKindJavaScriptScript
	FactoryDispatchKindJavaScriptSynthesize     = interfaces.FactoryDispatchKindJavaScriptSynthesize
	FactoryDispatchKindJavaScriptSystem         = interfaces.FactoryDispatchKindJavaScriptSystem
	FactoryDispatchKindJavaScriptTool           = interfaces.FactoryDispatchKindJavaScriptTool
	FactoryDispatchKindJavaScriptVerify         = interfaces.FactoryDispatchKindJavaScriptVerify
	FactoryDispatchKindPetriTransition          = interfaces.FactoryDispatchKindPetriTransition
	FactoryDispatchStatusCompleted              = interfaces.FactoryDispatchStatusCompleted
	FactoryDispatchStatusFailed                 = interfaces.FactoryDispatchStatusFailed
	FactoryDispatchStatusInterrupted            = interfaces.FactoryDispatchStatusInterrupted
	FactoryDispatchStatusQueued                 = interfaces.FactoryDispatchStatusQueued
	FactoryDispatchStatusRunning                = interfaces.FactoryDispatchStatusRunning
)

// Workstation-request projection vocabulary is owned by projection_query;
// peers import these aliases from the Recordings root package.
type (
	WorkstationFactoryWorldMutationView                         = recordingworkstation.WorkstationFactoryWorldMutationView
	WorkstationFactoryWorldRunnerBaselineCapability             = recordingworkstation.WorkstationFactoryWorldRunnerBaselineCapability
	WorkstationFactoryWorldRunnerCapabilitiesView               = recordingworkstation.WorkstationFactoryWorldRunnerCapabilitiesView
	WorkstationFactoryWorldRunnerOptionalCapability             = recordingworkstation.WorkstationFactoryWorldRunnerOptionalCapability
	WorkstationFactoryWorldRunnerOptionalCapabilityStatus       = recordingworkstation.WorkstationFactoryWorldRunnerOptionalCapabilityStatus
	WorkstationFactoryWorldRunnerOptionalCapabilitySupportView  = recordingworkstation.WorkstationFactoryWorldRunnerOptionalCapabilitySupportView
	WorkstationFactoryWorldScriptRequestView                    = recordingworkstation.WorkstationFactoryWorldScriptRequestView
	WorkstationFactoryWorldScriptResponseView                   = recordingworkstation.WorkstationFactoryWorldScriptResponseView
	WorkstationFactoryWorldSelectedRunnerView                   = recordingworkstation.WorkstationFactoryWorldSelectedRunnerView
	WorkstationFactoryWorldTokenView                            = recordingworkstation.WorkstationFactoryWorldTokenView
	WorkstationFactoryWorldWorkItemRef                          = recordingworkstation.WorkstationFactoryWorldWorkItemRef
	WorkstationFactoryWorldWorkItemRefLineageContinuity         = recordingworkstation.WorkstationFactoryWorldWorkItemRefLineageContinuity
	WorkstationFactoryWorldWorkItemRefLineageSourceKind         = recordingworkstation.WorkstationFactoryWorldWorkItemRefLineageSourceKind
	WorkstationFactoryWorldWorkItemRefPayloadStatus             = recordingworkstation.WorkstationFactoryWorldWorkItemRefPayloadStatus
	WorkstationFactoryWorldWorkstationRequestCountView          = recordingworkstation.WorkstationFactoryWorldWorkstationRequestCountView
	WorkstationFactoryWorldWorkstationRequestProjectionSlice    = recordingworkstation.WorkstationFactoryWorldWorkstationRequestProjectionSlice
	WorkstationFactoryWorldWorkstationRequestRequestView        = recordingworkstation.WorkstationFactoryWorldWorkstationRequestRequestView
	WorkstationFactoryWorldWorkstationRequestResponseView       = recordingworkstation.WorkstationFactoryWorldWorkstationRequestResponseView
	WorkstationFactoryWorldWorkstationRequestView               = recordingworkstation.WorkstationFactoryWorldWorkstationRequestView
	WorkstationRunnerID                                         = recordingworkstation.WorkstationRunnerID
	WorkstationRunnerSelectionSource                            = recordingworkstation.WorkstationRunnerSelectionSource
	WorkstationStringMap                                        = recordingworkstation.WorkstationStringMap
)

// BuildFactoryWorldWorkstationRequestProjectionSlice keeps the additive
// workstation-request contract at the API boundary while deriving it from the
// canonical selected-tick FactoryWorldState model.
func BuildFactoryWorldWorkstationRequestProjectionSlice(state FactoryWorldState) WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	return recordingworkstation.BuildFactoryWorldWorkstationRequestProjectionSlice(state)
}

var (
	// ErrReplayRecordingNotFound reports that a replay load could not select a
	// recording through its Recordings-owned identity.
	ErrReplayRecordingNotFound = errors.New("replay recording not found")
	// ErrReplayRecordingNotFinalized reports that a selected recording is not
	// yet stable enough to replay.
	ErrReplayRecordingNotFinalized = errors.New("replay recording is not finalized")
	// ErrCorruptReplayInput reports malformed scope, identity, or canonical
	// ordering in detached replay facts.
	ErrCorruptReplayInput = errors.New("corrupt replay input")
	// ErrUnsupportedReplayPlan reports an unknown plan schema, timing mode, or
	// other unsupported neutral replay option.
	ErrUnsupportedReplayPlan = errors.New("unsupported replay plan")
	// ErrReplayPlanNotFound reports an unknown opaque replay handle.
	ErrReplayPlanNotFound = errors.New("replay plan not found")
)

// ReplayPlanSchemaVersion identifies the neutral replay-plan vocabulary.
type ReplayPlanSchemaVersion string

const ReplayPlanSchemaV1 ReplayPlanSchemaVersion = "recordings.replay-plan.v1"

// ReplayTimingMode selects implementation-neutral timing behavior. Order-only
// replay preserves canonical ordering without exposing clocks or timers.
type ReplayTimingMode string

const ReplayTimingOrderOnly ReplayTimingMode = "ORDER_ONLY"

// ReplayRecordingFacts is a detached selection of one recording's canonical
// facts. Events is an independent slice and contains no decoder, store, or
// runtime handles.
type ReplayRecordingFacts struct {
	RecordingID RecordingID
	Scope       CanonicalEventScope
	Events      []CanonicalEvent
}

// LoadReplayRecordingRequest selects one recording by its opaque identity.
type LoadReplayRecordingRequest struct {
	RecordingID RecordingID
}

// LoadReplayRecordingResult returns detached canonical facts for replay.
type LoadReplayRecordingResult struct {
	Recording ReplayRecordingFacts
}

// ReplayPlanHandle is an opaque Recordings-owned replay identity.
type ReplayPlanHandle string

// CreateReplayPlanRequest asks Recordings to validate and retain a neutral
// replay plan. ExpectedThrough, when present, makes divergence observable
// without exposing a runtime engine.
type CreateReplayPlanRequest struct {
	SchemaVersion   ReplayPlanSchemaVersion
	Timing          ReplayTimingMode
	Recording       ReplayRecordingFacts
	ExpectedThrough *CanonicalEventCursor
	SelectedTick    int
}

// ReplayPlanFacts is the detached public description of an opaque plan.
type ReplayPlanFacts struct {
	Handle        ReplayPlanHandle
	RecordingID   RecordingID
	Scope         CanonicalEventScope
	TotalEvents   int
	SchemaVersion ReplayPlanSchemaVersion
	Timing        ReplayTimingMode
}

// CreateReplayPlanResult reports the accepted neutral plan.
type CreateReplayPlanResult struct {
	Plan ReplayPlanFacts
}

// ReplayObservationKind identifies one deterministic replay observation.
type ReplayObservationKind string

const (
	ReplayProgress  ReplayObservationKind = "PROGRESS"
	ReplayCompleted ReplayObservationKind = "COMPLETED"
	ReplayDiverged  ReplayObservationKind = "DIVERGED"
)

// ReplayDivergenceFacts contains safe expected and observed ordering facts.
type ReplayDivergenceFacts struct {
	Expected CanonicalEventCursor
	Observed CanonicalEventCursor
}

// ReplayObservation is one detached progress, completion, or divergence fact.
// WorldState is reduced from the canonical prefix reported by ProcessedEvents.
type ReplayObservation struct {
	Kind            ReplayObservationKind
	Plan            ReplayPlanHandle
	ProcessedEvents int
	TotalEvents     int
	Through         *CanonicalEventCursor
	WorldState      WorldStateView
	Divergence      *ReplayDivergenceFacts
}

// ObserveReplayRequest advances and observes one opaque replay plan.
type ObserveReplayRequest struct {
	Plan ReplayPlanHandle
}

// ObserveReplayResult returns one deterministic detached observation.
type ObserveReplayResult struct {
	Observation ReplayObservation
}

// Recordings-owned legacy replay artifact vocabulary. Peers import these
// aliases from pkg/services/recordings rather than treating the vocabulary as
// Factory Definitions-owned peer contract surface.
type (
	CheckpointResumabilityStatus = interfaces.CheckpointResumabilityStatus
	ReplayArtifact               = interfaces.ReplayArtifact
	ReplayDiagnostics            = interfaces.ReplayDiagnostics
	ReplayWallClockMetadata      = interfaces.ReplayWallClockMetadata
)

const (
	CheckpointResumabilityStatusResumable = interfaces.CheckpointResumabilityStatusResumable
)

// EventReconnectCursor retains the legacy projection-validation cursor shape.
// New subscriptions use CanonicalEventCursor.
type EventReconnectCursor = FactoryEventReconnectCursor

// EventReconnectScope retains the legacy projection-validation scope shape.
// New subscriptions use CanonicalEventScope.
type EventReconnectScope = FactoryEventReconnectScope

// AppendRecordedEventRequest is the plain ordered-append request peers send
// through the Recordings root append/subscribe slice.
type AppendRecordedEventRequest struct {
	Event CanonicalEvent
}

// AppendRecordedEventResult is the plain ordered-append success outcome. Event
// is a detached value and cannot be used to mutate Recordings state.
type AppendRecordedEventResult struct {
	Event CanonicalEvent
}

// SubscribeRequest is the plain reconnect-aware subscribe request peers send
// through the Recordings root append/subscribe slice.
type SubscribeRequest struct {
	Cursor *CanonicalEventCursor
	Scope  CanonicalEventScope
}

// SubscribeResult is the plain reconnect-aware subscribe success outcome.
type SubscribeResult struct {
	Subscription EventSubscription
}

// WorldStateViewSchemaVersion identifies the detached world-state payload
// schema emitted by the Recordings root query slice.
type WorldStateViewSchemaVersion string

const WorldStateViewSchemaV1 WorldStateViewSchemaVersion = "recordings.world-state.v1"

// WorldStateView is an immutable-by-contract Recordings-owned read-model value.
// Payload is JSON text so callers cannot retain a map, slice, projection store,
// runtime engine, or datastore reference owned by Recordings.
type WorldStateView struct {
	SchemaVersion WorldStateViewSchemaVersion
	Scope         CanonicalEventScope
	Through       CanonicalEventCursor
	SelectedTick  int
	Payload       string
}

// ReconstructWorldStateRequest is the canonical-fact reconstruction request
// peers send through the Recordings root projection-query slice. After, when
// present, identifies the cursor immediately preceding Events.
type ReconstructWorldStateRequest struct {
	Scope        CanonicalEventScope
	After        *CanonicalEventCursor
	Events       []CanonicalEvent
	SelectedTick int
}

// ReconstructWorldStateResult is the plain detached world-state success outcome.
type ReconstructWorldStateResult struct {
	WorldState WorldStateView
}

// SimpleDashboardQueryRequest is the plain simple-dashboard projection request
// peers send through the Recordings root projection-query slice.
type SimpleDashboardQueryRequest struct {
	WorldState WorldStateView
}

// SimpleDashboardQueryResult is the plain simple-dashboard projection outcome.
type SimpleDashboardQueryResult struct {
	Data SimpleDashboardRenderData
}

// WorkstationRequestsQueryRequest is the plain workstation-request projection
// request peers send through the Recordings root projection-query slice.
type WorkstationRequestsQueryRequest struct {
	WorldState WorldStateView
}

// WorkstationRequestsQueryResult is the plain workstation-request projection
// outcome.
type WorkstationRequestsQueryResult struct {
	Projection WorkstationFactoryWorldWorkstationRequestProjectionSlice
}

// ValidateReconnectReplayRequest is the plain reconnect-replay validation
// request peers send through the Recordings root projection-query slice.
// Events is the complete retained history for Scope, including the event
// identified by Cursor. The validated replay continuation is the ordered
// suffix strictly after that acknowledged event.
type ValidateReconnectReplayRequest struct {
	Events []CanonicalEvent
	Cursor CanonicalEventCursor
	Scope  CanonicalEventScope
}

// RecordingID is the Recordings-owned identity of one bound recording.
type RecordingID string

// RecordingArtifactReference is an opaque portable artifact reference. It does
// not grant filesystem authority or expose a writer, transaction, or temporary
// storage location.
type RecordingArtifactReference string

// RecordingLifecycleState identifies the observable state of one recording.
type RecordingLifecycleState string

const (
	RecordingActive    RecordingLifecycleState = "ACTIVE"
	RecordingFinalized RecordingLifecycleState = "FINALIZED"
	RecordingFailed    RecordingLifecycleState = "FAILED"
)

// RecordingFailure is a detached failure fact accumulated by a recording.
// Code and Message are values rather than a retained implementation error.
type RecordingFailure struct {
	Code       string
	Message    string
	RecordedAt time.Time
}

// RecordingStatusFacts is a detached snapshot of recording lifecycle state.
// Implementations must return independent cursor, time, and Failures values.
type RecordingStatusFacts struct {
	RecordingID    RecordingID
	Artifact       RecordingArtifactReference
	Scope          CanonicalEventScope
	State          RecordingLifecycleState
	AcceptedEvents int
	LastEvent      *CanonicalEventCursor
	FlushedThrough *CanonicalEventCursor
	Failures       []RecordingFailure
	FinalizedAt    *time.Time
}

// BindRecordingRequest identifies the Factory Session and opaque artifact
// reference for one recording. Repeating an explicit RecordingID with the same
// Artifact and Scope is idempotent and returns its existing status unchanged.
// Reusing it with different binding facts returns ErrRecordingBindingConflict.
// Artifact does not prescribe a file path, datastore key, writer, or
// persistence implementation.
type BindRecordingRequest struct {
	RecordingID RecordingID
	Artifact    RecordingArtifactReference
	Scope       CanonicalEventScope
}

// BindRecordingResult reports a newly active or idempotently existing detached
// recording status.
type BindRecordingResult struct {
	Status RecordingStatusFacts
}

// RecordingTargetRequest selects either an explicit opaque target or the
// Recordings-owned generated live-recording layout. Artifact takes precedence;
// HomeDir is required when Recordings must generate the target.
type RecordingTargetRequest struct {
	Artifact          RecordingArtifactReference
	HomeDir           string
	ReportedSessionID string
}

// StartRecordingRequest selects and binds one recording lifecycle. Disabled
// requests are intentionally inert: they do not select a target or allocate an
// identity.
type StartRecordingRequest struct {
	Enabled       bool
	RecordingID   RecordingID
	Scope         CanonicalEventScope
	Target        RecordingTargetRequest
	FlushInterval time.Duration
}

// StartRecordingResult reports whether recording was enabled and, when it was,
// the detached active binding. Target is the session-safe reported reference;
// no service path, writer, or storage handle crosses the root boundary.
type StartRecordingResult struct {
	Enabled bool
	Status  RecordingStatusFacts
}

// RecordRecordingEventRequest associates one canonical Recordings event with a
// bound recording.
type RecordRecordingEventRequest struct {
	RecordingID RecordingID
	Event       CanonicalEvent
}

// RecordRecordingEventResult reports the detached status after acceptance.
type RecordRecordingEventResult struct {
	Status RecordingStatusFacts
}

// RecordRecordingErrorRequest appends one detached failure fact. Cause is kept
// only inside the lifecycle owner so the terminal error can preserve standard
// error matching; it never crosses the detached status boundary.
type RecordRecordingErrorRequest struct {
	RecordingID RecordingID
	Failure     RecordingFailure
	Cause       error
}

// RecordRecordingErrorResult reports failed state with accumulated failures.
type RecordRecordingErrorResult struct {
	Status RecordingStatusFacts
}

// FlushRecordingRequest is the plain flush request for one bound recording.
type FlushRecordingRequest struct {
	RecordingID RecordingID
}

// FlushRecordingResult reports which accepted canonical position is durable
// without exposing the persistence mechanism.
type FlushRecordingResult struct {
	Status RecordingStatusFacts
}

// StopRecordingRequest identifies active periodic lifecycle work to cancel and
// join. It does not finalize the recording or perform a final flush.
type StopRecordingRequest struct {
	RecordingID RecordingID
}

// StopRecordingResult reports status after periodic lifecycle work has stopped.
type StopRecordingResult struct {
	Status RecordingStatusFacts
}

// FinishRecordingRequest is the plain finish request for one bound recording.
type FinishRecordingRequest struct {
	RecordingID RecordingID
	FinishedAt  time.Time
}

// FinishRecordingResult reports finalized state, or failed terminal state when
// the recording accumulated failures. FinishRecording returns all underlying
// causes in occurrence order while this result remains detached value data.
type FinishRecordingResult struct {
	Status RecordingStatusFacts
}

// RecordingStatusRequest is the plain status query for one bound recording.
type RecordingStatusRequest struct {
	RecordingID RecordingID
}

// RecordingStatusResult is the plain status outcome for one bound recording.
type RecordingStatusResult struct {
	Status RecordingStatusFacts
}

// RecordingSnapshot is the detached value passed to the exact persistence
// effect selected by Wire. Target selection remains private to Recordings.
type RecordingSnapshot struct {
	Status RecordingStatusFacts
	Events []CanonicalEvent
}

// RecordingSnapshotWriter persists one consistent lifecycle snapshot at the
// Recordings-private service target.
type RecordingSnapshotWriter func(string, RecordingSnapshot) error

// RecordingFlushTicker is the exact scheduling handle owned by one active
// recording. Its fields are effects rather than another service interface.
type RecordingFlushTicker struct {
	Ticks <-chan time.Time
	Stop  func()
}

// RecordingFlushTickerFactory constructs an injected cadence source.
type RecordingFlushTickerFactory func(time.Duration) RecordingFlushTicker

// LoadReplayArtifactRequest is retained for the pre-neutral replay adapter.
//
// Deprecated: peers should use LoadReplayRecordingRequest through Service.
type LoadReplayArtifactRequest struct {
	Path       string
	ArtifactID string
}

// LoadReplayArtifactResult is retained for the pre-neutral replay adapter.
//
// Deprecated: peers should use LoadReplayRecordingResult through Service.
type LoadReplayArtifactResult struct {
	Artifact *ReplayArtifact
}

// BindReplayExecutionRequest is retained for internal runtime compatibility.
//
// Deprecated: it is not part of the peer-facing Service replay slice.
type BindReplayExecutionRequest struct {
	Artifact *ReplayArtifact
}

// BindReplayExecutionResult is retained for internal runtime compatibility.
//
// Deprecated: provider and runner bindings are intentionally excluded from the
// peer-facing Service replay slice.
type BindReplayExecutionResult struct {
	Provider           workerexecution.Provider
	CommandRunner      workerexecution.CommandRunner
	Hooks              []ReplayHook
	CompletionDelivery CompletionDeliveryPlanner
}

// PortableArtifactSchemaVersion identifies the detached portable artifact
// document contract.
type PortableArtifactSchemaVersion string

const (
	// PortableArtifactSchemaV1 is the first Recordings-owned portable artifact
	// schema.
	PortableArtifactSchemaV1 PortableArtifactSchemaVersion = "recordings.portable-artifact.v1"

	// PortableArtifactIntegritySHA256 identifies the integrity algorithm used
	// by PortableArtifactSchemaV1.
	PortableArtifactIntegritySHA256 = "sha256"
)

var (
	// ErrPortableArtifactUnavailable reports a recording that is missing or
	// has not reached a terminal state from which an artifact can be built.
	ErrPortableArtifactUnavailable = errors.New("portable recording artifact unavailable")

	// ErrUnsupportedPortableArtifactSchema reports an artifact schema that the
	// receiver cannot validate or decode.
	ErrUnsupportedPortableArtifactSchema = errors.New("unsupported portable recording artifact schema")

	// ErrInvalidPortableArtifactIntegrity reports a missing, malformed, or
	// mismatched portable artifact digest.
	ErrInvalidPortableArtifactIntegrity = errors.New("invalid portable recording artifact integrity")

	// ErrInvalidPortableArtifactOrder reports canonical events whose scope,
	// cursor, or sequence facts are inconsistent.
	ErrInvalidPortableArtifactOrder = errors.New("invalid portable recording artifact event order")

	// ErrInvalidPortableArtifact reports malformed detached summary or decode
	// input that is not a schema, integrity, or ordering failure.
	ErrInvalidPortableArtifact = errors.New("invalid portable recording artifact")

	// ErrPortableArtifactExportFailed reports persistence failure while
	// publishing a completed portable artifact to its public destination.
	ErrPortableArtifactExportFailed = errors.New("portable recording artifact export failed")

	// ErrForeignPortableArtifact reports a public artifact handle that is not
	// owned by the selected recording export scope.
	ErrForeignPortableArtifact = errors.New("foreign portable recording artifact handle")

	// ErrPortableArtifactCancelled reports cancellation of an in-flight portable
	// artifact close, export, or read before publication or decode completes.
	ErrPortableArtifactCancelled = errors.New("portable recording artifact operation cancelled")
)

// PortableArtifactIntegrity contains the completed artifact digest. Digest is
// computed over the artifact with this Digest field empty.
type PortableArtifactIntegrity struct {
	Algorithm string `json:"algorithm"`
	Digest    string `json:"digest"`
}

// PortableArtifactSummary contains the detached facts a receiver needs to
// inspect an artifact without Recordings storage access.
type PortableArtifactSummary struct {
	RecordingID RecordingID                `json:"recordingId"`
	Reference   RecordingArtifactReference `json:"reference,omitempty"`
	Scope       CanonicalEventScope        `json:"scope"`
	State       RecordingLifecycleState    `json:"state"`
	EventCount  int                        `json:"eventCount"`
	FirstCursor *CanonicalEventCursor      `json:"firstCursor,omitempty"`
	LastCursor  *CanonicalEventCursor      `json:"lastCursor,omitempty"`
	Failures    []RecordingFailure         `json:"failures,omitempty"`
	Available   bool                       `json:"available"`
}

// PortableArtifact is a detached, self-validating Recordings document. Events
// remain canonical Recordings facts and preserve their assigned order.
type PortableArtifact struct {
	SchemaVersion PortableArtifactSchemaVersion `json:"schemaVersion"`
	Summary       PortableArtifactSummary       `json:"summary"`
	Events        []CanonicalEvent              `json:"events"`
	Integrity     PortableArtifactIntegrity     `json:"integrity"`
}

// BuildPortableArtifactRequest selects a closed recording by opaque identity.
type BuildPortableArtifactRequest struct {
	RecordingID RecordingID
}

// BuildPortableArtifactResult returns a detached completed artifact.
type BuildPortableArtifactResult struct {
	Artifact PortableArtifact
}

// ValidatePortableArtifactRequest validates one detached artifact.
type ValidatePortableArtifactRequest struct {
	Artifact PortableArtifact
}

// ValidatePortableArtifactResult returns its detached summary on success.
type ValidatePortableArtifactResult struct {
	Summary PortableArtifactSummary
}

// EncodePortableArtifactRequest requests completed portable bytes.
type EncodePortableArtifactRequest struct {
	Artifact PortableArtifact
}

// EncodePortableArtifactResult contains completed bytes, never an open writer,
// file, transaction, or temporary path.
type EncodePortableArtifactResult struct {
	Payload []byte
}

// DecodePortableArtifactRequest imports completed portable bytes.
type DecodePortableArtifactRequest struct {
	Payload []byte
}

// DecodePortableArtifactResult contains the validated detached artifact.
type DecodePortableArtifactResult struct {
	Artifact PortableArtifact
}

// SummarizePortableArtifactRequest inspects one detached artifact.
type SummarizePortableArtifactRequest struct {
	Artifact PortableArtifact
}

// SummarizePortableArtifactResult contains a detached summary copy.
type SummarizePortableArtifactResult struct {
	Summary PortableArtifactSummary
}

// ExportPortableArtifactRequest closes one finalized recording and publishes
// its completed portable artifact to the recording's public reference.
type ExportPortableArtifactRequest struct {
	RecordingID RecordingID
}

// ExportPortableArtifactResult reports the published public reference and the
// detached artifact that was exported.
type ExportPortableArtifactResult struct {
	Reference RecordingArtifactReference
	Artifact  PortableArtifact
}

// ReadPortableArtifactRequest reads one published portable artifact for the
// selected recording export scope.
type ReadPortableArtifactRequest struct {
	RecordingID RecordingID
	Reference   RecordingArtifactReference
}

// ReadPortableArtifactResult contains the validated detached artifact read from
// the public destination.
type ReadPortableArtifactResult struct {
	Artifact PortableArtifact
}

// Ledger is the legacy runtime append/subscribe composition capability.
// It remains available to existing runtime callers while producer cutovers are
// deferred, but it is not part of the peer-facing Service contract.
type Ledger interface {
	CanonicalEvents() []FactoryEvent
	Subscribe(
		context.Context,
		*FactoryEventReconnectCursor,
		FactoryEventReconnectScope,
	) (FactoryEventStream, error)
	StreamGenerationID() string
	AddEventRecorder(func(FactoryEvent))
	AddEventTypeRecorder(func(FactoryEventType))
	AppendRecordedEvent(FactoryEvent)
}

// Service is the singular Recordings root contract for cross-service peers.
// Published slices (append/subscribe, projection query, recording lifecycle,
// replay, and artifact export) use Recordings-owned request, result, value, and
// typed-error contracts. Legacy Ledger and ProjectionService capabilities are
// deliberately excluded so a peer can implement this authority without
// importing Factory Definitions or Recordings implementation packages.
type Service interface {
	// Append publishes one ordered Factory Event through the plain
	// append/subscribe root-contract slice.
	Append(AppendRecordedEventRequest) (AppendRecordedEventResult, error)
	// SubscribeFrom opens a reconnect-aware event stream through the plain
	// append/subscribe root-contract slice.
	SubscribeFrom(context.Context, SubscribeRequest) (SubscribeResult, error)

	// ReconstructWorldState reconstructs a detached world-state/read-model
	// through the plain projection-query root-contract slice.
	ReconstructWorldState(ReconstructWorldStateRequest) (ReconstructWorldStateResult, error)
	// QuerySimpleDashboard returns simple dashboard render data through the
	// plain projection-query root-contract slice.
	QuerySimpleDashboard(SimpleDashboardQueryRequest) (SimpleDashboardQueryResult, error)
	// QueryWorkstationRequests returns workstation-request projection values
	// through the plain projection-query root-contract slice.
	QueryWorkstationRequests(WorkstationRequestsQueryRequest) (WorkstationRequestsQueryResult, error)
	// ValidateReconnectReplayFrom validates reconnect-replay inputs through
	// the plain projection-query root-contract slice.
	ValidateReconnectReplayFrom(ValidateReconnectReplayRequest) error

	// BindRecording constructs or idempotently returns one session-scoped
	// recording through the plain recording-lifecycle root-contract slice.
	BindRecording(BindRecordingRequest) (BindRecordingResult, error)
	// StartRecording selects a target and binds an enabled recording, or
	// returns an inert disabled result without performing target work.
	StartRecording(StartRecordingRequest) (StartRecordingResult, error)
	// RecordRecordingEvent associates one canonical Factory Event with a bound
	// recording.
	RecordRecordingEvent(RecordRecordingEventRequest) (RecordRecordingEventResult, error)
	// RecordRecordingError retains a detached producer-boundary failure fact.
	RecordRecordingError(RecordRecordingErrorRequest) (RecordRecordingErrorResult, error)
	// FlushRecording flushes one bound recording through the published slice.
	FlushRecording(FlushRecordingRequest) (FlushRecordingResult, error)
	// StopRecording cancels and joins active periodic flush work without
	// finalizing or performing the caller-owned final flush.
	StopRecording(StopRecordingRequest) (StopRecordingResult, error)
	// FinishRecording finalizes one recording with terminal metadata.
	FinishRecording(FinishRecordingRequest) (FinishRecordingResult, error)
	// QueryRecordingStatus returns detached lifecycle and accumulated failure
	// facts for one bound recording.
	QueryRecordingStatus(RecordingStatusRequest) (RecordingStatusResult, error)

	// LoadReplayRecording selects finalized canonical facts through the neutral
	// replay root-contract slice.
	LoadReplayRecording(LoadReplayRecordingRequest) (LoadReplayRecordingResult, error)
	// CreateReplayPlan validates canonical facts and creates an opaque neutral
	// replay plan without exposing execution or storage machinery.
	CreateReplayPlan(CreateReplayPlanRequest) (CreateReplayPlanResult, error)
	// ObserveReplay returns deterministic progress, completion, or divergence
	// facts for an opaque replay plan.
	ObserveReplay(ObserveReplayRequest) (ObserveReplayResult, error)

	// BuildPortableArtifact builds one portable recording through the plain
	// artifact-export root-contract slice.
	BuildPortableArtifact(BuildPortableArtifactRequest) (BuildPortableArtifactResult, error)
	// ValidatePortableArtifact validates one portable recording through the
	// plain artifact-export root-contract slice.
	ValidatePortableArtifact(ValidatePortableArtifactRequest) (ValidatePortableArtifactResult, error)
	// EncodePortableArtifact validates and returns completed portable bytes.
	EncodePortableArtifact(EncodePortableArtifactRequest) (EncodePortableArtifactResult, error)
	// DecodePortableArtifact decodes and validates one portable recording
	// payload through the plain artifact-export root-contract slice.
	DecodePortableArtifact(DecodePortableArtifactRequest) (DecodePortableArtifactResult, error)
	// SummarizePortableArtifact returns detached summary/availability outcomes
	// for one portable recording through the plain artifact-export slice.
	SummarizePortableArtifact(SummarizePortableArtifactRequest) (SummarizePortableArtifactResult, error)
	// ExportPortableArtifact closes one finalized recording and atomically
	// publishes its completed portable artifact to the public reference.
	ExportPortableArtifact(context.Context, ExportPortableArtifactRequest) (ExportPortableArtifactResult, error)
	// ReadPortableArtifact reads and validates one published portable artifact
	// from its public reference.
	ReadPortableArtifact(context.Context, ReadPortableArtifactRequest) (ReadPortableArtifactResult, error)
}

// ProjectionService is the legacy runtime projection composition capability.
// It remains available to existing runtime callers while consumer cutovers are
// deferred, but it is not part of the peer-facing Service contract.
type ProjectionService interface {
	ReconstructFactoryWorldState(
		[]FactoryEvent,
		int,
	) (FactoryWorldState, error)
	SimpleDashboardRenderData(FactoryWorldState) SimpleDashboardRenderData
	ProjectActiveThrottlePauses(
		InitialStructurePayload,
		[]ActiveThrottlePause,
	) []FactoryWorldThrottlePause
	ProjectWorkstationRequests(
		FactoryWorldState,
	) WorkstationFactoryWorldWorkstationRequestProjectionSlice
	ValidateReconnectReplay(
		[]FactoryEvent,
		FactoryEventReconnectCursor,
		FactoryEventReconnectScope,
	) error
}

// WorkstationRequestProjector exposes the canonical workstation-request read
// model without coupling consumers to the complete Recordings projection
// service.
type WorkstationRequestProjector interface {
	ProjectWorkstationRequests(
		FactoryWorldState,
	) WorkstationFactoryWorldWorkstationRequestProjectionSlice
}

// WorldStateReconstructor is the narrow canonical reduction operation used by
// adapters that map external event shapes before invoking Recordings.
type WorldStateReconstructor func(
	[]FactoryEvent,
	int,
) (FactoryWorldState, error)

// ReplayArtifactLoader loads one canonical Factory-event replay artifact for
// existing Runtime opening callers. It is not the peer-facing replay seam;
// peers use LoadReplayRecording on Service.
type ReplayArtifactLoader func(string) (*ReplayArtifact, error)

// InitialStructureSource is the only topology capability Recordings consumes.
// Factory Runtime implements it without exposing its concrete graph.
type InitialStructureSource interface {
	RecordingInitialStructure(
		...interfaces.RuntimeDefinitionLookup,
	) InitialStructurePayload
}

// Clock is the minimal replay time source exposed by Recordings.
type Clock interface {
	Now() time.Time
}

// SubmissionRecorder observes one canonical Factory submission at the
// recordings boundary.
type SubmissionRecorder func(work.FactorySubmissionRecord)

// DispatchRecorder observes one canonical Factory dispatch at the recordings
// boundary.
type DispatchRecorder func(FactoryDispatchRecord)

// ReplaySnapshot contains only the runtime facts a replay hook needs. Runtime
// implementations adapt their concrete engine snapshot at the service edge.
type ReplaySnapshot struct {
	Tick          int
	TokenByWorkID map[string]ReplayWorkToken
}

// ReplayWorkToken identifies the live non-resource token for recorded Work.
type ReplayWorkToken struct {
	TokenID string
	PlaceID string
}

// ReplayHookResult is the engine-neutral output produced at one logical tick.
type ReplayHookResult struct {
	GeneratedBatches []work.GeneratedSubmissionBatch
	MarkingMutations []interfaces.MarkingMutation
	KeepAlive        bool
}

// ReplayHook is a Recordings-owned source of actions due at a logical tick.
type ReplayHook interface {
	Name() string
	Priority() int
	OnTick(context.Context, ReplaySnapshot) (ReplayHookResult, error)
}

// CompletionDeliveryPlanner describes recorded completion timing without
// exposing a Factory Runtime implementation type.
type CompletionDeliveryPlanner interface {
	DeliveryTickForDispatch(work.WorkDispatch) (int, bool, error)
}

// WorkerEventRecorder is the provider-neutral recording capability exposed to
// the Workers service during runtime construction.
type WorkerEventRecorder interface {
	RecordInferenceEvent(workerexecution.InferenceEvent)
	RecordModelEvent(workerexecution.ModelEvent)
	RecordScriptEvent(workerexecution.ScriptEvent)
	RecordAgentRunEvent(workerexecution.AgentRunResponseEvent)
}

// RuntimeRecorder adapts Factory Runtime calls to the Recordings-owned
// lifecycle and durable flush behavior of one recording.
type RuntimeRecorder interface {
	BindRecordingService(Service, CanonicalEventScope) error
	Start(context.Context)
	Stop()
	RecordEvent(FactoryEvent)
	RecordError(error)
	Finish(time.Time)
	Flush() error
	Err() error
	// Finalize stops periodic work, applies terminal metadata, and attempts one
	// final synchronous flush. Repeated calls return the first terminal result.
	Finalize(time.Time) error
}

// RuntimeRecorderFactory constructs one session-scoped replay recorder from
// root Factory Definition contracts.
type RuntimeRecorderFactory func(
	time.Duration,
	interfaces.LoadedFactorySource,
	func() time.Time,
	string,
) (RuntimeRecorder, error)

// ReplayExecutionFactory constructs the replay-specific provider, command
// runner, hooks, and completion policy consumed by existing Factory Runtime
// callers. It is not the peer-facing replay seam; peers use neutral plans and
// observations on Service.
type ReplayExecutionFactory func(
	*ReplayArtifact,
) (
	workerexecution.Provider,
	workerexecution.CommandRunner,
	[]ReplayHook,
	CompletionDeliveryPlanner,
	error,
)

// SessionLifecycleControlInput carries replay-safe pause/resume facts.
type SessionLifecycleControlInput struct {
	SessionID           string
	OrchestratorKind    string
	OrchestratorDialect string
	Source              string
	Tick                int
	Operation           interfaces.FactorySessionLifecycleControlKind
	Outcome             interfaces.FactorySessionLifecycleControlOutcome
	PreviousStatus      interfaces.FactorySessionLifecycleStatus
	NewStatus           interfaces.FactorySessionLifecycleStatus
	Reason              string
}

// RuntimeLedger is the canonical-event capability consumed by Factory Runtime.
type RuntimeLedger interface {
	Ledger

	RecordRunRequest()
	RecordInitialStructure()
	RecordWorkRequest(int, work.WorkRequestRecord, time.Time)
	RecordWorkInput(int, work.SubmitRequest, workerexecution.Token, time.Time)
	RecordWorkstationRequest(int, FactoryDispatchRecord, time.Time)
	RecordWorkstationResponse(int, workerexecution.WorkResult, CompletedDispatch)
	RecordRunResponse(int, FactoryState, string, time.Time)
	RecordWorkStateChange(int, work.WorkStateChangeRecord, time.Time)
	RecordFactoryStateChange(int, FactoryState, FactoryState, string, time.Time)
	RecordFactoryChange(int, interfaces.FactoryChangeEventPayload, time.Time)
	RecordSessionLifecycleFromFactoryConfig(string, *interfaces.FactoryConfig, int, time.Time)
	RecordSessionLifecycleCompletion(string, *interfaces.FactoryConfig, int, FactoryState, string, time.Time)
	RecordSessionPaused(SessionLifecycleControlInput, time.Time)
	RecordSessionResumed(SessionLifecycleControlInput, time.Time)
	RecordSessionLifecycleControl(SessionLifecycleControlInput, time.Time)
	SetFactoryRunnerOverride(string)
	SetInitialStructureFactory(*interfaces.FactorySnapshot)
}

// RuntimeEventLedger combines runtime-domain and Worker event publication for
// a single canonical session stream.
type RuntimeEventLedger interface {
	RuntimeLedger
	WorkerEventRecorder
}

// SimpleDashboardRenderData is the Recordings-owned event projection consumed
// by dashboard lifecycle edges.
type SimpleDashboardRenderData struct {
	InFlightDispatchCount            int
	ActiveExecutionsByDispatchID     map[string]SimpleDashboardActiveExecution
	ActiveThrottlePauses             []FactoryWorldThrottlePause
	PlaceTokenCounts                 map[string]int
	CurrentWorkItemsByPlaceID        map[string][]FactoryWorldWorkItemRef
	PlaceOccupancyWorkItemsByPlaceID map[string][]FactoryWorldWorkItemRef
	WorkstationActivityByNodeID      map[string]SimpleDashboardWorkstationActivity
	PlaceCategoriesByID              map[string]string
	Session                          SimpleDashboardSessionData
}

type SimpleDashboardActiveExecution struct {
	DispatchID      string
	TransitionID    string
	WorkstationName string
	StartedAt       time.Time
	WorkTypeIDs     []string
	WorkItems       []FactoryWorldWorkItemRef
}

type SimpleDashboardWorkstationActivity struct {
	NodeID            string
	WorkstationName   string
	ActiveDispatchIDs []string
	ActiveWorkItems   []FactoryWorldWorkItemRef
	TraceIDs          []string
}

type SimpleDashboardSessionData struct {
	HasData              bool
	DispatchedCount      int
	CompletedCount       int
	FailedCount          int
	DispatchedByWorkType map[string]int
	CompletedByWorkType  map[string]int
	FailedByWorkType     map[string]int
	DispatchHistory      []FactoryWorldDispatchCompletion
	ProviderSessions     []FactoryWorldProviderSessionRecord
}

// Portable recording aliases remain only for compatibility with existing
// Factory Session callers. They are not part of the Recordings Service portable
// artifact seam, whose detached values are defined above.
type PortableRecording = recordingartifacts.Recording
type PortableRecordingArtifactSummary = recordingartifacts.ArtifactSummary
type PortableRecordingAvailability = recordingartifacts.AvailabilityDetail
type PortableRecordingCanonicalArtifact = recordingartifacts.CanonicalArtifact
type PortableRecordingCanonicalCheckpoint = recordingartifacts.CanonicalCheckpoint
type PortableRecordingCanonicalFacts = recordingartifacts.CanonicalFacts
type PortableRecordingCanonicalResult = recordingartifacts.CanonicalResult
type PortableRecordingCheckpointSummary = recordingartifacts.CheckpointSummary
type PortableRecordingDiagnostic = recordingartifacts.Diagnostic
type PortableRecordingEventSummary = recordingartifacts.EventSummary
type PortableRecordingFailureSummary = recordingartifacts.FailureSummary
type PortableRecordingResult = recordingartifacts.ResultProjection
type PortableRecordingWriter = recordingartifacts.Writer
type RecordingTemporaryFile = recordingartifacts.TemporaryFile
type RecordingMakeDirectories = recordingartifacts.MakeDirectories
type RecordingCreateTemporaryFile = recordingartifacts.CreateTemporaryFile
type RecordingRemovePath = recordingartifacts.RemovePath
type RecordingRenamePath = recordingartifacts.RenamePath

const (
	KindJavaScriptFactorySession        = recordingartifacts.KindJavaScriptFactorySession
	DivergenceCategoryConfigMismatch    = "config_mismatch"
	PortableRecordingCodeInvalidDigest  = recordingartifacts.CodeInvalidDigest
	PortableRecordingCodeInvalidSummary = recordingartifacts.CodeInvalidSummary
)

// Legacy package-level helpers remain for existing callers. Peers should prefer
// the plain artifact-export methods on Service as the cross-service source of
// truth for build/validate/decode outcomes.
var (
	BuildPortableRecording    = recordingartifacts.Build
	DecodePortableRecording   = recordingartifacts.DecodeAndValidate
	ValidatePortableRecording = recordingartifacts.Validate
)
