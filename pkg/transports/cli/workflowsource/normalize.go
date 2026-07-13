package workflowsource

import (
	"github.com/portpowered/infinite-you/pkg/apisurface"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

// NormalizeRequest is the CLI entry point for shared workflow source lookup.
func NormalizeRequest(req workflowsource.Request, ctx workflowsource.Context) workflowsource.Resolution {
	return apisurface.NormalizeWorkflowSourceRequest(req, ctx)
}
