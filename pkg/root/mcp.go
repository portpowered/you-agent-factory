package root

import (
	"errors"

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
)

// MCPServerForExecution returns the MCP server factory composed into a root
// process, bound to one already-opened durable execution. It keeps callers on
// the public root boundary while the process and Wire retain transport
// construction policy.
func MCPServerForExecution(
	process *initializerapplication.Process,
	execution any,
) (processcontract.MCPServer, error) {
	if process == nil {
		return nil, errors.New("build MCP server: process is required")
	}
	factory := process.MCPServerFactory()
	if factory == nil {
		return nil, errors.New("build MCP server: process MCP server factory is required")
	}
	return factory(execution)
}
