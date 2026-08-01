package wire

import (
	"fmt"
	"net/http"

	transportcli "github.com/portpowered/infinite-you/pkg/transports/cli"
	transporthttp "github.com/portpowered/infinite-you/pkg/transports/http"
	mcpserver "github.com/portpowered/infinite-you/pkg/transports/mcp/server"
)

// TransportAggregate is the process-scoped protocol handoff produced by Wire.
// It retains only already-composed protocol adapters and registries; lifecycle
// activation and owner operation selection remain outside this value.
type TransportAggregate struct {
	HTTP        *transporthttp.Forwarder
	HTTPHandler http.Handler
	CLI         *transportcli.FamilyRegistry
	MCP         mcpserver.ToolRegistry
}

// NewTransportAggregate validates and snapshots the three protocol surfaces.
// The identity of every supplied collaborator is retained so one owner
// adapter/registry cannot be silently replaced by a second graph during an
// application invocation.
func NewTransportAggregate(
	httpForwarder *transporthttp.Forwarder,
	cliRegistry *transportcli.FamilyRegistry,
	mcpRegistry mcpserver.ToolRegistry,
) (*TransportAggregate, error) {
	if httpForwarder == nil {
		return nil, fmt.Errorf("construct transport aggregate: HTTP forwarder is required")
	}
	httpHandler, err := transporthttp.NewComposedHandler(httpForwarder)
	if err != nil {
		return nil, err
	}
	switch {
	case cliRegistry == nil:
		return nil, fmt.Errorf("construct transport aggregate: CLI family registry is required")
	case mcpRegistry == nil:
		return nil, fmt.Errorf("construct transport aggregate: MCP tool registry is required")
	default:
		return &TransportAggregate{
			HTTP:        httpForwarder,
			HTTPHandler: httpHandler,
			CLI:         cliRegistry,
			MCP:         mcpRegistry,
		}, nil
	}
}
