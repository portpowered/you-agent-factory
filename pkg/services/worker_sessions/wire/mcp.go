package wire

import (
	workersessionmcp "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/mcp"
)

// NewMCPAdapter binds one generated client to the selected host at construction.
func NewMCPAdapter(host workersessionmcp.HostClient) (*workersessionmcp.Adapter, error) {
	return workersessionmcp.New(host)
}
