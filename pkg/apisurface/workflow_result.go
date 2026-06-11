package apisurface

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
)

// BuildWorkflowSessionLiveResult projects the live terminal session result read shape.
func BuildWorkflowSessionLiveResult(input result.SessionResultInput) factoryapi.FactorySessionLiveResult {
	return result.BuildLiveSessionResult(input)
}

// BuildWorkflowSessionResult projects the durable terminal session result read shape.
func BuildWorkflowSessionResult(input result.SessionResultInput) factoryapi.FactorySessionResult {
	return result.BuildSessionResult(input)
}

// BuildWorkflowSessionResultUpdatedPayload projects the SESSION_RESULT_UPDATED
// event payload from the shared session result contract.
func BuildWorkflowSessionResultUpdatedPayload(input result.SessionResultInput) factoryapi.SessionResultUpdatedEventPayload {
	return result.BuildSessionResultUpdatedPayload(input)
}
