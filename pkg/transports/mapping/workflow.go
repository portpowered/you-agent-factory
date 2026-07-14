package apisurface

import (
	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ResolveWorkflowPolicy is the shared entry point for effective policy resolution.
func ResolveWorkflowPolicy(request workflowpolicy.Request) workflowpolicy.Resolution {
	return workflowpolicy.Resolve(request)
}

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

// NormalizeWorkflowSourceRequest is the shared API, CLI, MCP, and website entry
// point for workflow source lookup and artifact-root validation.
func NormalizeWorkflowSourceRequest(req workflowsource.Request, ctx workflowsource.Context) workflowsource.Resolution {
	return workflowsource.Resolve(req, ctx)
}
