package runtimebinding

import (
	"context"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// ActiveRuntime is Factory Session's selection of one running runtime handle.
type ActiveRuntime struct {
	Context   context.Context
	SessionID string
	Handle    factory.HostedHandle
}

// State owns startup and active runtime selection without depending on a
// concrete Factory Runtime host.
type State struct {
	startupMu sync.RWMutex
	startup   factory.HostedInstance
	activeMu  sync.RWMutex
	active    *ActiveRuntime
}

func (s *State) Startup() factory.HostedInstance {
	if s == nil {
		return nil
	}
	s.startupMu.RLock()
	defer s.startupMu.RUnlock()
	return s.startup
}

func (s *State) SetStartup(instance factory.HostedInstance) {
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

func (s *State) ActiveHandle() factory.HostedHandle {
	active := s.Active()
	if active == nil {
		return nil
	}
	return active.Handle
}

func (s *State) SetActive(ctx context.Context, sessionID string, handle factory.HostedHandle) {
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

func (s *State) Current(defaultInstance func() factory.HostedInstance) factory.HostedInstance {
	if handle := s.ActiveHandle(); handle != nil {
		if instance := handle.RuntimeInstance(); instance != nil {
			return instance
		}
	}
	if defaultInstance != nil {
		if instance := defaultInstance(); instance != nil {
			return instance
		}
	}
	return s.Startup()
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
	Instance factory.HostedInstance
	Handle   factory.HostedHandle
	Spec     any
}
