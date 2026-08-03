package service

import (
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

var _ chatsessions.Service = (*Store)(nil)

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

	newID  IDGenerator
	now    Clock
	logger logging.Logger
}

// sessionRecord is the Store-owned mutable aggregate for one Chat Session.
// episodes is the session's full, consecutively numbered TargetEpisode
// history ordered by Number, index 0 being Number 1; it is never rewritten
// in place, only replaced with a new slice on rollover. turns holds every
// Turn ever admitted for this session, keyed by Turn.ID, so AdvanceTurn can
// locate a turn by identity regardless of whether it is still active; a turn
// already present here is never removed, only replaced in place with its
// advanced value. Busy checks read the live active Turn through
// activeTurnValue() (session.ActiveTurnID looked up against turns) rather
// than a second, independently-mutable copy: an earlier revision kept a
// separate *chatsessions.Turn pointer that AdvanceTurn only refreshed on a
// terminal transition, so a BusyError raised after a public
// ADMITTED->RUNNING advancement reported the turn's stale ADMITTED state
// instead of its live RUNNING state. lastTurnID is the ID of the most
// recently admitted turn regardless of its current state -- unlike
// session.ActiveTurnID, it is never cleared when that turn terminates, only
// overwritten by the next StartTurn -- so AdvanceControl can distinguish "the
// captured turn already finished and no newer turn has been admitted"
// (NOOP) from "a newer turn has since been admitted" (SUPERSEDED); comparing
// against session.ActiveTurnID instead could never produce NOOP, since
// ActiveTurnID clears to blank in the same commit that marks a turn
// terminal. turnSequence is a private, per-session monotonic counter used
// solely to give each newly terminal Turn a distinct, non-zero
// TerminalSequence -- it is unrelated to and never written into the public
// Session.StreamHead, since this in-memory engine does not wire into a real
// event stream. attachments holds every currently connected Attachment keyed
// by its own ID; it is independent of session, episodes, and turns --
// attaching or detaching one connection never reads or writes any of those
// fields. controls holds every ControlIntent ever requested for this
// session, keyed by its own RequestIdentity -- RequestIdentity is a plain
// comparable struct with Kind as an explicit discriminator, so equal
// JSON-RPC ids from distinct ConnectionIDs (or against a bare TransportUUID)
// are already distinct map keys with no risk of collision, retrieval, or
// overwrite between them. A control intent already present here is never
// removed or overwritten, only advanced in place via AdvanceControl, so a
// reused RequestID can never retarget or rewrite an existing intent's
// captured facts.
type sessionRecord struct {
	session      chatsessions.Session
	episodes     []chatsessions.TargetEpisode
	turns        map[string]chatsessions.Turn
	lastTurnID   string
	turnSequence uint64
	attachments  map[string]chatsessions.Attachment
	controls     map[chatsessions.RequestIdentity]chatsessions.ControlIntent
}

// activeTurnValue returns the session's current active Turn read live from
// turns, or false when no turn is active. Reading through turns (rather than
// a second cached copy) guarantees the returned State always reflects the
// most recent AdvanceTurn commit, including a non-terminal advancement such
// as ADMITTED->RUNNING.
func (record sessionRecord) activeTurnValue() (chatsessions.Turn, bool) {
	if record.session.ActiveTurnID == "" {
		return chatsessions.Turn{}, false
	}
	turn, ok := record.turns[record.session.ActiveTurnID]
	return turn, ok
}

// New constructs an empty Store from explicit dependencies. newID and now
// must be non-nil. logger is optional and defaults to a no-op logger when
// omitted, matching the repository's optional-logger construction
// convention rather than a mutable reinjection path.
func New(newID IDGenerator, now Clock, logger ...logging.Logger) *Store {
	var provided logging.Logger
	if len(logger) > 0 {
		provided = logger[0]
	}
	return &Store{
		sessions: make(map[string]sessionRecord),
		newID:    newID,
		now:      now,
		logger:   logging.EnsureLogger(provided),
	}
}
