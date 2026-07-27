package service

import (
	models "github.com/portpowered/infinite-you/pkg/services/models"
	hostleases "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases"
)

// ActiveCapacityForTest reports the nested leases owner capacity holder count.
func ActiveCapacityForTest(
	leases hostleases.Service,
	scope models.RuntimeScopeRef,
	modelName string,
) int {
	svc, ok := leases.(*service)
	if !ok {
		return -1
	}
	slotKey := leaseSlotKey(scope, modelName)
	svc.mu.Lock()
	defer svc.mu.Unlock()
	return svc.capacityHolders[slotKey]
}
