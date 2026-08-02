package chatsessions

import "time"

// TargetEpisodeState is the lifecycle state of one immutable Target Episode.
type TargetEpisodeState string

const (
	// TargetEpisodeStateOpen means the episode accepts turn admission.
	TargetEpisodeStateOpen TargetEpisodeState = "OPEN"
	// TargetEpisodeStateClosed is terminal; the episode accepts no new turns.
	TargetEpisodeStateClosed TargetEpisodeState = "CLOSED"
)

// Validate reports whether s is one of the exactly declared
// TargetEpisodeState values. The zero value and any unknown value are
// rejected.
func (s TargetEpisodeState) Validate() error {
	switch s {
	case TargetEpisodeStateOpen, TargetEpisodeStateClosed:
		return nil
	default:
		return &InvalidTargetEpisodeStateError{State: s}
	}
}

var legalTargetEpisodeStateTransitions = map[TargetEpisodeState]map[TargetEpisodeState]bool{
	TargetEpisodeStateOpen:   {TargetEpisodeStateClosed: true},
	TargetEpisodeStateClosed: {},
}

// TransitionTargetEpisodeState validates a proposed TargetEpisodeState
// transition and returns the next state. On any invalid-value or
// invalid-transition error it returns from unchanged; from and to are never
// mutated.
func TransitionTargetEpisodeState(from, to TargetEpisodeState) (TargetEpisodeState, error) {
	if err := from.Validate(); err != nil {
		return from, err
	}
	if err := to.Validate(); err != nil {
		return from, err
	}
	if legalTargetEpisodeStateTransitions[from][to] {
		return to, nil
	}
	return from, &InvalidTargetEpisodeStateTransitionError{From: from, To: to}
}

// TargetEpisode is one immutable historical identity within a Chat Session's
// target history. Its Number and Target never change after creation; a
// target change is represented by closing the open episode and opening a new
// one with the next episode number, never by rewriting this episode's
// Target.
type TargetEpisode struct {
	Number           uint64
	State            TargetEpisodeState
	Target           ChatTargetRef
	FactorySessionID string
	StartedAt        time.Time
	ClosedAt         *time.Time
}

// CloseTargetEpisode returns ep transitioned to TargetEpisodeStateClosed with
// ClosedAt set to closedAt. Number, Target, and FactorySessionID are
// unchanged. ep itself is never mutated; on error the returned episode equals
// ep.
func CloseTargetEpisode(ep TargetEpisode, closedAt time.Time) (TargetEpisode, error) {
	next, err := TransitionTargetEpisodeState(ep.State, TargetEpisodeStateClosed)
	if err != nil {
		return ep, err
	}
	closed := ep
	closed.State = next
	closed.ClosedAt = &closedAt
	return closed, nil
}

// OpenNextTargetEpisode returns the next immutable Target Episode selecting
// target, requiring prior to already be closed. The returned episode's
// Number is exactly prior.Number+1; prior is never mutated and its Target is
// never rewritten.
func OpenNextTargetEpisode(prior TargetEpisode, target ChatTargetRef, factorySessionID string, startedAt time.Time) (TargetEpisode, error) {
	if prior.State != TargetEpisodeStateClosed {
		return TargetEpisode{}, &TargetEpisodeNotClosedError{Number: prior.Number, State: prior.State}
	}
	if err := target.Validate(); err != nil {
		return TargetEpisode{}, err
	}
	return TargetEpisode{
		Number:           prior.Number + 1,
		State:            TargetEpisodeStateOpen,
		Target:           target,
		FactorySessionID: factorySessionID,
		StartedAt:        startedAt,
	}, nil
}
