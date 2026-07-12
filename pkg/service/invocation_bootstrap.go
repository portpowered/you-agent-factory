package service

import (
	"context"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// InvocationBootstrap constructs and runs one-shot factory invocation
// session/runtime dependencies without binding a listening HTTP server.
type InvocationBootstrap struct {
	Service *FactoryService
}

// NormalizeInvocationBootstrapConfig returns a copy of cfg shaped for in-process
// one-shot invocation: service-mode runtime, no dashboard renderer, no work-file
// seeding, and no API/dashboard TCP listener.
func NormalizeInvocationBootstrapConfig(cfg *FactoryServiceConfig) *FactoryServiceConfig {
	if cfg == nil {
		return nil
	}
	normalized := *cfg
	normalized.Port = 0
	normalized.APIServerStarter = nil
	normalized.APIServerReady = nil
	normalized.SimpleDashboardRenderer = nil
	normalized.RuntimeMode = interfaces.RuntimeModeService
	normalized.WorkFile = ""
	return &normalized
}

// BuildInvocationBootstrap constructs FactoryService-owned session/runtime
// dependencies for one-shot invocation without starting a listening HTTP server.
func BuildInvocationBootstrap(ctx context.Context, cfg *FactoryServiceConfig) (*InvocationBootstrap, error) {
	normalized := NormalizeInvocationBootstrapConfig(cfg)
	if normalized == nil {
		return nil, fmt.Errorf("build invocation bootstrap: config is required")
	}
	service, err := BuildFactoryService(ctx, normalized)
	if err != nil {
		return nil, err
	}
	return &InvocationBootstrap{Service: service}, nil
}

// Run starts the bootstrap-owned factory session runtime loop.
func (b *InvocationBootstrap) Run(ctx context.Context) error {
	if b == nil || b.Service == nil {
		return fmt.Errorf("invocation bootstrap is required")
	}
	return b.Service.Run(ctx)
}

// GetCurrentFactoryForSession exposes the active factory definition for a live
// bootstrap-owned session.
func (b *InvocationBootstrap) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	if b == nil || b.Service == nil {
		return factoryapi.Factory{}, fmt.Errorf("invocation bootstrap is required")
	}
	return b.Service.GetCurrentFactoryForSession(ctx, sessionID)
}

// InvokeFactorySession forwards one-shot invocation through the bootstrap-owned
// FactoryService session invoker.
func (b *InvocationBootstrap) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.InvocationRequest,
) (apisurface.FactoryInvocationResult, error) {
	if b == nil || b.Service == nil {
		return apisurface.FactoryInvocationResult{}, fmt.Errorf("invocation bootstrap is required")
	}
	return b.Service.InvokeFactorySession(ctx, sessionID, request)
}
