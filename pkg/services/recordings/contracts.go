// Package recordings defines the public Recordings service boundary.
package recordings

import (
	"context"
	"errors"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordingartifacts "github.com/portpowered/infinite-you/pkg/services/recordings/artifacts"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	providercontract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

// ErrReconnectCursorNotFound reports that an acknowledged cursor does not
// identify an event in the selected ledger stream.
var ErrReconnectCursorNotFound = errors.New("reconnect cursor not found in event history")

// ErrInvalidSubscribeScope reports that a reconnect-aware subscribe request
// carries a malformed scope (for example a whitespace-only SessionID).
var ErrInvalidSubscribeScope = errors.New("invalid subscribe reconnect scope")

// ErrInvalidProjectionInput reports that a projection-query request carries
// empty or malformed inputs (for example a negative selected tick).
var ErrInvalidProjectionInput = errors.New("invalid projection query input")

// ErrMissingRecordingTarget reports that a recording-lifecycle request lacks a
// bound recording target (empty record path or unknown recording id).
var ErrMissingRecordingTarget = errors.New("missing recording target")

// ErrRecordingFlushFailed reports that a recording flush could not complete.
var ErrRecordingFlushFailed = errors.New("recording flush failed")

// ErrRecordingWriteRejected reports that a write was rejected after the
// recording finished.
var ErrRecordingWriteRejected = errors.New("recording write rejected after finish")

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

// EventReconnectCursor is the plain reconnect cursor value published at the
// Recordings root for the append/subscribe slice.
type EventReconnectCursor = interfaces.FactoryEventReconnectCursor

// EventReconnectScope is the plain reconnect scope value published at the
// Recordings root for the append/subscribe slice.
type EventReconnectScope = interfaces.FactoryEventReconnectScope

// EventStream is the plain ordered event stream/result value published at the
// Recordings root for the append/subscribe slice.
type EventStream = interfaces.FactoryEventStream

// AppendRecordedEventRequest is the plain ordered-append request peers send
// through the Recordings root append/subscribe slice.
type AppendRecordedEventRequest struct {
	Event interfaces.FactoryEvent
}

// AppendRecordedEventResult is the plain ordered-append success outcome.
type AppendRecordedEventResult struct{}

// SubscribeRequest is the plain reconnect-aware subscribe request peers send
// through the Recordings root append/subscribe slice.
type SubscribeRequest struct {
	Cursor *EventReconnectCursor
	Scope  EventReconnectScope
}

// SubscribeResult is the plain reconnect-aware subscribe success outcome.
type SubscribeResult struct {
	Stream EventStream
}

// ReconstructWorldStateRequest is the plain world-state reconstruction request
// peers send through the Recordings root projection-query slice.
type ReconstructWorldStateRequest struct {
	Events       []interfaces.FactoryEvent
	SelectedTick int
}

// ReconstructWorldStateResult is the plain detached world-state success outcome.
type ReconstructWorldStateResult struct {
	WorldState interfaces.FactoryWorldState
}

// SimpleDashboardQueryRequest is the plain simple-dashboard projection request
// peers send through the Recordings root projection-query slice.
type SimpleDashboardQueryRequest struct {
	WorldState interfaces.FactoryWorldState
}

// SimpleDashboardQueryResult is the plain simple-dashboard projection outcome.
type SimpleDashboardQueryResult struct {
	Data SimpleDashboardRenderData
}

// WorkstationRequestsQueryRequest is the plain workstation-request projection
// request peers send through the Recordings root projection-query slice.
type WorkstationRequestsQueryRequest struct {
	WorldState interfaces.FactoryWorldState
}

// WorkstationRequestsQueryResult is the plain workstation-request projection
// outcome.
type WorkstationRequestsQueryResult struct {
	Projection WorkstationFactoryWorldWorkstationRequestProjectionSlice
}

// ValidateReconnectReplayRequest is the plain reconnect-replay validation
// request peers send through the Recordings root projection-query slice.
type ValidateReconnectReplayRequest struct {
	Events []interfaces.FactoryEvent
	Cursor EventReconnectCursor
	Scope  EventReconnectScope
}

// BindRecordingRequest is the plain recorder construction/binding request peers
// send through the Recordings root recording-lifecycle slice.
type BindRecordingRequest struct {
	RecordingID   string
	RecordPath    string
	FlushInterval time.Duration
}

// BindRecordingResult is the plain recorder construction/binding success
// outcome.
type BindRecordingResult struct {
	RecordingID string
}

// StartRecordingRequest is the plain start request for one bound recording.
type StartRecordingRequest struct {
	RecordingID string
}

// StartRecordingResult is the plain start success outcome.
type StartRecordingResult struct{}

// RecordRecordingEventRequest is the plain record-event request for one bound
// recording.
type RecordRecordingEventRequest struct {
	RecordingID string
	Event       interfaces.FactoryEvent
}

// RecordRecordingEventResult is the plain record-event success outcome.
type RecordRecordingEventResult struct{}

// RecordRecordingErrorRequest is the plain record-error request for one bound
// recording.
type RecordRecordingErrorRequest struct {
	RecordingID string
	Err         error
}

// RecordRecordingErrorResult is the plain record-error success outcome.
type RecordRecordingErrorResult struct{}

// FlushRecordingRequest is the plain flush request for one bound recording.
type FlushRecordingRequest struct {
	RecordingID string
}

// FlushRecordingResult is the plain flush success outcome.
type FlushRecordingResult struct{}

// FinishRecordingRequest is the plain finish request for one bound recording.
type FinishRecordingRequest struct {
	RecordingID string
	FinishedAt  time.Time
}

// FinishRecordingResult is the plain finish success outcome.
type FinishRecordingResult struct{}

// StopRecordingRequest is the plain stop request for one bound recording.
type StopRecordingRequest struct {
	RecordingID string
}

// StopRecordingResult is the plain stop success outcome.
type StopRecordingResult struct{}

// RecordingStatusRequest is the plain status query for one bound recording.
type RecordingStatusRequest struct {
	RecordingID string
}

// RecordingStatusResult is the plain status outcome for one bound recording.
type RecordingStatusResult struct {
	Started  bool
	Finished bool
	Stopped  bool
	Err      error
}

// LoadReplayArtifactRequest is the plain replay-load request peers send through
// the Recordings root replay slice. Cross-service callers supply a path and/or
// artifact id only; nested implementation types and filesystem effect
// interfaces are not part of this request shape.
type LoadReplayArtifactRequest struct {
	Path       string
	ArtifactID string
}

// LoadReplayArtifactResult is the plain detached replay-artifact success
// outcome.
type LoadReplayArtifactResult struct {
	Artifact *interfaces.ReplayArtifact
}

// BindReplayExecutionRequest is the plain replay-binding request peers send
// through the Recordings root replay slice. Callers supply a detached replay
// artifact value; nested Recordings implementation types and public filesystem
// effect interfaces are not part of this request shape.
type BindReplayExecutionRequest struct {
	Artifact *interfaces.ReplayArtifact
}

// BindReplayExecutionResult is the plain replay execution-binding success
// outcome. Provider, command-runner, hooks, and completion planner facts are
// represented as Recordings-owned contract values or approved peer root
// contracts already published at this boundary.
type BindReplayExecutionResult struct {
	Provider           providercontract.Provider
	CommandRunner      workerexecution.CommandRunner
	Hooks              []ReplayHook
	CompletionDelivery CompletionDeliveryPlanner
}

// Ledger is the append/subscribe capability surface embedded in the singular
// Recordings root Service. Peers should depend on Service rather than treating
// Ledger as a second peer-facing Recordings authority. Nested ledger storage
// and event-history implementation packages remain out of the peer import path.
type Ledger interface {
	CanonicalEvents() []interfaces.FactoryEvent
	Subscribe(
		context.Context,
		*interfaces.FactoryEventReconnectCursor,
		interfaces.FactoryEventReconnectScope,
	) (interfaces.FactoryEventStream, error)
	StreamGenerationID() string
	AddEventRecorder(func(interfaces.FactoryEvent))
	AddEventTypeRecorder(func(interfaces.FactoryEventType))
	AppendRecordedEvent(interfaces.FactoryEvent)
}

// Service is the singular Recordings root contract for cross-service peers.
// Published slices (append/subscribe, projection query, recording lifecycle,
// replay, and artifact export) are additive methods or embedded capability
// surfaces on this one named interface and use plain Recordings-owned request,
// result, value, and typed-error contracts. Existing append/subscribe and
// projection query capabilities remain reachable through this singular root.
// Peers must depend on Service rather than introducing a second peer-facing
// Recordings authority. Nested IMP-REC-* implementation moves, event-backbone
// leases beyond additive root publication, CLI-manifest/provider-conductor
// ownership changes, and OpenAPI package-motion edits remain out of scope for
// the root-contract packet.
type Service interface {
	Ledger
	ProjectionService

	// Append publishes one ordered Factory Event through the plain
	// append/subscribe root-contract slice.
	Append(AppendRecordedEventRequest) AppendRecordedEventResult
	// SubscribeFrom opens a reconnect-aware event stream through the plain
	// append/subscribe root-contract slice.
	SubscribeFrom(context.Context, SubscribeRequest) (SubscribeResult, error)

	// ReconstructWorldState reconstructs a detached world-state/read-model
	// through the plain projection-query root-contract slice.
	ReconstructWorldState(ReconstructWorldStateRequest) (ReconstructWorldStateResult, error)
	// QuerySimpleDashboard returns simple dashboard render data through the
	// plain projection-query root-contract slice.
	QuerySimpleDashboard(SimpleDashboardQueryRequest) SimpleDashboardQueryResult
	// QueryWorkstationRequests returns workstation-request projection values
	// through the plain projection-query root-contract slice.
	QueryWorkstationRequests(WorkstationRequestsQueryRequest) WorkstationRequestsQueryResult
	// ValidateReconnectReplayFrom validates reconnect-replay inputs through
	// the plain projection-query root-contract slice.
	ValidateReconnectReplayFrom(ValidateReconnectReplayRequest) error

	// BindRecording constructs or binds one session-scoped recording through
	// the plain recording-lifecycle root-contract slice.
	BindRecording(BindRecordingRequest) (BindRecordingResult, error)
	// StartRecording starts periodic flush behavior for one bound recording.
	StartRecording(context.Context, StartRecordingRequest) (StartRecordingResult, error)
	// RecordRecordingEvent appends one Factory Event into a bound recording.
	RecordRecordingEvent(RecordRecordingEventRequest) (RecordRecordingEventResult, error)
	// RecordRecordingError retains a producer-boundary failure for flush/status.
	RecordRecordingError(RecordRecordingErrorRequest) (RecordRecordingErrorResult, error)
	// FlushRecording flushes one bound recording through the published slice.
	FlushRecording(FlushRecordingRequest) (FlushRecordingResult, error)
	// FinishRecording records terminal wall-clock metadata for one recording.
	FinishRecording(FinishRecordingRequest) (FinishRecordingResult, error)
	// StopRecording stops periodic flush behavior for one bound recording.
	StopRecording(StopRecordingRequest) (StopRecordingResult, error)
	// QueryRecordingStatus returns start/finish/stop/error status for one
	// bound recording.
	QueryRecordingStatus(RecordingStatusRequest) (RecordingStatusResult, error)

	// LoadReplayArtifact loads one detached replay artifact through the plain
	// replay root-contract slice.
	LoadReplayArtifact(LoadReplayArtifactRequest) (LoadReplayArtifactResult, error)
	// BindReplayExecution obtains the replay execution binding peers consume
	// through the plain replay root-contract slice.
	BindReplayExecution(BindReplayExecutionRequest) (BindReplayExecutionResult, error)
}

// ProjectionService is the projection-query capability surface embedded in the
// singular Recordings root Service. Peers should depend on Service rather than
// treating ProjectionService as a second peer-facing Recordings authority.
// Canonical replay reduction and dashboard projection stay owned here without
// exposing the concrete projections package.
type ProjectionService interface {
	ReconstructFactoryWorldState(
		[]interfaces.FactoryEvent,
		int,
	) (interfaces.FactoryWorldState, error)
	SimpleDashboardRenderData(interfaces.FactoryWorldState) SimpleDashboardRenderData
	ProjectActiveThrottlePauses(
		interfaces.InitialStructurePayload,
		[]interfaces.ActiveThrottlePause,
	) []interfaces.FactoryWorldThrottlePause
	ProjectWorkstationRequests(
		interfaces.FactoryWorldState,
	) WorkstationFactoryWorldWorkstationRequestProjectionSlice
	ValidateReconnectReplay(
		[]interfaces.FactoryEvent,
		interfaces.FactoryEventReconnectCursor,
		interfaces.FactoryEventReconnectScope,
	) error
}

// WorkstationRequestProjector exposes the canonical workstation-request read
// model without coupling consumers to the complete Recordings projection
// service.
type WorkstationRequestProjector interface {
	ProjectWorkstationRequests(
		interfaces.FactoryWorldState,
	) WorkstationFactoryWorldWorkstationRequestProjectionSlice
}

// WorldStateReconstructor is the narrow canonical reduction operation used by
// adapters that map external event shapes before invoking Recordings.
type WorldStateReconstructor func(
	[]interfaces.FactoryEvent,
	int,
) (interfaces.FactoryWorldState, error)

// ReplayArtifactLoader loads one canonical Factory-event replay artifact.
// Wire may still supply this construction helper for existing Runtime opening
// callers; peers should prefer LoadReplayArtifact on Service as the
// cross-service source of truth rather than treating loader injection as the
// peer seam.
type ReplayArtifactLoader func(string) (*interfaces.ReplayArtifact, error)

// InitialStructureSource is the only topology capability Recordings consumes.
// Factory Runtime implements it without exposing its concrete graph.
type InitialStructureSource interface {
	RecordingInitialStructure(
		...interfaces.RuntimeDefinitionLookup,
	) interfaces.InitialStructurePayload
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
type DispatchRecorder func(interfaces.FactoryDispatchRecord)

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

// RuntimeRecorder owns the lifecycle and durable flush behavior of one replay
// recording without exposing the concrete replay artifact writer. Existing
// Runtime callers may still consume this capability surface; peers should prefer
// the plain recording-lifecycle methods on Service as the cross-service source
// of truth rather than treating RuntimeRecorder construction as the peer seam.
type RuntimeRecorder interface {
	Start(context.Context)
	Stop()
	RecordEvent(interfaces.FactoryEvent)
	RecordError(error)
	Finish(time.Time)
	Flush() error
	Err() error
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
// runner, hooks, and completion policy consumed by Factory Runtime. Existing
// Runtime callers may still consume this construction helper; peers should
// prefer BindReplayExecution on Service as the cross-service source of truth
// rather than treating factory injection as the peer seam.
type ReplayExecutionFactory func(
	*interfaces.ReplayArtifact,
) (
	providercontract.Provider,
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
	RecordWorkstationRequest(int, interfaces.FactoryDispatchRecord, time.Time)
	RecordWorkstationResponse(int, workerexecution.WorkResult, interfaces.CompletedDispatch)
	RecordRunResponse(int, interfaces.FactoryState, string, time.Time)
	RecordWorkStateChange(int, work.WorkStateChangeRecord, time.Time)
	RecordFactoryStateChange(int, interfaces.FactoryState, interfaces.FactoryState, string, time.Time)
	RecordFactoryChange(int, interfaces.FactoryChangeEventPayload, time.Time)
	RecordSessionLifecycleFromFactoryConfig(string, *interfaces.FactoryConfig, int, time.Time)
	RecordSessionLifecycleCompletion(string, *interfaces.FactoryConfig, int, interfaces.FactoryState, string, time.Time)
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
	ActiveThrottlePauses             []interfaces.FactoryWorldThrottlePause
	PlaceTokenCounts                 map[string]int
	CurrentWorkItemsByPlaceID        map[string][]interfaces.FactoryWorldWorkItemRef
	PlaceOccupancyWorkItemsByPlaceID map[string][]interfaces.FactoryWorldWorkItemRef
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
	WorkItems       []interfaces.FactoryWorldWorkItemRef
}

type SimpleDashboardWorkstationActivity struct {
	NodeID            string
	WorkstationName   string
	ActiveDispatchIDs []string
	ActiveWorkItems   []interfaces.FactoryWorldWorkItemRef
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
	DispatchHistory      []interfaces.FactoryWorldDispatchCompletion
	ProviderSessions     []interfaces.FactoryWorldProviderSessionRecord
}

// Portable recording contracts are published at the Recordings service
// boundary so Factory Sessions do not depend on artifact implementation
// packages.
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

var (
	BuildPortableRecording    = recordingartifacts.Build
	DecodePortableRecording   = recordingartifacts.DecodeAndValidate
	ValidatePortableRecording = recordingartifacts.Validate
)
