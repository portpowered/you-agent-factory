package service

import (
	"context"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
)

var _ chatsessions.Service = (*Store)(nil)

// IDGenerator produces a new opaque, process-unique entity identity. Store
// never chooses an ambient UUID source itself; chat_sessions/wire supplies
// the production generator.
type IDGenerator func() string

// Clock returns the current time. Store never calls time.Now directly, so a
// caller can supply a deterministic clock under test.
type Clock func() time.Time

// EventsAppender is the narrow Events dependency the sequencer needs: commit
// one source-native record into a topic's aggregate ordering. Store depends
// on this port rather than the full events.Service so a caller wiring only
// the sequencer's own tests never has to satisfy Read/Subscribe/AttachSource
// too. Any events.Service value already satisfies this interface
// structurally.
type EventsAppender interface {
	Append(context.Context, events.AppendRequest) (events.AppendResult, error)
}

// EventsReader is the narrow Events dependency AcknowledgeAttachment needs:
// read a bounded slice of a topic's aggregate ordering to detect whether the
// range between an attachment's current position and its requested new
// position has fallen outside Events' retention window. Store depends on
// this port rather than the full events.Service for the same reason
// EventsAppender is narrow -- a caller wiring only tests that never call
// AcknowledgeAttachment never has to satisfy it. Any events.Service value
// already satisfies this interface structurally.
type EventsReader interface {
	Read(context.Context, events.ReadRequest) (events.ReadResult, error)
}

// Store is the synchronized in-memory implementation of the L1 V1 Chat
// Sessions engine. The zero value is not usable; construct with New.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]sessionRecord

	newID          IDGenerator
	now            Clock
	eventsAppender EventsAppender
	eventsReader   EventsReader
	logger         logging.Logger
}

// sequencedSourceIdentity is the (SourceType, SourceID, SourceSequence,
// SourceEventID) tuple Sequence committed for one aggregate position within
// one session. AdvanceStreamHead consults sessionRecord.sequencedPositions
// (keyed by that same aggregate position) against this exact tuple before
// ever advancing StreamHead, so a caller can never move StreamHead to a
// position this session's sequencer did not actually commit for the stated
// source identity -- including a position that belongs to a different
// session's own topic, or one committed for a different source tuple.
type sequencedSourceIdentity struct {
	SourceType     events.SourceType
	SourceID       events.SourceID
	SourceSequence events.SourceSequence
	SourceEventID  events.SourceEventID
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
// turnsByRequest indexes every admitted Turn by the RequestIdentity that
// admitted it, independent of turns (which is keyed by Turn.ID) -- so
// StartTurn can recognize a redelivered request (including one whose
// originally admitted turn has since terminalized and released
// ActiveTurnID) and return the existing turn instead of admitting a second
// one and dispatching its effects again. Like controls, an entry here is
// never removed or overwritten.
// sequencedItemIDs holds the ItemID of every aggregate record this session's
// sequencer has ever assigned, independent of turns, episodes, attachments,
// and controls. Sequence consults it to validate a child record's
// ParentItemID (accepted only when the parent's ItemID is already a member)
// and adds to it only on a newly accepted record -- a duplicate resolution
// never inserts a second time, since the ItemID it resolves to is already a
// member from the original accepted call.
// sequencedPositions holds, for every aggregate position this session's
// sequencer has ever committed, the exact source identity tuple Sequence
// committed it under. AdvanceStreamHead is the sole reader: it rejects any
// requested AggregateSequence that is not a member here under the caller's
// exact stated source identity, so StreamHead can never advance to a
// fabricated, uncommitted, or cross-session position. Like
// sequencedItemIDs, it is written only on a newly accepted Sequence record.
type sessionRecord struct {
	session            chatsessions.Session
	episodes           []chatsessions.TargetEpisode
	turns              map[string]chatsessions.Turn
	turnsByRequest     map[chatsessions.RequestIdentity]string
	lastTurnID         string
	turnSequence       uint64
	attachments        map[string]chatsessions.Attachment
	controls           map[chatsessions.RequestIdentity]chatsessions.ControlIntent
	sequencedItemIDs   map[string]struct{}
	sequencedPositions map[events.AggregateSequence]sequencedSourceIdentity
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

// NewStore constructs an empty Store from explicit dependencies. newID and
// now must be non-nil; eventsAppender and eventsReader may be nil for a
// Store a caller knows will never exercise Sequence or AcknowledgeAttachment
// (most existing Store tests predate the sequencer and never call either).
// logger is optional and defaults to a no-op logger when omitted. Every
// dependency is injected directly here, once, through this one canonical
// constructor -- general-backend-standards.md §2 forbids a secondary
// injector such as a post-construction `With<X>` mutator, so callers that do
// need Sequence/AcknowledgeAttachment must supply both events ports up
// front rather than attaching them afterward. Named NewStore (not New)
// because this package also owns the unrelated FactoryTargetCatalog
// Service's own New constructor (service.go); Go does not allow two
// same-named top-level functions in one package.
func NewStore(newID IDGenerator, now Clock, eventsAppender EventsAppender, eventsReader EventsReader, logger ...logging.Logger) *Store {
	var provided logging.Logger
	if len(logger) > 0 {
		provided = logger[0]
	}
	return &Store{
		sessions:       make(map[string]sessionRecord),
		newID:          newID,
		now:            now,
		eventsAppender: eventsAppender,
		eventsReader:   eventsReader,
		logger:         logging.EnsureLogger(provided),
	}
}
