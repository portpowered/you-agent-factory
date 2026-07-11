package apisurface

import (
	"fmt"
	"reflect"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
)

// DurableSessionAPI groups the durable execution role consumed by HTTP
// handlers, including start, listing, projection, and lifecycle operations.
// Keeping these capabilities on one collaborator preserves the server's
// optional-interface routes when the handler surface is composed.
type DurableSessionAPI interface {
	DurableSessionExecutionAPI
	DurableSessionListingAPI
	DurableSessionProjectionAPI
	DurableSessionLifecycleAPI
}

type composedSessionAPISurface struct {
	SessionAPI
	ModelAPI
	FactorySaveAPI
	InvocationAPI
	DurableSessionAPI
}

// NewSessionAPISurface composes the handler contract from five explicit domain
// collaborators. The composed surface is stateless: every method is promoted
// directly from its owning collaborator without adding policy or side effects.
func NewSessionAPISurface(
	session SessionAPI,
	model ModelAPI,
	factoryDefinition FactorySaveAPI,
	invocation InvocationAPI,
	durableExecution DurableSessionAPI,
) (SessionAPISurface, error) {
	required := []struct {
		role         string
		collaborator any
	}{
		{role: "session", collaborator: session},
		{role: "model", collaborator: model},
		{role: "factory-definition", collaborator: factoryDefinition},
		{role: "invocation", collaborator: invocation},
		{role: "durable-execution", collaborator: durableExecution},
	}
	for _, dependency := range required {
		if isNilCollaborator(dependency.collaborator) {
			return nil, fmt.Errorf("compose session API surface: %s collaborator is required", dependency.role)
		}
	}

	return &composedSessionAPISurface{
		SessionAPI:        session,
		ModelAPI:          model,
		FactorySaveAPI:    factoryDefinition,
		InvocationAPI:     invocation,
		DurableSessionAPI: durableExecution,
	}, nil
}

func isNilCollaborator(collaborator any) bool {
	if collaborator == nil {
		return true
	}
	value := reflect.ValueOf(collaborator)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// ValidateWritableNamedFactoryName enforces the public named-factory contract
// for create/import paths. The reserved default-current identifier is valid for
// readback only and must never be persisted as a customer-named factory.
func ValidateWritableNamedFactoryName(name factoryapi.FactoryName) error {
	if err := factoryconfig.ValidateNamedFactoryName(string(name)); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidNamedFactoryName, err)
	}
	if name == DefaultCurrentFactoryName {
		return fmt.Errorf("%w: %q is reserved for current-factory readback", ErrInvalidNamedFactoryName, name)
	}
	return nil
}

var _ SessionAPISurface = (*composedSessionAPISurface)(nil)
