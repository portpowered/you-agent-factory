package chatsessions

import "time"

// ChatTargetKind names the class of destination a Chat Session can select.
type ChatTargetKind string

const (
	ChatTargetKindFactory ChatTargetKind = "FACTORY"
	ChatTargetKindWorker  ChatTargetKind = "WORKER"
)

// Validate reports whether k is one of the declared ChatTargetKind members.
// WORKER validates as a legal vocabulary value; this package does not
// introduce Worker Sessions behavior or claim that direct Worker targets are
// implemented in this slice.
func (k ChatTargetKind) Validate() error {
	switch k {
	case ChatTargetKindFactory, ChatTargetKindWorker:
		return nil
	default:
		return newValidationError("ChatTargetKind", "", ErrUnknownEnumValue)
	}
}

// ChatTargetRef identifies one selectable Chat Session destination. Ref is an
// unversioned target reference (a Factory reference when Kind is
// ChatTargetKindFactory); this package does not resolve or catalog it.
type ChatTargetRef struct {
	Kind ChatTargetKind
	Ref  string
}

// Validate reports whether the ChatTargetRef is internally consistent: Kind
// must be a declared member and Ref must not be blank.
func (t ChatTargetRef) Validate() error {
	if err := t.Kind.Validate(); err != nil {
		return newValidationError("ChatTargetRef", "Kind", err)
	}
	if t.Ref == "" {
		return newValidationError("ChatTargetRef", "Ref", ErrRequiredValue)
	}
	return nil
}

// RequestIdentity is a caller-supplied request identity that is unique only
// for the connection it was minted on. Transports must pair a per-connection
// id (such as a JSON-RPC id) with ConnectionID, or mint a process-unique
// RequestToken, before the identity crosses the Chat Sessions boundary; the
// bare per-connection id is never sufficient on its own.
type RequestIdentity struct {
	ConnectionID string
	RequestToken string
}

// Validate reports whether both identity components are present.
func (r RequestIdentity) Validate() error {
	if r.ConnectionID == "" {
		return newValidationError("RequestIdentity", "ConnectionID", ErrRequiredValue)
	}
	if r.RequestToken == "" {
		return newValidationError("RequestIdentity", "RequestToken", ErrRequiredValue)
	}
	return nil
}

// SessionState is the Chat Session lifecycle state.
type SessionState string

const (
	SessionStateCreated SessionState = "CREATED"
	SessionStateActive  SessionState = "ACTIVE"
	SessionStateClosed  SessionState = "CLOSED"
)

// Validate reports whether s is one of the declared SessionState members.
func (s SessionState) Validate() error {
	switch s {
	case SessionStateCreated, SessionStateActive, SessionStateClosed:
		return nil
	default:
		return newValidationError("SessionState", "", ErrUnknownEnumValue)
	}
}

// IsTerminal reports whether s is the Session state machine's terminal state.
func (s SessionState) IsTerminal() bool {
	return s == SessionStateClosed
}

// Session is the detached, transport-independent Chat Session identity and
// state. StreamHead, an Attachment's AfterSequence, and a ControlIntent's
// captured turn/episode/version are three distinct positions and are never
// conflated by this contract.
type Session struct {
	ID             string
	State          SessionState
	SelectedTarget ChatTargetRef
	TargetEpisode  uint64
	// ActiveTurnID is the non-terminal turn currently admitted for this
	// session, or blank when no turn is active.
	ActiveTurnID string
	Version      uint64
	// StreamHead is the last event sequence associated with this session. It
	// is distinct from any single Attachment's AfterSequence delivery cursor.
	StreamHead uint64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Validate reports whether the Session is internally consistent. A blank
// ActiveTurnID is a valid "no active turn" value; a non-blank ActiveTurnID
// while State is CREATED is inconsistent because a session only reaches
// ACTIVE once its first turn is admitted.
func (s Session) Validate() error {
	if s.ID == "" {
		return newValidationError("Session", "ID", ErrRequiredValue)
	}
	if err := s.State.Validate(); err != nil {
		return newValidationError("Session", "State", err)
	}
	if err := s.SelectedTarget.Validate(); err != nil {
		return newValidationError("Session", "SelectedTarget", err)
	}
	if s.CreatedAt.IsZero() {
		return newValidationError("Session", "CreatedAt", ErrRequiredValue)
	}
	if s.UpdatedAt.IsZero() {
		return newValidationError("Session", "UpdatedAt", ErrRequiredValue)
	}
	if s.UpdatedAt.Before(s.CreatedAt) {
		return newValidationError("Session", "UpdatedAt", ErrInconsistentValue)
	}
	if s.State == SessionStateCreated && s.ActiveTurnID != "" {
		return newValidationError("Session", "ActiveTurnID", ErrInconsistentValue)
	}
	return nil
}

// TargetEpisodeState is the lifecycle state of one TargetEpisode.
type TargetEpisodeState string

const (
	TargetEpisodeStateOpen   TargetEpisodeState = "OPEN"
	TargetEpisodeStateClosed TargetEpisodeState = "CLOSED"
)

// Validate reports whether s is one of the declared TargetEpisodeState
// members.
func (s TargetEpisodeState) Validate() error {
	switch s {
	case TargetEpisodeStateOpen, TargetEpisodeStateClosed:
		return nil
	default:
		return newValidationError("TargetEpisodeState", "", ErrUnknownEnumValue)
	}
}

// IsTerminal reports whether s is the TargetEpisode state machine's terminal
// state.
func (s TargetEpisodeState) IsTerminal() bool {
	return s == TargetEpisodeStateClosed
}

// TargetEpisode is one immutable episode of a Chat Session's target history.
// Changing a session's target never rewrites a prior episode's identity.
type TargetEpisode struct {
	Number           uint64
	State            TargetEpisodeState
	Target           ChatTargetRef
	FactorySessionID string
	StartedAt        time.Time
	// ClosedAt is the episode's terminal time fact: unset while State is
	// OPEN and set (never before StartedAt) once State is CLOSED.
	ClosedAt *time.Time
}

// Validate reports whether the TargetEpisode is internally consistent,
// including the ClosedAt terminal time fact against State.
func (e TargetEpisode) Validate() error {
	if err := e.State.Validate(); err != nil {
		return newValidationError("TargetEpisode", "State", err)
	}
	if err := e.Target.Validate(); err != nil {
		return newValidationError("TargetEpisode", "Target", err)
	}
	if e.StartedAt.IsZero() {
		return newValidationError("TargetEpisode", "StartedAt", ErrRequiredValue)
	}
	switch e.State {
	case TargetEpisodeStateOpen:
		if e.ClosedAt != nil {
			return newValidationError("TargetEpisode", "ClosedAt", ErrInconsistentValue)
		}
	case TargetEpisodeStateClosed:
		if e.ClosedAt == nil {
			return newValidationError("TargetEpisode", "ClosedAt", ErrInconsistentValue)
		}
		if e.ClosedAt.Before(e.StartedAt) {
			return newValidationError("TargetEpisode", "ClosedAt", ErrInconsistentValue)
		}
	}
	return nil
}

// TurnState is the lifecycle state of one Turn.
type TurnState string

const (
	TurnStateAdmitted  TurnState = "ADMITTED"
	TurnStateRunning   TurnState = "RUNNING"
	TurnStateCompleted TurnState = "COMPLETED"
	TurnStateFailed    TurnState = "FAILED"
	TurnStateCanceled  TurnState = "CANCELED"
)

// Validate reports whether s is one of the declared TurnState members.
func (s TurnState) Validate() error {
	switch s {
	case TurnStateAdmitted, TurnStateRunning, TurnStateCompleted, TurnStateFailed, TurnStateCanceled:
		return nil
	default:
		return newValidationError("TurnState", "", ErrUnknownEnumValue)
	}
}

// IsTerminal reports whether s is one of the Turn state machine's terminal
// states (COMPLETED, FAILED, CANCELED). At most one non-terminal turn may be
// active per Session.
func (s TurnState) IsTerminal() bool {
	switch s {
	case TurnStateCompleted, TurnStateFailed, TurnStateCanceled:
		return true
	default:
		return false
	}
}

// Turn is one admitted unit of work within a TargetEpisode.
type Turn struct {
	ID        string
	Episode   uint64
	State     TurnState
	RequestID RequestIdentity
	// StartSequence is the stream sequence at which this turn began
	// producing events, or zero when the turn has not yet started.
	StartSequence uint64
	// TerminalSequence is the turn's terminal sequence fact: zero while State
	// is non-terminal, and non-zero (never before StartSequence when
	// StartSequence is set) once State reaches a terminal value.
	TerminalSequence uint64
}

// Validate reports whether the Turn is internally consistent, including the
// TerminalSequence terminal sequence fact against State.
func (t Turn) Validate() error {
	if t.ID == "" {
		return newValidationError("Turn", "ID", ErrRequiredValue)
	}
	if err := t.State.Validate(); err != nil {
		return newValidationError("Turn", "State", err)
	}
	if err := t.RequestID.Validate(); err != nil {
		return newValidationError("Turn", "RequestID", err)
	}
	if t.State.IsTerminal() {
		if t.TerminalSequence == 0 {
			return newValidationError("Turn", "TerminalSequence", ErrInconsistentValue)
		}
		if t.StartSequence != 0 && t.TerminalSequence < t.StartSequence {
			return newValidationError("Turn", "TerminalSequence", ErrInconsistentValue)
		}
	} else if t.TerminalSequence != 0 {
		return newValidationError("Turn", "TerminalSequence", ErrInconsistentValue)
	}
	return nil
}

// Attachment is one connection's delivery position against a Chat Session's
// event stream. AfterSequence is that attachment's own delivery cursor and is
// never the same position as Session.StreamHead.
type Attachment struct {
	ID           string
	SessionID    string
	ConnectionID string
	// AfterSequence is the last sequence already delivered to this
	// attachment; zero is a valid "nothing delivered yet" value.
	AfterSequence uint64
	Interactive   bool
}

// Validate reports whether the Attachment carries its required identities.
func (a Attachment) Validate() error {
	if a.ID == "" {
		return newValidationError("Attachment", "ID", ErrRequiredValue)
	}
	if a.SessionID == "" {
		return newValidationError("Attachment", "SessionID", ErrRequiredValue)
	}
	if a.ConnectionID == "" {
		return newValidationError("Attachment", "ConnectionID", ErrRequiredValue)
	}
	return nil
}

// ControlAction names an operation a ControlIntent may request.
type ControlAction string

const (
	ControlActionCancel ControlAction = "CANCEL"
	ControlActionClose  ControlAction = "CLOSE"
	// ControlActionPause, ControlActionResume, and ControlActionTerminate are
	// declared vocabulary for a later lane. They validate as
	// ErrUnsupportedControlAction, not ErrUnknownEnumValue, and are never
	// treated as executable L1 actions.
	ControlActionPause     ControlAction = "PAUSE"
	ControlActionResume    ControlAction = "RESUME"
	ControlActionTerminate ControlAction = "TERMINATE"
)

// Validate reports whether a is a declared ControlAction. CANCEL and CLOSE
// are the only actions this L1 V0 slice treats as executable; PAUSE, RESUME,
// and TERMINATE are declared for a later lane and report
// ErrUnsupportedControlAction rather than ErrUnknownEnumValue.
func (a ControlAction) Validate() error {
	switch a {
	case ControlActionCancel, ControlActionClose:
		return nil
	case ControlActionPause, ControlActionResume, ControlActionTerminate:
		return newValidationError("ControlAction", "", ErrUnsupportedControlAction)
	default:
		return newValidationError("ControlAction", "", ErrUnknownEnumValue)
	}
}

// SupportedInL1 reports whether a is executable in this slice's contract
// (CANCEL or CLOSE), as distinct from merely being declared vocabulary.
func (a ControlAction) SupportedInL1() bool {
	return a == ControlActionCancel || a == ControlActionClose
}

// ControlIntentState is the lifecycle state of one ControlIntent.
type ControlIntentState string

const (
	ControlIntentStateRequested  ControlIntentState = "REQUESTED"
	ControlIntentStateCommitted  ControlIntentState = "COMMITTED"
	ControlIntentStateCompleted  ControlIntentState = "COMPLETED"
	ControlIntentStateNoop       ControlIntentState = "NOOP"
	ControlIntentStateSuperseded ControlIntentState = "SUPERSEDED"
)

// Validate reports whether s is one of the declared ControlIntentState
// members.
func (s ControlIntentState) Validate() error {
	switch s {
	case ControlIntentStateRequested, ControlIntentStateCommitted,
		ControlIntentStateCompleted, ControlIntentStateNoop, ControlIntentStateSuperseded:
		return nil
	default:
		return newValidationError("ControlIntentState", "", ErrUnknownEnumValue)
	}
}

// IsTerminal reports whether s is one of the ControlIntent state machine's
// terminal states (COMPLETED, NOOP, SUPERSEDED). NOOP and SUPERSEDED are
// distinct outcomes: NOOP means the captured turn was already terminal on
// arrival, SUPERSEDED means the captured turn was no longer current and the
// intent was never applied.
func (s ControlIntentState) IsTerminal() bool {
	switch s {
	case ControlIntentStateCompleted, ControlIntentStateNoop, ControlIntentStateSuperseded:
		return true
	default:
		return false
	}
}

// ControlIntent captures one control request (cancel or close) against a
// specific turn, episode, and expected session version at the moment the
// intent commits. Chat Sessions captures this target atomically before
// invoking any downstream control so a newly admitted turn can never be
// affected by an older control intent.
type ControlIntent struct {
	RequestID RequestIdentity
	SessionID string
	// TurnID is the concrete turn this intent targets. Control targets are
	// always a captured turn ID, episode, and expected version — never the
	// session's StreamHead or an Attachment's AfterSequence cursor.
	TurnID          string
	TargetEpisode   uint64
	ExpectedVersion uint64
	Action          ControlAction
	State           ControlIntentState
	RequestedAt     time.Time
}

// Validate reports whether the ControlIntent carries its required identities
// and a declared Action/State.
func (c ControlIntent) Validate() error {
	if err := c.RequestID.Validate(); err != nil {
		return newValidationError("ControlIntent", "RequestID", err)
	}
	if c.SessionID == "" {
		return newValidationError("ControlIntent", "SessionID", ErrRequiredValue)
	}
	if c.TurnID == "" {
		return newValidationError("ControlIntent", "TurnID", ErrRequiredValue)
	}
	if err := c.Action.Validate(); err != nil {
		return newValidationError("ControlIntent", "Action", err)
	}
	if err := c.State.Validate(); err != nil {
		return newValidationError("ControlIntent", "State", err)
	}
	if c.RequestedAt.IsZero() {
		return newValidationError("ControlIntent", "RequestedAt", ErrRequiredValue)
	}
	return nil
}
