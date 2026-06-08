package workflowsource

import (
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/workflowsource"
)

// NormalizeRequest is the CLI entry point for shared workflow source lookup.
func NormalizeRequest(req workflowsource.Request, ctx workflowsource.Context) workflowsource.Resolution {
	return apisurface.NormalizeWorkflowSourceRequest(req, ctx)
}
