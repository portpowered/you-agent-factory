package wire

import (
	workersessionmcp "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/mcp"
	client "github.com/portpowered/infinite-you/pkg/transports/http/client"
)

// NewMCPAdapter binds one generated client to the selected host at construction.
func NewMCPAdapter(serverURL string, doer client.HttpRequestDoer) (*workersessionmcp.Adapter, error) {
	host, err := client.NewClient(serverURL, client.WithHTTPClient(doer))
	if err != nil {
		return nil, err
	}
	return workersessionmcp.New(host)
}
