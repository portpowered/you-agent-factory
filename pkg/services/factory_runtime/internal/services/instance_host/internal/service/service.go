// Package service implements the parent-private Factory Runtime instance host.
package service

import (
	"context"
	"fmt"
	"sync"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/service/host"
)

// Host owns hosted-instance handle capacity and lifecycle delegation for one
// Factory Runtime parent.
type Host struct {
	clock     factoryruntime.Clock
	lifecycle *factoryhost.LifecycleService

	mu      sync.Mutex
	handles map[string]*factoryhost.Handle
}

var _ instancehost.Service = (*Host)(nil)

// New constructs an inert instance host that allocates handle state without
// starting a hosted run loop, attaching sidecars, publishing a replacement, or
// finalizing artifacts.
func New(dependencies instancehost.Dependencies) (instancehost.Service, error) {
	if dependencies.Clock == nil {
		return nil, fmt.Errorf("%w: clock is required", instancehost.ErrInvalidDependencies)
	}
	lifecycle, err := factoryhost.NewLifecycleService(dependencies.Clock)
	if err != nil {
		return nil, err
	}
	return &Host{
		clock:     dependencies.Clock,
		lifecycle: lifecycle,
		handles:   make(map[string]*factoryhost.Handle),
	}, nil
}

func (h *Host) Stop(handle factoryruntime.HostedHandle) error {
	concrete, ok := handle.(*factoryhost.Handle)
	if !ok || concrete == nil {
		return h.lifecycle.Stop(handle)
	}
	err := h.lifecycle.Stop(concrete)
	h.removeHandle(concrete)
	return err
}

func (h *Host) StopSidecars(handle factoryruntime.HostedHandle) {
	h.lifecycle.StopSidecars(handle)
}

func (h *Host) PublishReplacement(
	ctx context.Context,
	current factoryruntime.HostedHandle,
	replacement factoryruntime.HostedInstance,
) error {
	return h.lifecycle.PublishReplacement(ctx, current, replacement)
}
