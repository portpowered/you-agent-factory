package service

import (
	"context"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// DirectCapacityResolver is the fixed saved-settings role supplied by Wire.
type DirectCapacityResolver func(context.Context) (int64, error)

// Freeze the saved bound once for this host. No registry lock spans document IO.
func (r *registry) resolveDirectCapacity(ctx context.Context) error {
	r.capacityOnce.Do(func() {
		r.directCapacity = operatorsettings.DefaultDirectSessionCapacity
		if r.capacityResolver != nil {
			r.directCapacity, r.capacityErr = r.capacityResolver(ctx)
		}
		if r.capacityErr == nil {
			_, r.capacityErr = (operatorsettings.Defaults{DirectSessionCapacity: &r.directCapacity}).ResolveDirectSessionCapacity()
		}
	})
	return r.capacityErr
}

// The caller holds r.mu. Admission and charge acquisition are one atomic operation.
func (r *registry) chargeDirectLocked(id string) error {
	capacity := r.directCapacity
	if capacity == 0 {
		capacity = operatorsettings.DefaultDirectSessionCapacity
	}
	if int64(len(r.directCharges)) >= capacity {
		host := (workersessions.DirectCapacityFailure{Host: r.executionEndpointLocked()}).PublicHost()
		r.logger.Info("worker session direct capacity", "outcome", "refused", "host", host, "capacity", capacity)
		return workersessions.DirectCapacityFailure{Host: host, Capacity: capacity}
	}
	if r.directCharges == nil {
		r.directCharges = make(map[string]struct{})
	}
	r.directCharges[id] = struct{}{}
	r.logger.Info("worker session direct capacity", "outcome", "charged", "sessionID", publicWorkerID(id), "capacity", capacity)
	return nil
}

func (r *registry) releaseDirect(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, owned := r.directCharges[id]; !owned {
		return
	}
	delete(r.directCharges, id)
	r.logger.Info("worker session direct capacity", "outcome", "released", "sessionID", publicWorkerID(id), "capacity", r.directCapacity)
}
