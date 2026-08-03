package workersessions

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
