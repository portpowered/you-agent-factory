package service

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/apisurface"
)

// SessionRuntimeHost is the transport-facing session/runtime shell composed from
// a FactoryCore without exposing root FactoryService at initializer boundaries.
type SessionRuntimeHost FactoryService

// NewSessionRuntimeHostFromCore composes the API/CLI session runtime host from a
// built FactoryCore and attaches factory-save collaborators from cfg.
func NewSessionRuntimeHostFromCore(core *FactoryCore, cfg *FactoryServiceConfig) *SessionRuntimeHost {
	if core == nil {
		return nil
	}
	shell := FactoryServiceShell{Service: NewFactoryServiceFromCore(core)}
	service := AttachFactorySaveCollaborator(shell, ProvideFactorySaveCollaborator(shell, cfg))
	return (*SessionRuntimeHost)(service)
}

// SessionAPISurface returns the host as the API handler dependency seam.
func (h *SessionRuntimeHost) SessionAPISurface() apisurface.SessionAPISurface {
	if h == nil {
		return nil
	}
	return (*FactoryService)(h)
}

// Run starts service-mode sidecars and the default session runtime loop.
func (h *SessionRuntimeHost) Run(ctx context.Context) error {
	if h == nil {
		return nil
	}
	return (*FactoryService)(h).Run(ctx)
}

// FactoryService returns the compatibility shell view of this host.
func (h *SessionRuntimeHost) FactoryService() *FactoryService {
	if h == nil {
		return nil
	}
	return (*FactoryService)(h)
}
