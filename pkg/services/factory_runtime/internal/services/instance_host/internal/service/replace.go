package service

import (
	"context"
	"fmt"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/service/host"
)

func (h *Host) Replace(req instancehost.ReplaceRequest) (factoryruntime.HostedHandle, error) {
	current, err := h.requireControllableHandle(req.Current)
	if err != nil {
		return nil, err
	}
	replacementBundle, err := bundleFromInstance(req.Replacement)
	if err != nil {
		return nil, err
	}
	serviceCtx := req.ServiceContext
	if serviceCtx == nil {
		serviceCtx = context.Background()
	}
	readinessCtx := req.ReadinessContext
	if readinessCtx == nil {
		readinessCtx = context.Background()
	}

	attempt := &factoryhost.ReplacementAttempt{
		Current:         current,
		ServiceCtx:      serviceCtx,
		ServiceMode:     req.AttachSidecarsInServiceMode,
		RestoreSidecars: adaptSidecarStarter(req.AttachSidecars),
	}
	attempt.Begin()
	committed := false
	defer func() {
		if !committed {
			attempt.End()
		}
	}()

	replacementHandle, err := factoryhost.StartReplacement(
		readinessCtx,
		serviceCtx,
		replacementBundle,
		h.clock,
		adaptSidecarStarter(req.AttachSidecars),
		req.AttachSidecarsInServiceMode,
	)
	if err != nil {
		return nil, err
	}

	if err := factoryhost.PublishFactoryChange(readinessCtx, current, replacementBundle, h.clock); err != nil {
		_ = h.lifecycle.Stop(replacementHandle)
		return nil, fmt.Errorf("publish replacement factory change: %w", err)
	}

	if err := h.commitActiveHandle(current, replacementHandle); err != nil {
		_ = h.lifecycle.Stop(replacementHandle)
		return nil, err
	}

	attempt.Commit()
	committed = true
	return replacementHandle, nil
}

func adaptSidecarStarter(
	starter func(context.Context, factoryruntime.HostedHandle) error,
) factoryhost.SidecarStarter {
	if starter == nil {
		return nil
	}
	return func(ctx context.Context, handle *factoryhost.Handle) error {
		return starter(ctx, handle)
	}
}

func (h *Host) commitActiveHandle(current, replacement *factoryhost.Handle) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if current != nil && current.Bundle != nil {
		instanceID := current.Bundle.RuntimeInstanceID
		if instanceID != "" {
			registered, ok := h.handles[instanceID]
			if !ok || registered != current {
				return factoryruntime.ErrNotRunning
			}
			delete(h.handles, instanceID)
		}
	}
	if replacement != nil && replacement.Bundle != nil {
		if instanceID := replacement.Bundle.RuntimeInstanceID; instanceID != "" {
			h.handles[instanceID] = replacement
		}
	}
	return nil
}
