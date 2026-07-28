package modelmcp

import (
	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// ModelsRoot is the accepted Models root contract used by the MCP adapter.
// Adapter-owned operations invoke this surface rather than Models internal
// packages.
type ModelsRoot = models.Service

// RootBinding binds the MCP adapter to one injected Models root.
type RootBinding struct {
	Models ModelsRoot
}

// NewFromRoot constructs an MCP tool operation that calls through the supplied
// Models root binding. Tests inject focused fakes without constructing real
// catalog, assets, host, lease, inference graphs, or service-local Wire.
func NewFromRoot(binding RootBinding) ToolOperation {
	return Bind(binding)
}
