// Package wire constructs the parent-private Factory Runtime instance host.
package wire

import (
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/internal/service"
)

// New constructs the inert instance host composed by the Factory Runtime root.
func New(clock factoryruntime.Clock, scheduler platformclock.TimerSource, lifecycle *factoryhost.LifecycleService) (instancehost.Service, error) {
	return internalservice.New(clock, scheduler, lifecycle)
}
