package workersessions

import (
	"context"
	"errors"
)

// Service is the W1 Worker Session identity and registry foundation: stable
// identity reservation and immutable deterministic inspection. Supervision,
// Start/StartTurn, Events publication, Runtime and Provider Session
// association, Pause/Resume/Cancel/Terminate controls, persistence, and
// transport behavior are later ACP Worker Events slices (W2-W7) and are not
// exposed here. Later slices land as additive methods on this same named
// interface; W1 does not publish placeholder methods for them.
type Service interface {
	// Reserve validates req and, when req.ID is not already registered,
	// stores a new session in StateReserved and returns its snapshot.
	// Reserving an identity that already exists returns
	// ErrSessionAlreadyExists and leaves the existing session unchanged; this
	// is the one deterministic outcome for duplicate reservation.
	Reserve(ctx context.Context, req ReserveRequest) (Session, error)

	// Get returns the current immutable snapshot for req.ID. An invalid
	// req.ID returns ErrInvalidSessionID; a valid but unregistered req.ID
	// returns the distinguishable ErrSessionNotFound.
	Get(ctx context.Context, req GetRequest) (Session, error)

	// List returns immutable snapshots matching req.Filter in the documented
	// deterministic order (ascending by ID), independent of insertion order
	// or concurrent access. An empty registry or filter match returns a
	// successful empty ListResult, not a not-found failure. An invalid
	// filter value returns a typed validation error and no partial result.
	List(ctx context.Context, req ListRequest) (ListResult, error)
}

// ReserveRequest asks Service to reserve one new Worker Session identity.
type ReserveRequest struct {
	ID string
}

// Validate reports whether req carries a non-empty stable identity. Validate
// is pure and does not mutate req.
func (req ReserveRequest) Validate() error {
	if !validSessionID(req.ID) {
		return ErrInvalidSessionID
	}
	return nil
}

// GetRequest asks Service to inspect one Worker Session identity.
type GetRequest struct {
	ID string
}

// Validate reports whether req carries a non-empty stable identity. Validate
// is pure and does not mutate req.
func (req GetRequest) Validate() error {
	if !validSessionID(req.ID) {
		return ErrInvalidSessionID
	}
	return nil
}

// Filter narrows List results. The zero-value Filter matches every session.
type Filter struct {
	// States restricts results to sessions whose State is in States. An
	// empty States matches every state.
	States []State
}

// Validate reports whether every state named in f.States is one of the eight
// accepted lifecycle states. Validate is pure and does not mutate f.
func (f Filter) Validate() error {
	for _, state := range f.States {
		if !state.Valid() {
			return ErrInvalidState
		}
	}
	return nil
}

// ListRequest asks Service for the deterministic filtered set of current
// Worker Session snapshots.
type ListRequest struct {
	Filter Filter
}

// Validate reports whether req.Filter is valid. Validate is pure and does not
// mutate req.
func (req ListRequest) Validate() error {
	return req.Filter.Validate()
}

// ListResult is the deterministic immutable snapshot collection returned by
// List. Mutating Sessions, or any element of it, after List returns never
// affects registry-owned state or a later List/Get result.
type ListResult struct {
	Sessions []Session
}

var (
	// ErrInvalidSessionID reports a request or session with an empty or
	// whitespace-only identity.
	ErrInvalidSessionID = errors.New("worker session: invalid session id")
	// ErrInvalidState reports a request, filter, or session naming a value
	// outside the eight accepted lifecycle states.
	ErrInvalidState = errors.New("worker session: invalid state")
	// ErrSessionAlreadyExists reports Reserve called with an identity that is
	// already registered.
	ErrSessionAlreadyExists = errors.New("worker session: already exists")
	// ErrSessionNotFound reports Get called with a valid but unregistered
	// identity.
	ErrSessionNotFound = errors.New("worker session: not found")
)
