package apisurface

import (
	"context"
	"errors"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workcontent"
)

// ModelAPI is the model catalog and direct-invocation seam for API handlers and
// bounded test doubles.
type ModelAPI interface {
	ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error)
	GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error)
	InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (ModelInvocationResult, error)
	PullModel(ctx context.Context, modelName string) (ModelPullResult, error)
}

// FactorySaveAPI is the session-scoped factory definition read and persist seam.
type FactorySaveAPI interface {
	GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error)
	SaveFactoryForSession(
		ctx context.Context,
		sessionID string,
		mode factoryapi.FactorySaveMode,
		request factoryapi.Factory,
	) (factoryapi.Factory, error)
	SaveCurrentFactoryForSession(ctx context.Context, sessionID string, request factoryapi.Factory) (factoryapi.Factory, error)
}

// SessionAPI is the factory-session inventory and lifecycle seam.
type SessionAPI interface {
	ListFactorySessions(ctx context.Context) (factoryapi.ListFactorySessionsResponse, error)
	GetFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySession, error)
	GetFactorySessionSyncPreflight(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (factoryapi.FactorySessionSyncPreflightResponse, error)
	GetFactorySessionResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionLiveResult, error)
	GetFactorySessionPartialResult(ctx context.Context, sessionID string) (factoryapi.FactorySessionPartialResult, error)
	OpenFactorySession(ctx context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error)
	CloseFactorySession(ctx context.Context, sessionID string) error
	PauseLiveFactorySession(ctx context.Context, sessionID string, request factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	ResumeLiveFactorySession(ctx context.Context, sessionID string, request factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
}

// WorkAPI is the session-scoped work submission, operator move, and runtime observability seam.
type WorkAPI interface {
	SubmitWorkRequestForSession(ctx context.Context, sessionID string, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error)
	MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (interfaces.OperatorMoveResult, error)
	SubscribeFactoryEventsForSession(ctx context.Context, sessionID string, reconnect *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error)
	GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error)
}

// InvocationAPI is the session-scoped factory invocation seam used by the API
// transport to submit one logical input and return the selected primary result.
type InvocationAPI interface {
	InvokeFactorySession(ctx context.Context, sessionID string, request factoryapi.InvocationRequest) (FactoryInvocationResult, error)
}

// DurableSessionLifecycleAPI is the shared durable session read and lifecycle
// control seam for pause, resume, cancel, terminate, approve, and retry-dispatch.
type DurableSessionLifecycleAPI interface {
	GetDurableFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySessionDurableReadModel, error)
	PauseDurableFactorySession(ctx context.Context, sessionID string, request factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	ResumeDurableFactorySession(ctx context.Context, sessionID string, request factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	CancelDurableFactorySession(ctx context.Context, sessionID string, request factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	TerminateDurableFactorySession(ctx context.Context, sessionID string, request factoryapi.FactorySessionLifecycleControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	ApproveDurableFactorySession(ctx context.Context, sessionID string, request factoryapi.FactorySessionApproveRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	RetryDurableFactorySessionDispatch(ctx context.Context, sessionID string, request factoryapi.FactorySessionRetryDispatchRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
	InterruptDurableFactorySessionDispatch(ctx context.Context, sessionID string, request factoryapi.FactorySessionInterruptDispatchRequest) (factoryapi.FactorySessionLifecycleControlResponse, error)
}

// DurableSessionExecutionAPI is the shared durable factory-session execution start
// seam for async and sync dynamic workflow sessions. Live-session open and
// invocation remain on SessionAPI and InvocationAPI.
type DurableSessionExecutionAPI interface {
	StartDurableFactorySessionAsync(ctx context.Context, request factoryapi.FactorySessionExecutionRequest) (factoryapi.FactorySessionExecutionResponse, error)
	StartDurableFactorySessionSync(ctx context.Context, request factoryapi.FactorySessionExecutionRequest) (factoryapi.FactorySessionSyncExecutionResponse, error)
}

// DurableSessionListingAPI is the shared scoped session listing seam for live and
// persisted durable execution sessions.
type DurableSessionListingAPI interface {
	ListDurableFactorySessions(ctx context.Context, params factoryapi.ListFactorySessionsParams) (factoryapi.ListFactorySessionsResponse, error)
}

// DurableSessionProjectionAPI is the shared durable session result, dispatch,
// artifact, and event replay seam for dynamic workflow inspection.
type DurableSessionProjectionAPI interface {
	GetDurableFactorySessionResult(ctx context.Context, sessionID string, params factoryapi.GetFactorySessionResultsParams) (factoryapi.FactorySessionResult, error)
	ListDurableFactorySessionDispatches(ctx context.Context, sessionID string) (factoryapi.ListFactorySessionDispatchesResponse, error)
	GetDurableFactorySessionDispatch(ctx context.Context, sessionID, dispatchID string) (factoryapi.FactoryDispatch, error)
	ListDurableFactorySessionArtifacts(ctx context.Context, sessionID string) (factoryapi.ListFactorySessionArtifactsResponse, error)
	GetDurableFactorySessionArtifact(ctx context.Context, sessionID, artifactID string) (factoryapi.FactorySessionArtifactDetail, error)
	ReadDurableFactorySessionEvents(ctx context.Context, sessionID string, params factoryapi.GetEventsBySessionIdParams) (*interfaces.FactoryEventStream, error)
}

// APISurface is the runtime seam consumed by the Agent Factory API server.
// It resolves requests against the service-owned current runtime so activation
// can swap the active runtime without leaving API reads pinned to startup
// state.
type APISurface interface {
	factory.APIFactory
	GetCurrentFactory(ctx context.Context) (factoryapi.Factory, error)
	ModelAPI
}

// SessionAPISurface extends APISurface with explicit per-session routing while
// preserving the legacy unscoped compatibility behavior through APISurface.
type SessionAPISurface interface {
	APISurface
	SessionAPI
	FactorySaveAPI
	WorkAPI
	InvocationAPI
	DurableSessionExecutionAPI
}

// FactoryInvocationResult carries the runtime-owned outcome of one session
// invocation request after input resolution and primary-result selection.
type FactoryInvocationResult struct {
	RequestID     string
	TraceID       string
	Status        factoryapi.InvocationTerminalStatus
	PrimaryResult []interfaces.WorkContentPart
	ErrorCode     string
	Message       string
	SessionID     string
	WorkID        string
	WorkName      string
	WorkState     string
}

// InvocationResponseFromResult maps a shared invocation result onto the public
// invocation response contract used by both API and CLI JSON surfaces.
func InvocationResponseFromResult(result FactoryInvocationResult) factoryapi.InvocationResponse {
	response := factoryapi.InvocationResponse{
		RequestId: result.RequestID,
		TraceId:   result.TraceID,
		Status:    result.Status,
	}
	if content := workcontent.GeneratedPtrFromParts(result.PrimaryResult); content != nil {
		response.PrimaryResult = content
	}
	if code := strings.TrimSpace(result.ErrorCode); code != "" {
		value := factoryapi.InvocationResponseErrorCode(code)
		response.ErrorCode = &value
	}
	if message := strings.TrimSpace(result.Message); message != "" {
		response.Message = &message
	}
	if sessionID := strings.TrimSpace(result.SessionID); sessionID != "" {
		response.SessionId = &sessionID
	}
	if workID := strings.TrimSpace(result.WorkID); workID != "" {
		response.WorkId = &workID
	}
	if workName := strings.TrimSpace(result.WorkName); workName != "" {
		response.WorkName = &workName
	}
	if workState := strings.TrimSpace(result.WorkState); workState != "" {
		response.WorkState = &workState
	}
	return response
}

// RequestValidationError reports a stable client-side validation failure that
// should map to HTTP 400 at the transport boundary.
type RequestValidationError struct {
	Message string
}

func (e *RequestValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// ErrFactoryActivationRequiresIdle reports that runtime replacement was
// attempted while the current runtime still had active work.
var ErrFactoryActivationRequiresIdle = errors.New("factory activation requires idle runtime")

// ErrInvalidNamedFactoryName reports that the requested named-factory name is
// not a safe canonical layout segment.
var ErrInvalidNamedFactoryName = errors.New("invalid named factory name")

// ErrInvalidNamedFactory reports that the submitted named-factory payload could
// not be persisted or validated as a runnable runtime config.
var ErrInvalidNamedFactory = errors.New("invalid named factory")

// ErrCurrentFactoryNotFound reports that no durable current-factory
// pointer could be resolved for named-factory readback.
var ErrCurrentFactoryNotFound = errors.New("current factory not found")

// ErrFactorySessionNotFound reports that no live session matched the requested
// public session identifier.
var ErrFactorySessionNotFound = errors.New("factory session not found")

// ErrInvalidEventReconnectCursor reports that the reconnect cursor did not
// match any recorded event in the targeted stream.
var ErrInvalidEventReconnectCursor = errors.New("invalid event reconnect cursor")

// ErrFactorySessionResultUnavailable reports that the requested session does not
// expose JavaScript result or partial-result reads.
var ErrFactorySessionResultUnavailable = errors.New("factory session result unavailable")

// ErrFactoryVersionStale reports that a complete current-factory
// save was based on an older factory definition version than the current one.
var ErrFactoryVersionStale = errors.New("factory version is stale")

// ErrModelNotFound reports that the requested discovered model identifier is
// not present in the current runtime configuration.
var ErrModelNotFound = errors.New("model not found")

// ErrModelNotAvailable reports that a discovered local model exists but its
// required local assets are not present in the managed cache.
var ErrModelNotAvailable = errors.New("model not available")

// ErrModelPullUnsupported reports that the requested model does not support
// managed local asset pulls in the current runtime or platform.
var ErrModelPullUnsupported = errors.New("model pull is not supported")

// ErrManagedRuntimeSourceFetchFailed reports that required managed runtime
// assets could not be fetched from the configured backend source.
var ErrManagedRuntimeSourceFetchFailed = errors.New("managed runtime source fetch failed")

// ErrModelInvocationUnsupportedMode reports that the requested direct
// invocation response mode is not valid for the selected operation output.
var ErrModelInvocationUnsupportedMode = errors.New("model invocation response mode is not supported")

// ErrModelInvocationUnsupportedOperation reports that the targeted model does
// not expose the requested provider-agnostic operation.
var ErrModelInvocationUnsupportedOperation = errors.New("model invocation operation is not supported")

// ModelInvocationResult carries the backend-owned direct invocation result used
// by the API transport for either JSON metadata or streamed audio responses.
type ModelInvocationResult struct {
	ModelName         string
	Worker            string
	Operation         string
	ProviderLocality  string
	Content           []interfaces.WorkContentPart
	Bindings          []interfaces.ResolvedModelOperationBinding
	StreamFile        string
	StreamContentType string
}

// ModelPullDownloadedFile describes one cached artifact materialized by a
// managed local-model asset pull.
type ModelPullDownloadedFile struct {
	Path   string
	Bytes  int64
	SHA256 string
}

// ModelPullResult carries the service-owned result of pulling one model into
// the managed local cache.
type ModelPullResult struct {
	ModelName          string
	ProviderLocality   string
	Outcome            string
	CachePath          string
	Revision           string
	DownloadedFiles    []ModelPullDownloadedFile
	ManagedPullOutcome string
	ReadinessState     string
	LifecycleState     string
	SourceKind         string
	SourceID           string
	ResolverNotes      string
}

// TopologyValidationError carries validation targets that the graph editor can
// map back to form fields, nodes, edges, or save-level messages.
type TopologyValidationError struct {
	Message string
	Targets []factoryapi.FactoryValidationTarget
}

func (e *TopologyValidationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return "factory topology validation failed"
}

func (e *TopologyValidationError) Is(target error) bool {
	return target == ErrInvalidNamedFactory
}

func NewTopologyValidationError(message string, targets []factoryapi.FactoryValidationTarget) *TopologyValidationError {
	if message == "" {
		message = "factory topology validation failed"
	}
	return &TopologyValidationError{
		Message: message,
		Targets: append([]factoryapi.FactoryValidationTarget(nil), targets...),
	}
}

// DefaultCurrentFactoryName is the reserved current-factory identifier used
// when the active runtime is the root factory and no named-factory pointer
// exists.
const DefaultCurrentFactoryName factoryapi.FactoryName = "UNDEFINED"
