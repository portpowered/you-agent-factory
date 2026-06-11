package apisurface

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
)

// BuildWorkflowSessionLiveResult projects the live terminal session result read shape.
func BuildWorkflowSessionLiveResult(input workflowresult.SessionResultInput) factoryapi.FactorySessionLiveResult {
	return workflowresult.BuildLiveSessionResult(input)
}

// BuildWorkflowSessionResult projects the durable terminal session result read shape.
func BuildWorkflowSessionResult(input workflowresult.SessionResultInput) factoryapi.FactorySessionResult {
	return workflowresult.BuildSessionResult(input)
}

// BuildWorkflowSessionResultUpdatedPayload projects the SESSION_RESULT_UPDATED
// event payload from the shared session result contract.
func BuildWorkflowSessionResultUpdatedPayload(input workflowresult.SessionResultInput) factoryapi.SessionResultUpdatedEventPayload {
	return workflowresult.BuildSessionResultUpdatedPayload(input)
}
