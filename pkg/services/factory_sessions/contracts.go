package factorysessions

import (
	"context"
	"io"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	internalcontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/contracts"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// DefinitionActivationGateway is retained as the Sessions implementation's
// view of the gateway contract, whose authoritative definition belongs to
// Factory Definitions. It is a value contract, not a second Sessions service
// authority.
type DefinitionActivationGateway = interfaces.DefinitionActivationGateway

// InvocationMetric records one emitted runtime counter together with its
// low-cardinality dimensions.
type InvocationMetric = internalcontracts.InvocationMetric

// SessionIDGenerator supplies opaque identities for live sessions, durable
// sessions, and session-owned invocation requests.
type SessionIDGenerator = internalcontracts.SessionIDGenerator

// Effect-port contracts are published from the Sessions root as aliases so
// peers and Wire bind through factorysessions names without adding service
// authorities to the root.
type (
	ExecutionOpeningFileSystem           = internalcontracts.ExecutionOpeningFileSystem
	DirectoryInspection                  = internalcontracts.DirectoryInspection
	CursorPersistenceFileSystem          = internalcontracts.CursorPersistenceFileSystem
	CursorPersistenceTemporaryFile       = internalcontracts.CursorPersistenceTemporaryFile
	CursorPersistenceCreateTemporaryFile = internalcontracts.CursorPersistenceCreateTemporaryFile
	RuntimePersistenceFileSystem         = internalcontracts.RuntimePersistenceFileSystem
	InvocationMetricsRecorder            = internalcontracts.InvocationMetricsRecorder
)

// InvocationTarget contains the detached configuration selected for one
// invocation. Operations remain consumer-owned interfaces.
type InvocationTarget struct {
	FactoryDir            string
	FactorySourcePath     string
	RunnerID              string
	WorkerReasoningEffort string
	Worktree              string
	OperatorDefaults      operatorsettings.ResolvedDefaults
	ExecutionBaseDir      string
	HomeDir               string
	Verbose               bool
	RecordPath            string
	ReplayPath            string
	ResumePath            string
	// CanonicalSessionID is the preallocated runtime UUID for an automatic
	// default-session recording. It remains separate from the public alias and
	// prevents the reserved empty artifact from being treated as prior board
	// history during one-shot startup.
	CanonicalSessionID               string
	RuntimeLogDir                    string
	RuntimeLogConfig                 factoryruntime.RuntimeLogStorageConfig
	RuntimeMetricsDir                string
	RuntimeMetricsConfig             factoryruntime.RuntimeMetricsStorageConfig
	ModelCacheDir                    string
	WorkflowID                       string
	MockWorkersConfig                *workers.MockWorkersConfig
	SkipPermissionsOverride          *bool
	SkipRunnerPrerequisiteValidation bool
	InvocationArguments              *work.InvocationArguments
	// EventScopeID identifies owner-private Factory Event presentation state.
	// It is the only invocation-time handle for transport event delivery.
	EventScopeID OpeningScopeID
}

// FactoryInvocationOutcome is the detached result of one Factory invocation.
type FactoryInvocationOutcome struct {
	Result interfaces.FactoryInvocationResult
}

// FactoryEventConsumer receives ordered canonical events for an owner-private
// invocation presentation scope. It is registered with OpeningPresentationOwner
// and never crosses an invocation operation boundary.
type FactoryEventConsumer func([]interfaces.FactoryEvent)

// OpeningScopeID is an opaque process-local identity for transport-owned
// presentation state. Operation requests carry this identity instead of
// retaining streams, observers, services, or callbacks.
type OpeningScopeID string

// VisualizationSinkID identifies a transport-selected visualization sink
// without importing the Factory Visualization implementation into the
// Factory Sessions root contract.
type VisualizationSinkID string

// DirectJavaScriptRunScope holds protocol presentation state behind the
// process-scoped opening owner. Direct JavaScript requests carry only values
// and the opaque scope identity.
type DirectJavaScriptRunScope struct {
	Output              io.Writer
	RuntimeHostObserver RuntimeHostObserver
}

// StdioOpeningScope holds protocol streams behind the process-scoped opening
// owner. MCP opening requests carry only values and the opaque scope identity.
type StdioOpeningScope struct {
	Input  io.Reader
	Output io.Writer
}

// InvocationEventScope retains one transport's canonical Factory Event
// presentation callback behind the process-scoped owner.
type InvocationEventScope struct {
	Consume FactoryEventConsumer
}

// OpeningPresentationOwner owns only transport-local stream and event state
// associated with value-only protocol operations. Canonical Wire constructs
// one owner once and each transport adapter registers a scope before invoking
// an operation. Application HTTP binding, visualization sink selection, and
// runtime-host readiness are owned by their canonical Wire/initializer paths.
type OpeningPresentationOwner interface {
	RegisterDirectJavaScript(DirectJavaScriptRunScope) (OpeningScopeID, error)
	DirectJavaScript(OpeningScopeID) (DirectJavaScriptRunScope, bool)
	RegisterStdio(StdioOpeningScope) (OpeningScopeID, error)
	Stdio(OpeningScopeID) (StdioOpeningScope, bool)
	RegisterInvocationEvents(InvocationEventScope) (OpeningScopeID, error)
	InvocationEvents(OpeningScopeID) (FactoryEventConsumer, bool)
	// StartFactoryEventBridge returns owner-private lifecycle state. The
	// anonymous contract keeps this transport collaborator out of the service
	// root's named interface inventory; callers exchange only the injected
	// service root and detached outcome.
	StartFactoryEventBridge(context.Context, interface {
		SubscribeFactoryEventsForSession(context.Context, string, *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error)
		ReadDurableFactorySessionEventStream(context.Context, string, EventReconnectRequest) (*interfaces.FactoryEventStream, error)
	}, OpeningScopeID) (interface {
		Finish(context.Context, interface {
			SubscribeFactoryEventsForSession(context.Context, string, *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error)
			ReadDurableFactorySessionEventStream(context.Context, string, EventReconnectRequest) (*interfaces.FactoryEventStream, error)
		}, FactoryInvocationOutcome) error
	}, error)
	Close(OpeningScopeID)
}

// HistoricalReplayInspection is the detached public read model restored from
// a portable Factory Session recording. It uses the same Factory Session,
// artifact, result, and ordered-event facts as the ordinary inspection
// surfaces while making its read-only and redaction boundaries explicit.
type HistoricalReplayInspection struct {
	Session   SessionReadResult
	Events    EventReadResult
	Artifacts ListArtifactsResult
	Result    ResultReadResult
	// WorkerHistory is derived from the recording schema. Legacy recordings
	// report an explicit unavailable outcome instead of an empty history.
	WorkerHistory recordings.PortableRecordingWorkerHistory
	Checkpoint    *HistoricalReplayCheckpoint
	Redaction     HistoricalReplayRedaction
}

// HistoricalReplayCheckpoint is the public checkpoint summary available from
// a historical portable recording. It never includes checkpoint body state.
type HistoricalReplayCheckpoint struct {
	ID, Label, Summary, ArtifactID string
	Timestamp                      time.Time
}

// HistoricalReplayRedaction identifies the intentionally omitted recording
// content without exposing any omitted content or integrity material.
type HistoricalReplayRedaction struct {
	RuntimeStateOmitted        bool
	CheckpointBodiesOmitted    bool
	ProviderTranscriptsOmitted bool
	ChildDispatchesOmitted     bool
	SecretsRedacted            int64
}

// FactoryScaffoldInitializer initializes a newly selected Factory directory.
type FactoryScaffoldInitializer func(string) error

// EditableFactoryValidator validates a detached Factory definition before
// persistence.
type EditableFactoryValidator func(
	context.Context,
	*interfaces.FactorySnapshot,
	interfaces.WorkstationLoader,
) error

// ReconnectCursorValidator validates an acknowledged cursor against a
// detached canonical event snapshot. Recordings supplies the implementation.
type ReconnectCursorValidator func(
	[]interfaces.FactoryEvent,
	interfaces.FactoryEventReconnectCursor,
	interfaces.FactoryEventReconnectScope,
) error
