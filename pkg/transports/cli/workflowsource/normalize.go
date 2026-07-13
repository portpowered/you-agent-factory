package workflowsource

import (
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// NormalizeRequest is the CLI entry point for shared workflow source lookup.
func NormalizeRequest(req workflowsource.Request, ctx workflowsource.Context) workflowsource.Resolution {
	return apisurface.NormalizeWorkflowSourceRequest(req, ctx)
}
