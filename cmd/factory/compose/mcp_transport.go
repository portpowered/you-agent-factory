package compose

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/initializer"
)

// InjectMCPTransport composes MCP tool dependencies through pkg/initializer
// without using the wire FactoryService composition root.
func InjectMCPTransport(ctx context.Context, cfg *initializer.MCPConfig) (*initializer.MCPTransport, error) {
	return initializer.InitializeMCPTransport(ctx, cfg)
}
