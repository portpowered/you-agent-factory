package initializer

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
)

// LocalRuntimeRunner is the session/runtime seam used by local in-process CLI
// startup without coupling transports to root pkg/service.FactoryService.
type LocalRuntimeRunner interface {
	Run(ctx context.Context) error
}

// SessionRuntimeHost is the transport-facing session/runtime shell composed from
// a Core without exposing root FactoryService at transport boundaries.
type SessionRuntimeHost struct {
	host *runtimehost.Host
}

// NewSessionRuntimeHostFromCore composes the API/CLI session runtime host from a
// built Core and attaches factory-save collaborators from cfg.
func NewSessionRuntimeHostFromCore(core *Core, cfg *Config) *SessionRuntimeHost {
	if core == nil {
		return nil
	}
	shell := runtimehost.HostShell{Host: runtimehost.NewHostFromCore(core)}
	host := runtimehost.AttachFactorySaveCollaborator(
		shell,
		runtimehost.ProvideFactorySaveCollaborator(shell, cfg),
	)
	return &SessionRuntimeHost{host: host}
}

// SessionAPISurface returns handler dependencies for api.NewServer.
func (h *SessionRuntimeHost) SessionAPISurface() apisurface.SessionAPISurface {
	if h == nil || h.host == nil {
		return nil
	}
	return h.host
}

// SessionAPI returns the session lifecycle and session-scoped work collaborator.
func (h *SessionRuntimeHost) SessionAPI() apisurface.SessionAPI {
	if h == nil || h.host == nil {
		return nil
	}
	return h.host
}

// FactoryDefinitionAPI returns the factory-definition read/write collaborator.
func (h *SessionRuntimeHost) FactoryDefinitionAPI() apisurface.FactorySaveAPI {
	if h == nil || h.host == nil {
		return nil
	}
	return h.host
}

// InvocationAPI returns the session invocation collaborator.
func (h *SessionRuntimeHost) InvocationAPI() apisurface.InvocationAPI {
	if h == nil || h.host == nil {
		return nil
	}
	return h.host.InvocationAPI()
}

// DurableExecutionAPI returns the durable execution collaborator.
func (h *SessionRuntimeHost) DurableExecutionAPI() apisurface.DurableSessionAPI {
	if h == nil || h.host == nil {
		return nil
	}
	return h.host.DurableExecutionAPI()
}

// Run starts service-mode sidecars and the default session runtime loop.
func (h *SessionRuntimeHost) Run(ctx context.Context) error {
	if h == nil || h.host == nil {
		return nil
	}
	return h.host.Run(ctx)
}

// RunWithAPISurface starts lifecycle-owned runtime work while providing the
// independently composed handler surface to API startup.
func (h *SessionRuntimeHost) RunWithAPISurface(
	ctx context.Context,
	surface apisurface.SessionAPISurface,
) error {
	if h == nil || h.host == nil {
		return nil
	}
	return h.host.RunWithAPISurface(ctx, surface)
}

// LocalRuntimeRunner returns the local in-process CLI runtime seam implemented
// by this host without exposing the root FactoryService compatibility shell.
func (h *SessionRuntimeHost) LocalRuntimeRunner() LocalRuntimeRunner {
	if h == nil || h.host == nil {
		return nil
	}
	return h.host
}

// CompatibilityServiceShell exposes the temporary runtime host shell for legacy
// harness callbacks. New transport code must not depend on this method.
func (h *SessionRuntimeHost) CompatibilityServiceShell() *runtimehost.Host {
	if h == nil {
		return nil
	}
	return h.host
}

// RuntimeHost returns the authoritative runtime/session host owned by pkg/runtimehost.
func (h *SessionRuntimeHost) RuntimeHost() *runtimehost.Host {
	if h == nil {
		return nil
	}
	return h.host
}
