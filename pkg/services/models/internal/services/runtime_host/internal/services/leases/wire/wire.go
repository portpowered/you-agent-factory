// Package wire constructs the parent-private Models Runtime Host leases owner.
package wire

import (
	"fmt"
	"reflect"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	hostleases "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases/internal/service"
)

// NewService constructs an inert leases owner over injected host effects.
// Construction validates required dependencies and allocates lease/capacity
// state only; it does not launch subprocesses or start application lifecycle.
func NewService(
	hostClock modelseffects.HostClock,
	slotFacts modelseffects.SlotFactsProvider,
) (hostleases.Service, error) {
	if isNilDependency(hostClock) {
		return nil, fmt.Errorf("%w: model host clock is required", models.ErrInvalidHostDependencies)
	}
	if isNilDependency(slotFacts) {
		return nil, fmt.Errorf("%w: model host slot facts provider is required", models.ErrInvalidHostDependencies)
	}
	return internalservice.New(hostClock, slotFacts), nil
}

// BindCoordinator attaches holder-aware Runtime Host cleanup after both
// private services have been constructed.
func BindCoordinator(leases hostleases.Service, coordinator modelseffects.SlotCapacityCoordinator) {
	if bindable, ok := leases.(modelseffects.CoordinatorBindable); ok {
		bindable.BindSlotCapacityCoordinator(coordinator)
	}
}

func isNilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
