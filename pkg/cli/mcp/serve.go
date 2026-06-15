package mcpcli

import (
	"context"
	"io"
	"os"

	mcpserver "github.com/portpowered/infinite-you/pkg/mcp/server"
)

// ServeConfig holds parameters for the canonical MCP stdio serve command.
type ServeConfig struct {
	Input  io.Reader
	Output io.Writer
}

// Serve runs the repo-owned workflow preview MCP server over stdio until the
// client disconnects or the context is canceled.
func Serve(ctx context.Context, cfg ServeConfig) error {
	in := cfg.Input
	if in == nil {
		in = os.Stdin
	}
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}
	return mcpserver.ServeStdio(ctx, in, out)
}
