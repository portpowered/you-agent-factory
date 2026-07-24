package factory

import "errors"

// Root contract typed errors for orchestration-neutral Runtime control and
// observation. Peers branch with errors.Is; nested IMP-RUN implementations map
// concrete lifecycle failures onto these sentinels.
var (
	// ErrNotRunning indicates the targeted Factory Runtime instance is not in a
	// running state for the requested control or observation operation.
	ErrNotRunning = errors.New("factory runtime is not running")

	// ErrNotFound indicates the targeted Factory Runtime instance or work scope
	// does not exist for the requested operation.
	ErrNotFound = errors.New("factory runtime target not found")

	// ErrAlreadyStopped indicates terminate/stop was requested against an
	// instance that has already stopped.
	ErrAlreadyStopped = errors.New("factory runtime is already stopped")

	// ErrInvalidLifecycleTransition indicates the requested control operation is
	// not valid from the instance's current lifecycle state.
	ErrInvalidLifecycleTransition = errors.New("factory runtime invalid lifecycle transition")

	// ErrInvalidObservationScope indicates the observation request asked for a
	// scope outside the published orchestration-neutral observation vocabulary.
	ErrInvalidObservationScope = errors.New("factory runtime invalid observation scope")
)
