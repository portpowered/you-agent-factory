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
// Wire supplies the concrete persistence implementation.
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
// recording without exposing the concrete replay artifact writer.
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
// runner, hooks, and completion policy consumed by Factory Runtime.
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
