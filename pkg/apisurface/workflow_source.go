package apisurface

import (
	"github.com/portpowered/infinite-you/pkg/workflowsource"
)

// NormalizeWorkflowSourceRequest is the shared API, CLI, MCP, and website entry
// point for workflow source lookup and artifact-root validation.
func NormalizeWorkflowSourceRequest(req workflowsource.Request, ctx workflowsource.Context) workflowsource.Resolution {
	return workflowsource.Resolve(req, ctx)
}
