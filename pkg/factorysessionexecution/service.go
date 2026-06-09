package factorysessionexecution

import "context"

// Service is the shared durable factory-session execution start contract consumed
// by API, CLI, MCP, and UI transports. Live-session open and invocation remain
// on the separate factorysessions compatibility surface.
type Service interface {
	StartAsync(ctx context.Context, req StartRequest) (AsyncStartResult, error)
	StartSync(ctx context.Context, req StartRequest) (SyncStartResult, error)
}
