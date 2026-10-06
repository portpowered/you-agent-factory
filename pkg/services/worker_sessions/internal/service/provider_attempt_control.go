package service

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// providerAttemptControl belongs to one physical execution generation. All
// access uses its supervision's mutex. A retry or continuation installs a new
// slot even if an attempt identity is reused; late observers cannot attach to
// that replacement. Neither the capability nor the slot is durable state.
type providerAttemptControl struct {
	control providers.AttemptControl
	retired bool
}

// Binding belongs to the admitted handle, not a lookup by reusable dispatch
// identity. Installation runs outside ownership locks. If a caller unwinds,
// close its admitted observation window before propagating the panic.
func (a *runtimeAttempt) bindProviderAttemptControl(ctx context.Context, bind func(providers.AttemptControlObserver), next providers.AttemptControlObserver) {
	if bind == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = a.Complete(context.WithoutCancel(ctx), workers.WorkstationDispatchResult{}, errors.New("runtime attempt control observer installation failed"))
			panic(recovered)
		}
	}()
	bind(func(control providers.AttemptControl) {
		if a.observeProviderAttemptControl(control) && next != nil {
			next(control)
		}
	})
}

func (a *runtimeAttempt) observeProviderAttemptControl(control providers.AttemptControl) bool {
	if control == nil {
		return false
	}
	r := a.registry
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.runtimeAttemptControls[a.workerID] != a || r.runtimeAttemptOwners[a.key] != a.workerID ||
		r.latestRuntimeDispatchIDs[a.workerID] != a.dispatchID {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.providerControlRetired || a.completing || a.providerControl != nil {
		return false
	}
	a.providerControl = control
	return true
}

func (a *runtimeAttempt) retireProviderAttemptControl() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.providerControlRetired = true
	a.providerControl = nil
}

// bindProviderAttemptControl captures the slot before Workers enters Providers.
// The caller must retire it on every return, including panic unwinding. An
// external observer runs outside the supervision lock and only after the
// exact live generation has retained its first nonnil capability.
func (s *supervision) bindProviderAttemptControl(request *workers.ExecuteRequest) func() {
	s.mu.Lock()
	if s.providerAttempt == nil {
		s.providerAttempt = &providerAttemptControl{}
	}
	slot := s.providerAttempt
	dispatchID := s.dispatchID
	s.mu.Unlock()

	next := request.Input.AttemptControlObserver
	request.Input.AttemptControlObserver = func(control providers.AttemptControl) {
		if s.attachProviderAttemptControl(slot, dispatchID, control) && next != nil {
			next(control)
		}
	}
	return func() {
		s.mu.Lock()
		slot.retired = true
		slot.control = nil
		s.mu.Unlock()
	}
}

func (s *supervision) attachProviderAttemptControl(slot *providerAttemptControl, dispatchID string, control providers.AttemptControl) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if control == nil || s.providerAttempt != slot || s.dispatchID != dispatchID || slot.retired || slot.control != nil {
		return false
	}
	slot.control = control
	return true
}
