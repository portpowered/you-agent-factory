package service

import (
	"context"
	"fmt"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
)

func (h *Host) Pause(
	ctx context.Context,
	handle factoryruntime.HostedHandle,
) (factoryruntime.PauseResult, error) {
	concrete, err := h.requireControllableHandle(handle)
	if err != nil {
		return factoryruntime.PauseResult{}, err
	}
	service := runtimeServiceFromHandle(concrete)
	if service == nil {
		return factoryruntime.PauseResult{}, factoryruntime.ErrNotRunning
	}
	return service.ControlPause(ctx, factoryruntime.PauseRequest{})
}

func (h *Host) Resume(
	ctx context.Context,
	handle factoryruntime.HostedHandle,
) (factoryruntime.ResumeResult, error) {
	concrete, err := h.requireControllableHandle(handle)
	if err != nil {
		return factoryruntime.ResumeResult{}, err
	}
	service := runtimeServiceFromHandle(concrete)
	if service == nil {
		return factoryruntime.ResumeResult{}, factoryruntime.ErrNotRunning
	}
	return service.ControlResume(ctx, factoryruntime.ResumeRequest{})
}

func (h *Host) requireControllableHandle(handle factoryruntime.HostedHandle) (*factoryhost.Handle, error) {
	concrete, ok := handle.(*factoryhost.Handle)
	if !ok || concrete == nil {
		return nil, fmt.Errorf("factory runtime host requires a runtime handle")
	}
	if concrete.Completed() {
		return nil, factoryruntime.ErrNotRunning
	}
	if concrete.Bundle == nil || concrete.Bundle.RuntimeInstanceID == "" {
		return nil, factoryruntime.ErrNotRunning
	}
	h.mu.Lock()
	registered, ok := h.handles[concrete.Bundle.RuntimeInstanceID]
	h.mu.Unlock()
	if !ok || registered != concrete {
		return nil, factoryruntime.ErrNotRunning
	}
	return concrete, nil
}

func runtimeServiceFromHandle(handle *factoryhost.Handle) factoryruntime.Service {
	if handle == nil || handle.Bundle == nil {
		return nil
	}
	return handle.Bundle.RuntimeService()
}
