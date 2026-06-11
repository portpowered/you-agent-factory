package workflowsource

import (
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

// NormalizeRequest is the CLI entry point for shared workflow source lookup.
func NormalizeRequest(req source.Request, ctx source.Context) source.Resolution {
	return apisurface.NormalizeWorkflowSourceRequest(req, ctx)
}
