package apisurface

import (
	"context"
	"errors"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
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

// ErrEditableFactoryVersionStale reports that a complete editable-definition
// save was based on an older factory definition version than the current one.
var ErrEditableFactoryVersionStale = errors.New("editable factory definition version is stale")

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
