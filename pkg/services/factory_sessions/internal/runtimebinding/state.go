package runtimebinding

import (
	"context"
	"sync"
)

// ActiveRuntime is Factory Session's selection of one running runtime handle.
type ActiveRuntime struct {
	Context   context.Context
	SessionID string
	Handle    RuntimeHandle
}

// State owns startup and active runtime selection without depending on a
// concrete Factory Runtime host.
type State struct {
	startupMu sync.RWMutex
	startup   RuntimeInstance
	activeMu  sync.RWMutex
	active    *ActiveRuntime
}

func (s *State) Startup() RuntimeInstance {
	if s == nil {
		return nil
	}
	s.startupMu.RLock()
	defer s.startupMu.RUnlock()
	return s.startup
}

func (s *State) SetStartup(instance RuntimeInstance) {
	if s == nil {
		return
	}
	s.startupMu.Lock()
	s.startup = instance
	s.startupMu.Unlock()
}

func (s *State) ClearStartup() { s.SetStartup(nil) }

func (s *State) Active() *ActiveRuntime {
	if s == nil {
		return nil
	}
	s.activeMu.RLock()
	defer s.activeMu.RUnlock()
	if s.active == nil {
		return nil
	}
	copy := *s.active
	return &copy
}

func (s *State) ActiveHandle() RuntimeHandle {
	active := s.Active()
	if active == nil {
		return nil
	}
	return active.Handle
}

func (s *State) SetActive(ctx context.Context, sessionID string, handle RuntimeHandle) {
	if s == nil {
		return
	}
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	if ctx == nil {
		s.active = nil
		return
	}
	s.active = &ActiveRuntime{Context: ctx, SessionID: sessionID, Handle: handle}
}

func (s *State) ClearActive() {
	if s == nil {
		return
	}
	s.activeMu.Lock()
	s.active = nil
	s.activeMu.Unlock()
}

func (s *State) Current(defaultInstance func() RuntimeInstance) RuntimeInstance {
	if handle := s.ActiveHandle(); handle != nil {
		if instance := handle.RuntimeInstance(); instance != nil {
			return instance
		}
	}
	// A newly opened SessionRuntime owns its startup instance. The process-wide
	// default may belong to another concurrent invocation and must not replace
	// that invocation-local startup authority before StartInitial registers it.
	if instance := s.Startup(); instance != nil {
		return instance
	}
	if defaultInstance != nil {
		if instance := defaultInstance(); instance != nil {
			return instance
		}
	}
	return nil
}

func runtimeContext(fallback context.Context, active *ActiveRuntime) context.Context {
	if active != nil && active.Context != nil {
		return active.Context
	}
	return fallback
}

// SessionState is the opaque Factory Runtime payload retained by a live
// Factory Session.
type SessionState struct {
	Instance RuntimeInstance
	Handle   RuntimeHandle
	Spec     any
}
