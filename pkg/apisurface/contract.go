package apisurface

import (
	"context"
	"errors"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
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
	OpenFactorySession(ctx context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error)
	CloseFactorySession(ctx context.Context, sessionID string) error
}

// WorkAPI is the session-scoped work submission, operator move, and runtime observability seam.
type WorkAPI interface {
	SubmitWorkRequestForSession(ctx context.Context, sessionID string, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error)
	MoveWorkForSession(ctx context.Context, sessionID, workID, stateName, requestID string) (interfaces.OperatorMoveResult, error)
	SubscribeFactoryEventsForSession(ctx context.Context, sessionID string) (*interfaces.FactoryEventStream, error)
	GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error)
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
	ModelName        string
	ProviderLocality string
	Outcome          string
	CachePath        string
	Revision         string
	DownloadedFiles  []ModelPullDownloadedFile
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
