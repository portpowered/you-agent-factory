package wire

import (
	"context"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionmcp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
)

// NewMCPSubagentOperation binds RUN to the exact selected HTTP host.
func NewMCPSubagentOperation(server string, transport factorysessionmcp.HostTransport, workingRoot string, generateID factorysessions.SessionIDGenerator, resolveProvider factorysessionmcp.ProviderIdentityResolver) (func(context.Context, factorysessionmcp.SubagentInput) factorysessionmcp.ToolResponse[factorysessionmcp.SubagentResult], error) {
	return factorysessionmcp.NewHostOperation(server, transport, workingRoot, generateID, resolveProvider)
}
