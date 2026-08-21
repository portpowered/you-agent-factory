package recordingreplay

import (
	"context"
	"errors"

	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
)

// restorableStateProbe is implemented by the live execution owner. It is a
// read-only eligibility check: the owner may load and validate durable state,
// but it must not schedule work, invoke a Worker, or mutate the session.
type restorableStateProbe interface {
	HasRestorableState(context.Context, string) (bool, error)
}

func (s *Service) handedOffOwner() (fse.Service, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.live, s.handedOff && s.live != nil
}

func (s *Service) handedOffOwnerForSession() (fse.Service, bool) {
	owner, handedOff := s.handedOffOwner()
	if !handedOff || s.session(s.projection.Session.SessionID) != nil {
		return nil, false
	}
	return owner, true
}

func (s *Service) handedOffOwnerForSessionOperation(sessionID string) (fse.Service, error) {
	if err := s.session(sessionID); err != nil {
		return nil, err
	}
	owner, handedOff := s.handedOffOwnerForSession()
	if !handedOff {
		return nil, ErrNonLiveReplay
	}
	return owner, nil
}

func (s *Service) resumeOwnerLocked(ctx context.Context, sessionID string) (fse.Service, error) {
	if err := s.session(sessionID); err != nil {
		return nil, err
	}
	if owner, handedOff := s.handedOffOwner(); handedOff {
		return owner, nil
	}
	if s == nil || s.live == nil {
		return nil, ErrNonLiveReplay
	}
	probe, ok := s.live.(restorableStateProbe)
	if !ok {
		return nil, ErrNonLiveReplay
	}
	available, err := probe.HasRestorableState(ctx, sessionID)
	if errors.Is(err, fse.ErrSessionNotFound) {
		return nil, ErrNonLiveReplay
	}
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, ErrNonLiveReplay
	}
	return s.live, nil
}

func (s *Service) markHandedOff() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.handedOff = s.live != nil
	s.mu.Unlock()
}
