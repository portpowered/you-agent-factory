// Package service implements the parent-private Factory Runtime instance host.
package service

import (
	"context"
	"fmt"
	"sync"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
)

// Host owns hosted-instance handle capacity and lifecycle delegation for one
// Factory Runtime parent.
type Host struct {
	clock     factoryruntime.Clock
	scheduler platformclock.TimerSource
	lifecycle *factoryhost.LifecycleService

	mu      *sync.Mutex
	handles map[string]*factoryhost.Handle
}

var _ instancehost.Service = (*Host)(nil)

// New constructs an inert instance host that allocates handle state without
// starting a hosted run loop, attaching sidecars, publishing a replacement, or
// finalizing artifacts.
func New(clock factoryruntime.Clock, scheduler platformclock.TimerSource, lifecycle *factoryhost.LifecycleService) (instancehost.Service, error) {
	if clock == nil {
		return nil, fmt.Errorf("%w: clock is required", instancehost.ErrInvalidDependencies)
	}
	if scheduler == nil {
		return nil, fmt.Errorf("%w: scheduler is required", instancehost.ErrInvalidDependencies)
	}
	if lifecycle == nil {
		return nil, fmt.Errorf("%w: lifecycle is required", instancehost.ErrInvalidDependencies)
	}
	return &Host{clock: clock, scheduler: scheduler, lifecycle: lifecycle, mu: &sync.Mutex{}, handles: make(map[string]*factoryhost.Handle)}, nil
}

// Scope preserves the invocation's fact time over the same handle registry and
// lock. This compatibility view retires with the keyed activation migration.
func (h *Host) Scope(clock factoryruntime.Clock) instancehost.Service {
	return &Host{clock: clock, scheduler: h.scheduler, lifecycle: h.lifecycle, mu: h.mu, handles: h.handles}
}

func (h *Host) StopSidecars(handle factoryruntime.RuntimeRun) {
	h.lifecycle.StopSidecars(handle)
}

func (h *Host) PublishReplacement(
	ctx context.Context,
	current factoryruntime.RuntimeRun,
	replacement factoryruntime.RuntimeRecord,
) error {
	return h.lifecycle.PublishReplacementWithClock(ctx, current, replacement, h.clock)
}
