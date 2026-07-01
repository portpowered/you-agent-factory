package initializer

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/service"
)

// SessionRuntimeHost is the transport-facing session/runtime shell composed from
// a Core without exposing root FactoryService at transport boundaries.
type SessionRuntimeHost struct {
	shell *service.FactoryService
}

// NewSessionRuntimeHostFromCore composes the API/CLI session runtime host from a
// built Core and attaches factory-save collaborators from cfg.
func NewSessionRuntimeHostFromCore(core *Core, cfg *Config) *SessionRuntimeHost {
	if core == nil {
		return nil
	}
	shell := service.FactoryServiceShell{Service: service.NewFactoryServiceFromCore(core)}
	serviceShell := service.AttachFactorySaveCollaborator(shell, service.ProvideFactorySaveCollaborator(shell, cfg))
	return &SessionRuntimeHost{shell: serviceShell}
}

// SessionAPISurface returns handler dependencies for api.NewServer.
func (h *SessionRuntimeHost) SessionAPISurface() apisurface.SessionAPISurface {
	if h == nil || h.shell == nil {
		return nil
	}
	return h.shell
}

// Run starts service-mode sidecars and the default session runtime loop.
func (h *SessionRuntimeHost) Run(ctx context.Context) error {
	if h == nil || h.shell == nil {
		return nil
	}
	return h.shell.Run(ctx)
}

// LocalRuntimeRunner returns the local in-process CLI runtime seam implemented
// by this host without exposing the root FactoryService compatibility shell.
func (h *SessionRuntimeHost) LocalRuntimeRunner() LocalRuntimeRunner {
	if h == nil || h.shell == nil {
		return nil
	}
	return h.shell
}

// CompatibilityServiceShell exposes the temporary FactoryService shell for
// legacy harness callbacks. New transport code must not depend on this method.
func (h *SessionRuntimeHost) CompatibilityServiceShell() *service.FactoryService {
	if h == nil {
		return nil
	}
	return h.shell
}
