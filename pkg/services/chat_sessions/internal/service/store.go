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
// episodes is the session's full, consecutively numbered TargetEpisode
// history ordered by Number, index 0 being Number 1; it is never rewritten
// in place, only replaced with a new slice on rollover. activeTurn is the
// session's current non-terminal Turn, or nil when no turn is active.
// turns holds every Turn ever admitted for this session, keyed by Turn.ID,
// so AdvanceTurn can locate a turn by identity regardless of whether it is
// still active; a turn already present here is never removed, only replaced
// in place with its advanced value. turnSequence is a private, per-session
// monotonic counter used solely to give each newly terminal Turn a distinct,
// non-zero TerminalSequence -- it is unrelated to and never written into the
// public Session.StreamHead, since this in-memory engine does not wire into
// a real event stream. attachments holds every currently connected
// Attachment keyed by its own ID; it is independent of session, episodes,
// and turns -- attaching or detaching one connection never reads or writes
// any of those fields. controls holds every ControlIntent ever requested for
// this session, keyed by its own RequestIdentity -- RequestIdentity is a
// plain comparable struct with Kind as an explicit discriminator, so equal
// JSON-RPC ids from distinct ConnectionIDs (or against a bare TransportUUID)
// are already distinct map keys with no risk of collision, retrieval, or
// overwrite between them. A control intent already present here is never
// removed, only replaced in place with its advanced value, mirroring turns.
type sessionRecord struct {
	session      chatsessions.Session
	episodes     []chatsessions.TargetEpisode
	activeTurn   *chatsessions.Turn
	turns        map[string]chatsessions.Turn
	turnSequence uint64
	attachments  map[string]chatsessions.Attachment
	controls     map[chatsessions.RequestIdentity]chatsessions.ControlIntent
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
