package service

import (
	"context"
	"errors"
	"fmt"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
)

func (h *Host) Start(
	ctx context.Context,
	instance factoryruntime.HostedInstance,
) (factoryruntime.HostedHandle, error) {
	bundle, err := bundleFromInstance(instance)
	if err != nil {
		return nil, err
	}
	instanceID := bundle.RuntimeInstanceID
	if instanceID == "" {
		return nil, fmt.Errorf("factory runtime host requires a built runtime instance")
	}

	h.mu.Lock()
	if existing, ok := h.handles[instanceID]; ok && !existing.Completed() {
		h.mu.Unlock()
		return nil, fmt.Errorf(
			"factory runtime instance %s already has an active hosted handle",
			instanceID,
		)
	}
	h.mu.Unlock()

	handle := factoryhost.Start(ctx, bundle)
	if handle == nil {
		return nil, fmt.Errorf("factory runtime host requires a built runtime instance")
	}
	if handle.Completed() {
		if runErr := handle.Result(); runErr != nil {
			return nil, runErr
		}
	}

	h.mu.Lock()
	if existing, ok := h.handles[instanceID]; ok && !existing.Completed() {
		h.mu.Unlock()
		if !handle.Completed() {
			_ = h.lifecycle.Stop(handle)
		}
		return nil, fmt.Errorf(
			"factory runtime instance %s already has an active hosted handle",
			instanceID,
		)
	}
	h.handles[instanceID] = handle
	h.mu.Unlock()
	return handle, nil
}

func (h *Host) WaitForStart(ctx context.Context, handle factoryruntime.HostedHandle) error {
	concrete, ok := handle.(*factoryhost.Handle)
	if !ok || concrete == nil {
		return fmt.Errorf("factory runtime host requires a runtime handle")
	}
	if err := factoryhost.WaitForStart(ctx, concrete); err != nil {
		h.removeHandle(concrete)
		stopErr := h.lifecycle.Stop(concrete)
		return errors.Join(err, stopErr)
	}
	return nil
}

func bundleFromInstance(instance factoryruntime.HostedInstance) (*factoryhost.Bundle, error) {
	if instance == nil {
		return nil, fmt.Errorf("factory runtime host requires a built runtime instance")
	}
	bundle, ok := instance.(*factoryhost.Bundle)
	if !ok || bundle == nil {
		return nil, fmt.Errorf("factory runtime host requires a built runtime instance")
	}
	return bundle, nil
}

func (h *Host) removeHandle(handle *factoryhost.Handle) {
	_ = h.clearRegisteredHandle(handle)
}

func (h *Host) clearRegisteredHandle(handle *factoryhost.Handle) bool {
	if handle == nil || handle.Bundle == nil {
		return false
	}
	instanceID := handle.Bundle.RuntimeInstanceID
	if instanceID == "" {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	registered, ok := h.handles[instanceID]
	if !ok || registered != handle {
		return false
	}
	delete(h.handles, instanceID)
	return true
}
