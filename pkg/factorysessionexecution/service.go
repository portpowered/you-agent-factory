package factorysessionexecution

import "context"

// Service is the shared durable factory-session execution contract consumed by
// API, CLI, MCP, and UI transports. Live-session open and invocation remain on
// the separate factorysessions compatibility surface. All methods are
// cancellation-aware; transports must not mutate runtime state directly.
type Service interface {
	StartAsync(ctx context.Context, req StartRequest) (AsyncStartResult, error)
	StartSync(ctx context.Context, req StartRequest) (SyncStartResult, error)
	GetSession(ctx context.Context, sessionID string) (SessionReadResult, error)
	Pause(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error)
	Resume(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error)
	Cancel(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error)
	Terminate(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error)
	Approve(ctx context.Context, sessionID string, req ApproveRequest) (LifecycleControlResult, error)
	RetryDispatch(ctx context.Context, sessionID string, req RetryDispatchRequest) (LifecycleControlResult, error)
}
