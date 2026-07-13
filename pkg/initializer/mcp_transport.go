package initializer

import (
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
)

// MCPOptions carries MCP-specific composition inputs beyond factory startup config.
type MCPOptions struct {
	FixtureCatalogPath string
	RuntimeBacked      bool
	ProjectRoot        string
}

// MCPConfig combines optional factory startup config with MCP serve options.
type MCPConfig struct {
	Factory *Config
	Options MCPOptions
}

// MCPTransport bundles initializer-produced domain services with the durable
// Factory Session execution service used by MCP tool handlers without
// constructing root FactoryService at the composition boundary.
type MCPTransport struct {
	Services         *Services
	SessionExecution factorysessionexecution.Service
}

// SessionClient returns the MCP Factory Session client backed by the composed
// durable session execution service.
func (t *MCPTransport) SessionClient() *mcpfactorysession.Client {
	if t == nil || t.SessionExecution == nil {
		return mcpfactorysession.NewClient()
	}
	return mcpfactorysession.NewClientWithService(t.SessionExecution)
}
