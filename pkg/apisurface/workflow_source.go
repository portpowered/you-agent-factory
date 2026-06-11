package apisurface

import (
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

// NormalizeWorkflowSourceRequest is the shared API, CLI, MCP, and website entry
// point for workflow source lookup and artifact-root validation.
func NormalizeWorkflowSourceRequest(req workflowsource.Request, ctx workflowsource.Context) workflowsource.Resolution {
	return workflowsource.Resolve(req, ctx)
}
