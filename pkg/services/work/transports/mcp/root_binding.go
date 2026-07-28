package workmcp

import (
	work "github.com/portpowered/infinite-you/pkg/services/work"
)

// WorkRoot is the accepted Work root contract used by the MCP adapter.
// Adapter-owned operations invoke this surface rather than Work internal packages.
type WorkRoot = work.Service

// RootDependencies are the accepted Work root roles consumed by the MCP adapter.
// Work is the singular Service root; transports inject an implementation or test
// fake rather than importing Work internals or constructing canonical state.
type RootDependencies struct {
	Work WorkRoot
}

// NewFromRoot constructs an MCP tool operation that calls through the supplied
// Work root binding. Tests inject focused fakes without constructing real
// admission, content-staging, content-materialization, state-access graphs, or
// service-local Wire.
func NewFromRoot(deps RootDependencies) ToolOperation {
	return Bind(deps)
}
