package service

import (
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
