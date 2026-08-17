// Package stdio owns construction of inert MCP stdio transport sessions.
package stdio

import (
	"context"
	"fmt"
	"io"

	mcpserver "github.com/portpowered/infinite-you/pkg/transports/mcp/server"
)

// Session is one inert MCP stdio transport invocation.
type Session interface {
	Run(context.Context) error
}

type session struct {
	server *mcpserver.Server
	input  io.Reader
	output io.Writer
}

func (s *session) Run(ctx context.Context) error {
	if s == nil || s.server == nil {
		return fmt.Errorf("MCP stdio session is required")
	}
	return s.server.ServeStdio(ctx, s.input, s.output)
}

type Opener func(
	*mcpserver.Server,
	io.Reader,
	io.Writer,
) (Session, error)

func NewOpener() Opener { return Open }

// Open binds invocation-local streams to an already-composed MCP protocol
// server. Service adapters and execution roles are composed by Wire before
// this transport receives the server.
func Open(
	server *mcpserver.Server,
	input io.Reader,
	output io.Writer,
) (Session, error) {
	if input == nil || output == nil {
		return nil, fmt.Errorf("MCP stdio streams are required")
	}
	if server == nil {
		return nil, fmt.Errorf("MCP stdio server is required")
	}
	return &session{server: server, input: input, output: output}, nil
}
