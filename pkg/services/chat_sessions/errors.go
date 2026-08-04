package chatsessions

import (
	"errors"
	"fmt"
)

// Sentinel value-validation categories. Callers use errors.Is against these
// sentinels (typically through a returned *ValidationError) instead of
// parsing error text.
var (
	// ErrRequiredValue reports a blank or zero value where the L1 V0 contract
	// requires a caller-supplied identity, reference, or timestamp.
	ErrRequiredValue = errors.New("chat sessions: required value is blank or zero")
	// ErrUnknownEnumValue reports an enum value outside its declared members.
	ErrUnknownEnumValue = errors.New("chat sessions: unknown enum value")
	// ErrInconsistentValue reports a value whose fields disagree with each
	// other under the L1 V0 model, such as a terminal sequence or terminal
	// time fact that does not match the value's declared state, or a
	// RequestIdentity carrying a field that is populated but inactive for its
	// declared Kind.
	ErrInconsistentValue = errors.New("chat sessions: structurally inconsistent value")
	// ErrMalformedValue reports a non-blank value that fails a required
	// lexical shape, such as a RequestIdentity TransportUUID that is not a
	// well-formed UUID. It is distinct from ErrRequiredValue (blank/zero) and
	// ErrUnknownEnumValue (declared enum vocabulary).
	ErrMalformedValue = errors.New("chat sessions: malformed value")
	// ErrUnsupportedControlAction reports a ControlAction that is declared in
	// the L1 vocabulary for a later lane (PAUSE, RESUME, TERMINATE) but is not
	// an executable action in this L1 V0 slice. It is distinct from
	// ErrUnknownEnumValue, which reports a value outside the declared
	// vocabulary entirely.
	ErrUnsupportedControlAction = errors.New("chat sessions: control action is not supported in L1")
	// ErrInvalidTransition reports a state pair that is not a legal L1 V0
	// lifecycle transition, including any transition attempted from a
	// terminal state. It is distinct from the story-001 value-validation
	// sentinels: both the from and to states are already declared enum
	// members, but the L1 V0 transition table does not permit moving between
	// them.
	ErrInvalidTransition = errors.New("chat sessions: illegal state transition")
	// ErrNotFound reports that a Service operation referenced a session or a
	// subordinate entity (turn, attachment, control intent) that does not
	// exist.
	ErrNotFound = errors.New("chat sessions: not found")
	// ErrBusy reports that a Service operation was rejected because the
	// session already has a non-terminal active turn.
	ErrBusy = errors.New("chat sessions: active turn busy")
	// ErrStaleVersion reports that a Service operation's expected session
	// version no longer matches the session's current version.
	ErrStaleVersion = errors.New("chat sessions: stale expected version")
	// ErrTargetEpisodeNotClosed reports that OpenNextTargetEpisode was
	// invoked against a prior TargetEpisode that has not yet reached its
	// CLOSED terminal state. It is distinct from ErrInvalidTransition: the
	// prior TargetEpisodeState itself is a legal, validated value, but the
	// immutable-episode rule requires it to already be closed before the
	// next episode can be opened.
	ErrTargetEpisodeNotClosed = errors.New("chat sessions: prior target episode is not closed")
	// ErrTargetEpisodeNumberExhausted reports that OpenNextTargetEpisode was
	// invoked against a prior TargetEpisode already at the maximum
	// representable episode Number (math.MaxUint64), so prior.Number+1 would
	// silently wrap to 0 instead of producing the next monotonic episode
	// identity.
	ErrTargetEpisodeNumberExhausted = errors.New("chat sessions: target episode number is exhausted")
	// ErrFactorySessionAlreadyBound reports that BindFactorySession could
	// not commit a Factory Session identity onto a TargetEpisode because
	// that episode's binding is already committed to a different identity.
	// It is distinct from ErrStaleVersion: the caller's ExpectedVersion was
	// current, but a concurrent or earlier bind attempt already won.
	ErrFactorySessionAlreadyBound = errors.New("chat sessions: target episode factory session is already bound to a different identity")
	// ErrSequencedIdentityContradiction reports that Sequence was called
	// again with an already-accepted (SourceType, SourceID, SourceSequence,
	// SourceEventID) identity tuple, but ParentItemID, Kind, SchemaID, or
	// Payload contradicts the record originally committed for that exact
	// tuple. Sequence rejects the reused tuple deterministically instead of
	// silently returning the stale, contradicted identity.
	ErrSequencedIdentityContradiction = errors.New("chat sessions: reused source identity contradicts originally sequenced record")
	// ErrAttachmentBeyondStreamHead reports that AcknowledgeAttachment was
	// asked to advance an Attachment's AfterSequence past the session's
	// current StreamHead -- a position no Sequence call has committed yet,
	// so no attachment can legitimately claim to have observed it.
	ErrAttachmentBeyondStreamHead = errors.New("chat sessions: requested attachment position is beyond the session's stream head")
	// ErrAttachmentRetentionGap reports that AcknowledgeAttachment's
	// requested AfterSequence spans a range between the attachment's current
	// position and the requested one that Events retention has since
	// evicted, so the attachment cannot have genuinely observed it.
	ErrAttachmentRetentionGap = errors.New("chat sessions: requested attachment position spans an evicted retention gap")
	// ErrUncommittedStreamPosition reports that AdvanceStreamHead was asked
	// to advance StreamHead to an AggregateSequence this session's sequencer
	// never actually committed for the exact stated (SourceType, SourceID,
	// SourceSequence, SourceEventID) tuple -- either the position was never
	// assigned by Sequence for this session at all, or it was assigned to a
	// different source identity (including one sequenced in another
	// session). AdvanceStreamHead's contract is "the position a prior
	// accepted Sequence call committed for this exact source identity";
	// trusting an unvalidated position would let StreamHead advance to
	// fabricated or cross-session state, which AcknowledgeAttachment would
	// then trust as proof of a range no attachment ever actually observed.
	ErrUncommittedStreamPosition = errors.New("chat sessions: requested stream head position was not committed by the stated source identity for this session")
)

// ValidationError reports one Chat Sessions value-validation failure. Value
// names the owning type, Field names the offending field (empty when the
// failure applies to the whole value, such as an enum receiver), and Err is
// one of the package sentinel errors so callers can use errors.Is/errors.As
// without parsing Error() text.
type ValidationError struct {
	Value string
	Field string
	Err   error
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("chat sessions: %s: %v", e.Value, e.Err)
	}
	return fmt.Sprintf("chat sessions: %s.%s: %v", e.Value, e.Field, e.Err)
}

// Unwrap exposes the underlying sentinel (or nested *ValidationError) so
// errors.Is/errors.As can classify the failure across a wrapped chain.
func (e *ValidationError) Unwrap() error {
	return e.Err
}

func newValidationError(value, field string, err error) *ValidationError {
	return &ValidationError{Value: value, Field: field, Err: err}
}

// TransitionError reports one illegal Chat Sessions lifecycle transition.
// Value names the owning state machine and From/To record the attempted
// state pair. Err is ErrInvalidTransition unless From or To is itself not a
// declared enum member, in which case Err is the underlying
// ErrUnknownEnumValue *ValidationError so callers can distinguish an
// invalid-state input from a legal-state-but-illegal-transition outcome.
type TransitionError struct {
	Value string
	From  string
	To    string
	Err   error
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("chat sessions: %s: %s -> %s: %v", e.Value, e.From, e.To, e.Err)
}

// Unwrap exposes the underlying sentinel so errors.Is/errors.As can classify
// the failure.
func (e *TransitionError) Unwrap() error {
	return e.Err
}

func newTransitionError(value, from, to string) *TransitionError {
	return &TransitionError{Value: value, From: from, To: to, Err: ErrInvalidTransition}
}

// NotFoundError reports that a Service operation referenced a session or
// subordinate entity that does not exist. Value names the entity kind (for
// example "Session" or "Turn") and ID names the identity that was not found.
type NotFoundError struct {
	Value string
	ID    string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("chat sessions: %s %q: %v", e.Value, e.ID, ErrNotFound)
}

// Unwrap exposes ErrNotFound so errors.Is/errors.As can classify the
// failure.
func (e *NotFoundError) Unwrap() error {
	return ErrNotFound
}

// BusyError reports that a Service operation was rejected because the named
// session already has a non-terminal active turn. ActiveTurnID and
// ActiveTurnState carry the safe identity and state of that blocking turn so
// a caller can distinguish which turn is busy without a follow-up read.
type BusyError struct {
	Value           string
	ID              string
	ActiveTurnID    string
	ActiveTurnState TurnState
}

func (e *BusyError) Error() string {
	return fmt.Sprintf("chat sessions: %s %q: active turn %q (%s): %v", e.Value, e.ID, e.ActiveTurnID, e.ActiveTurnState, ErrBusy)
}

// Unwrap exposes ErrBusy so errors.Is/errors.As can classify the failure.
func (e *BusyError) Unwrap() error {
	return ErrBusy
}

// ConflictError reports that a Service operation's Expected session version
// no longer matches Actual, the session's current version.
type ConflictError struct {
	Value    string
	ID       string
	Expected uint64
	Actual   uint64
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("chat sessions: %s %q: expected version %d, current version %d: %v",
		e.Value, e.ID, e.Expected, e.Actual, ErrStaleVersion)
}

// Unwrap exposes ErrStaleVersion so errors.Is/errors.As can classify the
// failure.
func (e *ConflictError) Unwrap() error {
	return ErrStaleVersion
}

// FactorySessionConflictError reports that BindFactorySession rejected a
// commit attempt because the named session's target episode already carries
// a different Factory Session identity than the one being bound. Bound is
// the identity already committed; Attempted is the identity the rejected
// call tried to commit instead. Neither value is sensitive: both are opaque
// Factory Session identifiers, safe to cross a service boundary the same way
// BusyError's ActiveTurnID already is.
type FactorySessionConflictError struct {
	SessionID string
	Episode   uint64
	Bound     string
	Attempted string
}

func (e *FactorySessionConflictError) Error() string {
	return fmt.Sprintf("chat sessions: session %q episode %d: %v: bound %q, attempted %q",
		e.SessionID, e.Episode, ErrFactorySessionAlreadyBound, e.Bound, e.Attempted)
}

// Unwrap exposes ErrFactorySessionAlreadyBound so errors.Is/errors.As can
// classify the failure.
func (e *FactorySessionConflictError) Unwrap() error {
	return ErrFactorySessionAlreadyBound
}

// SequencedIdentityConflictError reports one Sequence call that reused an
// already-accepted (SourceType, SourceID, SourceSequence, SourceEventID)
// identity tuple with a Field whose value contradicts the record originally
// committed for that exact tuple. SourceType/SourceID/SourceSequence/
// SourceEventID are opaque source identity values, safe to cross a service
// boundary the same way BusyError's ActiveTurnID already is.
type SequencedIdentityConflictError struct {
	SessionID      string
	SourceType     string
	SourceID       string
	SourceSequence uint64
	SourceEventID  string
	// Field names the contradicted request field: "ParentItemID", "Kind",
	// "SchemaID", or "Payload".
	Field string
}

func (e *SequencedIdentityConflictError) Error() string {
	return fmt.Sprintf("chat sessions: session %q source identity (%s, %s, %d, %s): %s: %v",
		e.SessionID, e.SourceType, e.SourceID, e.SourceSequence, e.SourceEventID, e.Field, ErrSequencedIdentityContradiction)
}

// Unwrap exposes ErrSequencedIdentityContradiction so errors.Is/errors.As
// can classify the failure.
func (e *SequencedIdentityConflictError) Unwrap() error {
	return ErrSequencedIdentityContradiction
}

// AttachmentPositionError reports that AcknowledgeAttachment rejected a
// requested AfterSequence because it names a position beyond the session's
// current StreamHead. StreamHead is the session's actual bound at rejection
// time; neither the attachment nor the session is mutated.
type AttachmentPositionError struct {
	SessionID    string
	AttachmentID string
	Requested    uint64
	StreamHead   uint64
}

func (e *AttachmentPositionError) Error() string {
	return fmt.Sprintf("chat sessions: session %q attachment %q: requested position %d exceeds stream head %d: %v",
		e.SessionID, e.AttachmentID, e.Requested, e.StreamHead, ErrAttachmentBeyondStreamHead)
}

// Unwrap exposes ErrAttachmentBeyondStreamHead so errors.Is/errors.As can
// classify the failure.
func (e *AttachmentPositionError) Unwrap() error {
	return ErrAttachmentBeyondStreamHead
}

// AttachmentRetentionGapError reports that AcknowledgeAttachment rejected a
// requested AfterSequence because Events retention no longer retains the
// full range between the attachment's current position and the requested
// one -- the attachment cannot have genuinely observed records that no
// longer exist to prove delivery of. EarliestRetained and Head are the
// Events-reported retained bounds at rejection time; neither the attachment
// nor the session is mutated.
type AttachmentRetentionGapError struct {
	SessionID        string
	AttachmentID     string
	Requested        uint64
	EarliestRetained uint64
	Head             uint64
}

func (e *AttachmentRetentionGapError) Error() string {
	return fmt.Sprintf("chat sessions: session %q attachment %q: requested position %d: %v: earliest retained %d, head %d",
		e.SessionID, e.AttachmentID, e.Requested, ErrAttachmentRetentionGap, e.EarliestRetained, e.Head)
}

// Unwrap exposes ErrAttachmentRetentionGap so errors.Is/errors.As can
// classify the failure.
func (e *AttachmentRetentionGapError) Unwrap() error {
	return ErrAttachmentRetentionGap
}

// UncommittedStreamPositionError reports that AdvanceStreamHead rejected a
// requested AggregateSequence because this session's sequencer never
// committed that exact position for the stated source identity tuple.
// Neither the session nor any attachment is mutated. SourceType/SourceID/
// SourceSequence/SourceEventID are opaque source identity values, safe to
// cross a service boundary the same way BusyError's ActiveTurnID already is.
type UncommittedStreamPositionError struct {
	SessionID         string
	AggregateSequence uint64
	SourceType        string
	SourceID          string
	SourceSequence    uint64
	SourceEventID     string
}

func (e *UncommittedStreamPositionError) Error() string {
	return fmt.Sprintf("chat sessions: session %q: position %d source identity (%s, %s, %d, %s): %v",
		e.SessionID, e.AggregateSequence, e.SourceType, e.SourceID, e.SourceSequence, e.SourceEventID, ErrUncommittedStreamPosition)
}

// Unwrap exposes ErrUncommittedStreamPosition so errors.Is/errors.As can
// classify the failure.
func (e *UncommittedStreamPositionError) Unwrap() error {
	return ErrUncommittedStreamPosition
}

// TargetEpisodeNotClosedError reports that OpenNextTargetEpisode was invoked
// against a prior TargetEpisode that has not yet reached its CLOSED terminal
// state. Number and State name the offending prior episode's own identity
// and state, not the caller-supplied target.
type TargetEpisodeNotClosedError struct {
	Number uint64
	State  TargetEpisodeState
}

func (e *TargetEpisodeNotClosedError) Error() string {
	return fmt.Sprintf("chat sessions: TargetEpisode %d: %v: state %s", e.Number, ErrTargetEpisodeNotClosed, e.State)
}

// Unwrap exposes ErrTargetEpisodeNotClosed so errors.Is/errors.As can
// classify the failure.
func (e *TargetEpisodeNotClosedError) Unwrap() error {
	return ErrTargetEpisodeNotClosed
}
