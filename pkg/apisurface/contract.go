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

// APISurface is the runtime seam consumed by the Agent Factory API server.
// It resolves requests against the service-owned current runtime so activation
// can swap the active runtime without leaving API reads pinned to startup
// state.
type APISurface interface {
	factory.APIFactory
	CreateNamedFactory(ctx context.Context, namedFactory factoryapi.Factory) (factoryapi.Factory, error)
	GetCurrentNamedFactory(ctx context.Context) (factoryapi.Factory, error)
	GetEditableFactoryDefinition(ctx context.Context) (factoryapi.EditableFactoryDefinition, error)
	SaveEditableFactoryDefinition(ctx context.Context, request factoryapi.SaveEditableFactoryDefinitionRequest) (factoryapi.EditableFactoryDefinition, error)
	ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error)
	GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error)
	InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (ModelInvocationResult, error)
}

// SessionAPISurface extends APISurface with explicit per-session routing while
// preserving the legacy unscoped compatibility behavior through APISurface.
type SessionAPISurface interface {
	APISurface
	ListFactorySessions(ctx context.Context) (factoryapi.ListFactorySessionsResponse, error)
	OpenFactorySession(ctx context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error)
	CloseFactorySession(ctx context.Context, sessionID string) error
	SubmitWorkRequestForSession(ctx context.Context, sessionID string, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error)
	SubscribeFactoryEventsForSession(ctx context.Context, sessionID string) (*interfaces.FactoryEventStream, error)
	GetEngineStateSnapshotForSession(ctx context.Context, sessionID string) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error)
	GetCurrentNamedFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error)
	GetEditableFactoryDefinitionForSession(ctx context.Context, sessionID string) (factoryapi.EditableFactoryDefinition, error)
	SaveEditableFactoryDefinitionForSession(ctx context.Context, sessionID string, request factoryapi.SaveEditableFactoryDefinitionRequest) (factoryapi.EditableFactoryDefinition, error)
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

// ErrCurrentNamedFactoryNotFound reports that no durable current-factory
// pointer could be resolved for named-factory readback.
var ErrCurrentNamedFactoryNotFound = errors.New("current named factory not found")

// ErrFactorySessionNotFound reports that no live session matched the requested
// public session identifier.
var ErrFactorySessionNotFound = errors.New("factory session not found")

// ErrEditableFactoryVersionStale reports that a complete editable-definition
// save was based on an older factory definition version than the current one.
var ErrEditableFactoryVersionStale = errors.New("editable factory definition version is stale")

// ErrModelNotFound reports that the requested discovered model identifier is
// not present in the current runtime configuration.
var ErrModelNotFound = errors.New("model not found")

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

// TopologyValidationError carries validation targets that the graph editor can
// map back to form fields, nodes, edges, or save-level messages.
type TopologyValidationError struct {
	Message string
	Targets []factoryapi.ErrorTarget
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

func NewTopologyValidationError(message string, targets []factoryapi.ErrorTarget) *TopologyValidationError {
	if message == "" {
		message = "factory topology validation failed"
	}
	return &TopologyValidationError{
		Message: message,
		Targets: append([]factoryapi.ErrorTarget(nil), targets...),
	}
}

// DefaultCurrentFactoryName is the reserved current-factory identifier used
// when the active runtime is the root factory and no named-factory pointer
// exists.
const DefaultCurrentFactoryName factoryapi.FactoryName = "UNDEFINED"
