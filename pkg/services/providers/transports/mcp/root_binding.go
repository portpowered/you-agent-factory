package providersmcp

import (
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// ProvidersRoot is the accepted Providers root contract used by the MCP adapter.
// Adapter-owned operations invoke this surface rather than Providers internal packages.
type ProvidersRoot = providers.Service

// RootDependencies are the accepted Providers root roles consumed by the MCP adapter.
// Providers is the singular Service root; transports inject an implementation or
// test fake rather than importing Providers internals or constructing canonical state.
type RootDependencies struct {
	Providers ProvidersRoot
}

// NewFromRoot constructs an MCP tool operation that calls through the supplied
// Providers root binding. Tests inject focused fakes without constructing real
// catalog, execution, or service-local Wire graphs.
func NewFromRoot(deps RootDependencies) ToolOperation {
	return Bind(deps)
}
