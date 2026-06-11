package apisurface

import (
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

// NormalizeWorkflowSourceRequest is the shared API, CLI, MCP, and website entry
// point for workflow source lookup and artifact-root validation.
func NormalizeWorkflowSourceRequest(req source.Request, ctx source.Context) source.Resolution {
	return source.Resolve(req, ctx)
}
