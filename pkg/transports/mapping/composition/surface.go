// Package composition composes stateless transport-facing application surfaces
// from explicit collaborators assembled by the application graph.
package composition

import (
	"fmt"
	"reflect"

	"github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// DurableSessionAPI is the durable execution role consumed by transports.
type DurableSessionAPI = apisurface.DurableSessionAPI

type composedSessionAPISurface struct {
	apisurface.SessionAPI
	apisurface.ModelAPI
	apisurface.FactorySaveAPI
	apisurface.InvocationAPI
	DurableSessionAPI
}

// NewSessionAPISurface composes the handler contract from five explicit
// application collaborators. The composed surface is stateless: every method
// is promoted directly from its owning collaborator without adding policy,
// normalization, or side effects.
func NewSessionAPISurface(
	session apisurface.SessionAPI,
	model apisurface.ModelAPI,
	factoryDefinition apisurface.FactorySaveAPI,
	invocation apisurface.InvocationAPI,
	durableExecution DurableSessionAPI,
) (apisurface.SessionAPISurface, error) {
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

var _ apisurface.SessionAPISurface = (*composedSessionAPISurface)(nil)
