package service

import (
	"sync"
	"time"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// IDGenerator produces a new opaque, process-unique entity identity. Store
// never chooses an ambient UUID source itself; chat_sessions/wire supplies
// the production generator.
type IDGenerator func() string

// Clock returns the current time. Store never calls time.Now directly, so a
// caller can supply a deterministic clock under test.
type Clock func() time.Time

// Store is the synchronized in-memory implementation of the L1 V1 Chat
// Sessions engine. The zero value is not usable; construct with New.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]sessionRecord

	newID IDGenerator
	now   Clock
}

// sessionRecord is the Store-owned mutable aggregate for one Chat Session.
// Story 001 tracks only the Session value itself; later stories add the
// episode, turn, attachment, and control-intent state that hangs off it.
type sessionRecord struct {
	session chatsessions.Session
}

// New constructs an empty Store from explicit dependencies. newID and now
// must be non-nil.
func New(newID IDGenerator, now Clock) *Store {
	return &Store{
		sessions: make(map[string]sessionRecord),
		newID:    newID,
		now:      now,
	}
}
