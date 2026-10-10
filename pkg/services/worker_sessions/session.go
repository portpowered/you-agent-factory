package workersessions

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// ErrInvalidSessionMetadata rejects malformed descriptive admission facts.
var ErrInvalidSessionMetadata = errors.New("worker sessions: invalid session metadata")

// Requester identifies the Worker Session that produced or invoked this work.
// These nonsecret facts describe lineage; they do not authorize messaging.
type Requester struct {
	Kind            string `json:"kind"`
	WorkerSessionID string `json:"workerSessionId"`
	WorkID          string `json:"workId,omitempty"`
}

// Correlation retains the original work context through direct continuation.
type Correlation struct {
	WorkID           string `json:"workId,omitempty"`
	FactorySessionID string `json:"factorySessionId,omitempty"`
}

// SessionMetadata carries only nonsecret facts supplied by the admitting owner.
// Nil metadata preserves legacy absence; a nil Requester describes an
// unattributed root and never implies an operator or ambient message target.
type SessionMetadata struct {
	Requester   *Requester   `json:"requester"`
	Correlation *Correlation `json:"correlation,omitempty"`
	Labels      []string     `json:"labels,omitempty"`
}

const (
	maxSessionLabels     = 32
	maxSessionLabelRunes = 200
)

// Validate checks the additive metadata contract without rewriting its values.
func (m *SessionMetadata) Validate() error {
	if m == nil {
		return nil
	}
	if m.Requester != nil && (m.Requester.Kind != "WORKER_SESSION" ||
		!validSessionID(m.Requester.WorkerSessionID) || !validOptionalMetadataID(m.Requester.WorkID)) {
		return ErrInvalidSessionMetadata
	}
	if m.Correlation != nil && (!validOptionalMetadataID(m.Correlation.WorkID) ||
		!validOptionalMetadataID(m.Correlation.FactorySessionID)) {
		return ErrInvalidSessionMetadata
	}
	if len(m.Labels) > maxSessionLabels {
		return ErrInvalidSessionMetadata
	}
	for _, label := range m.Labels {
		if !utf8.ValidString(label) || utf8.RuneCountInString(label) > maxSessionLabelRunes {
			return ErrInvalidSessionMetadata
		}
	}
	return nil
}

func validOptionalMetadataID(id string) bool {
	return id == "" || validSessionID(id)
}

// Clone detaches every reference-backed field, including opaque labels.
func (m *SessionMetadata) Clone() *SessionMetadata {
	if m == nil {
		return nil
	}
	clone := *m
	if m.Requester != nil {
		requester := *m.Requester
		clone.Requester = &requester
	}
	if m.Correlation != nil {
		correlation := *m.Correlation
		clone.Correlation = &correlation
	}
	if m.Labels != nil {
		clone.Labels = append([]string{}, m.Labels...)
	}
	return &clone
}

// Session is an immutable snapshot of one Worker Session's stable identity,
// current lifecycle state, and — once terminal — its committed TerminalResult.
// Session is a plain value; callers that mutate a returned Session, or its
// Result, never affect registry-owned state.
type Session struct {
	ID       string
	State    State
	Metadata *SessionMetadata `json:"Metadata,omitempty"`
	// Model is the optional model identifier resolved for the provider
	// invocation that opened this Worker Session. A nil value is durable
	// absence: Worker Sessions never derives it from current configuration.
	Model *string
	// ReasoningEffort is the optional reasoning effort resolved for the
	// provider invocation that opened this Worker Session. It is independent
	// from Model because providers may report either fact without the other.
	ReasoningEffort *string
	// Result is non-nil exactly when State is StateCompleted or StateFailed,
	// and carries the exactly-once committed terminal outcome. Result is nil
	// for every non-terminal state, and for the W1 CANCELED/TERMINATED
	// states that W2 does not produce.
	Result *TerminalResult
	// ProviderSessionAssociation is the optional exact Providers-owned
	// reference observed for this session's supervised attempt. A nil value is
	// explicit absence: Worker Sessions never synthesizes it from a runner,
	// model, current provider, or bare session ID.
	ProviderSessionAssociation *ProviderSessionAssociation
	// PredecessorWorkerSessionID and SuccessorWorkerSessionID expose the
	// server-owned continuation lineage. Empty values mean that the session is
	// respectively the first or latest link in its chain.
	PredecessorWorkerSessionID string
	SuccessorWorkerSessionID   string
}

// IdentityEnvironment renders admitted nonsecret facts for this execution.
// Callers supply the public Worker Session ID, never an internal scoped key.
// Legacy metadata and unattributed roots do not invent a requester or context.
// Credentials and the selected server endpoint are bound separately by the
// execution owner; they are never derived from descriptive metadata.
func (s Session) IdentityEnvironment() []string {
	environment := []string{"YOU_WORKER_SESSION_ID=" + s.ID}
	if s.Metadata == nil {
		return environment
	}
	if requester := s.Metadata.Requester; requester != nil {
		environment = append(environment, "YOU_MESSAGE_TARGET="+requester.WorkerSessionID)
		if requester.WorkID != "" {
			environment = append(environment, "YOU_MESSAGE_TARGET_WORK_ID="+requester.WorkID)
		}
	}
	if correlation := s.Metadata.Correlation; correlation != nil {
		if correlation.WorkID != "" {
			environment = append(environment, "YOU_WORK_ID="+correlation.WorkID)
		}
		if correlation.FactorySessionID != "" {
			environment = append(environment, "YOU_FACTORY_SESSION_ID="+correlation.FactorySessionID)
		}
	}
	return environment
}

// Validate reports whether s has a non-empty stable identity, exactly one
// accepted lifecycle state, and a Result that is present if and only if
// State is StateCompleted or StateFailed and, when present, agrees with
// State. Validate is pure and does not mutate s.
func (s Session) Validate() error {
	if !validSessionID(s.ID) {
		return ErrInvalidSessionID
	}
	if !s.State.Valid() {
		return ErrInvalidState
	}
	if err := s.Metadata.Validate(); err != nil {
		return err
	}
	switch s.State {
	case StateCompleted, StateFailed:
		if s.Result == nil {
			return fmt.Errorf("%w: terminal state requires a non-nil TerminalResult", ErrInvalidTerminalResult)
		}
		if err := s.Result.Validate(); err != nil {
			return err
		}
		if (s.State == StateCompleted) != (s.Result.Outcome == TerminalOutcomeCompleted) {
			return fmt.Errorf("%w: state and TerminalResult outcome disagree", ErrInvalidTerminalResult)
		}
	default:
		if s.Result != nil {
			return fmt.Errorf("%w: non-terminal state must not carry a TerminalResult", ErrInvalidTerminalResult)
		}
	}
	if s.ProviderSessionAssociation != nil {
		if err := s.ProviderSessionAssociation.Validate(); err != nil {
			return err
		}
		if s.ProviderSessionAssociation.WorkerSessionID != s.ID {
			return fmt.Errorf("%w: provider session association worker session id disagrees", ErrInvalidProviderSessionAssociation)
		}
	}
	if err := validateLineage(s.ID, s.PredecessorWorkerSessionID, s.SuccessorWorkerSessionID); err != nil {
		return err
	}
	return nil
}

// Clone returns a detached immutable snapshot. Session is otherwise a value,
// but Result, Model, ReasoningEffort, and ProviderSessionAssociation contain
// pointers that must not alias registry-owned state across a continuation
// replay.
func (s Session) Clone() Session {
	clone := s
	clone.Metadata = s.Metadata.Clone()
	clone.Model = cloneString(s.Model)
	clone.ReasoningEffort = cloneString(s.ReasoningEffort)
	if s.Result != nil {
		result := *s.Result
		if s.Result.Cause != nil {
			cause := *s.Result.Cause
			result.Cause = &cause
		}
		clone.Result = &result
	}
	if s.ProviderSessionAssociation != nil {
		association := *s.ProviderSessionAssociation
		association.Reference = s.ProviderSessionAssociation.Reference.Clone()
		clone.ProviderSessionAssociation = &association
	}
	return clone
}

func validateLineage(id, predecessor, successor string) error {
	if predecessor != "" {
		if !validSessionID(predecessor) || predecessor == id {
			return ErrInvalidContinuationLineage
		}
	}
	if successor != "" {
		if !validSessionID(successor) || successor == id {
			return ErrInvalidContinuationLineage
		}
	}
	return nil
}

// Terminal reports whether s is currently in one of the four absorbing
// terminal states.
func (s Session) Terminal() bool {
	return s.State.Terminal()
}

func validSessionID(id string) bool {
	return strings.TrimSpace(id) != ""
}

// State is the exact eight-value Worker Session lifecycle vocabulary. No
// other value, including INTERRUPTED, is accepted anywhere W1 validates a
// state.
type State string

const (
	StateReserved   State = "RESERVED"
	StateStarting   State = "STARTING"
	StateRunning    State = "RUNNING"
	StatePaused     State = "PAUSED"
	StateCompleted  State = "COMPLETED"
	StateFailed     State = "FAILED"
	StateCanceled   State = "CANCELED"
	StateTerminated State = "TERMINATED"
)

// Valid reports whether s is one of the eight accepted lifecycle states.
func (s State) Valid() bool {
	switch s {
	case StateReserved, StateStarting, StateRunning, StatePaused, StateCompleted, StateFailed, StateCanceled, StateTerminated:
		return true
	default:
		return false
	}
}

// Terminal reports whether s is one of the four absorbing terminal states:
// COMPLETED, FAILED, CANCELED, or TERMINATED. Transition rules into or out of
// a terminal state are outside W1.
func (s State) Terminal() bool {
	switch s {
	case StateCompleted, StateFailed, StateCanceled, StateTerminated:
		return true
	default:
		return false
	}
}
