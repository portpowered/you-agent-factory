// Package stdio owns construction of inert MCP stdio transport sessions.
package stdio

import (
	"context"
	"fmt"
	"io"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingmcp "github.com/portpowered/infinite-you/pkg/services/recordings/transports/mcp"
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
	mcpfactorysession.DurableExecution,
	recordings.Service,
	mcpfactorysession.RequestPreparation,
	factoryruntime.WorkflowPreviewOperation,
	io.Reader,
	io.Writer,
) (Session, error)

func NewOpener() Opener { return Open }

// Open binds invocation-local streams and an opened Factory Session execution
// role to an inert MCP protocol server.
func Open(
	execution mcpfactorysession.DurableExecution,
	recordingsService recordings.Service,
	prepare mcpfactorysession.RequestPreparation,
	workflows factoryruntime.WorkflowPreviewOperation,
	input io.Reader,
	output io.Writer,
) (Session, error) {
	if input == nil || output == nil {
		return nil, fmt.Errorf("MCP stdio streams are required")
	}
	inspection := recordingmcp.FactorySessionInspectionService(recordingsService)
	if inspection == nil {
		inspection = recordingmcp.NewLegacyFactorySessionInspection(execution)
	}
	server, err := mcpserver.New(mcpserver.Options{
		ToolOperation: mcpfactorysession.BindToolOperation(execution, inspection, prepare, workflows),
	})
	if err != nil {
		return nil, err
	}
	return &session{server: server, input: input, output: output}, nil
}
