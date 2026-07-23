// Package sessionregistry owns the in-memory live Factory Session directory.
package sessionregistry

import (
	"sort"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
)

// Service is the private live Factory Session directory contract shared by
// owner-local runtime and identity capabilities.
type Service interface {
	Upsert(*livesession.LiveSession, bool)
	Select(string) bool
	Current() *livesession.LiveSession
	Get(string) *livesession.LiveSession
	Remove(string)
	Count() int
	IDs() []string
	DefaultSession() *livesession.LiveSession
	FindByLogicalSessionKeyID(string) *livesession.LiveSession
}

// Registry is the synchronized in-memory implementation of the Factory
// Sessions registry contract.
type Registry struct {
	mu         sync.RWMutex
	selectedID string
	sessions   map[string]*livesession.LiveSession
}

var _ Service = (*Registry)(nil)

// New constructs an empty live session registry.
func New() *Registry {
	return &Registry{sessions: make(map[string]*livesession.LiveSession)}
}

func (r *Registry) Upsert(session *livesession.LiveSession, selectSession bool) {
	if r == nil || session == nil || session.ID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[session.ID] = session
	if selectSession || r.selectedID == "" {
		r.selectedID = session.ID
	}
}

func (r *Registry) Select(id string) bool {
	if r == nil || id == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[id]; !ok {
		return false
	}
	r.selectedID = id
	return true
}

func (r *Registry) Current() *livesession.LiveSession {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sessions[r.selectedID]
}

func (r *Registry) Get(id string) *livesession.LiveSession {
	if r == nil {
		return nil
	}
	if logicaltarget.IsLiveSessionDefaultSelector(id) {
		return r.DefaultSession()
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sessions[id]
}

func (r *Registry) Remove(id string) {
	if r == nil || id == "" {
		return
	}
	if logicaltarget.IsLiveSessionDefaultSelector(id) {
		if session := r.DefaultSession(); session != nil {
			id = session.ID
		} else {
			return
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
	if r.selectedID != id {
		return
	}
	r.selectedID = ""
	if len(r.sessions) == 0 {
		return
	}
	ids := make([]string, 0, len(r.sessions))
	for sessionID := range r.sessions {
		ids = append(ids, sessionID)
	}
	sort.Strings(ids)
	r.selectedID = ids[0]
}

func (r *Registry) Count() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.sessions)
}

func (r *Registry) IDs() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.sessions))
	for id := range r.sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (r *Registry) DefaultSession() *livesession.LiveSession {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.sessions))
	for id, session := range r.sessions {
		if session != nil && session.IsDefault {
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 {
		sort.Strings(ids)
		return r.sessions[ids[0]]
	}
	return nil
}

func (r *Registry) FindByLogicalSessionKeyID(logicalSessionKeyID string) *livesession.LiveSession {
	if r == nil {
		return nil
	}
	logicalSessionKeyID = strings.TrimSpace(logicalSessionKeyID)
	if logicalSessionKeyID == "" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, session := range r.sessions {
		if session != nil && logicaltarget.LegacyLiveSessionKeyID(session) == logicalSessionKeyID {
			return session
		}
	}
	return nil
}
