package service

import (
	"fmt"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/service/host"
)

func (h *Host) Stop(handle factoryruntime.HostedHandle) error {
	concrete, err := h.classifyStopHandle(handle)
	if err != nil {
		return err
	}
	stopErr := h.lifecycle.Stop(concrete)
	h.removeHandle(concrete)
	return stopErr
}

func (h *Host) classifyStopHandle(handle factoryruntime.HostedHandle) (*factoryhost.Handle, error) {
	if handle == nil {
		return nil, factoryruntime.ErrNotRunning
	}
	concrete, ok := handle.(*factoryhost.Handle)
	if !ok || concrete == nil {
		return nil, fmt.Errorf("factory runtime host requires a runtime handle")
	}
	if concrete.Bundle == nil || concrete.Bundle.RuntimeInstanceID == "" {
		if concrete.Completed() {
			return nil, factoryruntime.ErrAlreadyStopped
		}
		return nil, factoryruntime.ErrNotRunning
	}

	instanceID := concrete.Bundle.RuntimeInstanceID
	h.mu.Lock()
	registered, inRegistry := h.handles[instanceID]
	h.mu.Unlock()

	if !inRegistry || registered != concrete {
		if concrete.Completed() {
			return nil, factoryruntime.ErrAlreadyStopped
		}
		return nil, factoryruntime.ErrNotRunning
	}
	return concrete, nil
}
