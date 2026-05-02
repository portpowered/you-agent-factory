package workers

import "github.com/portpowered/infinite-you/pkg/interfaces"

// PostResultAction determines what the engine should do after processing a work result.
type PostResultAction string

const (
	// ActionAdvance routes tokens normally via the appropriate arc set (output/rejection/failure).
	// This is the behavior of the "standard" workstation type.
	ActionAdvance PostResultAction = "advance"
	// ActionRepeat keeps the token in its input place and re-fires the transition
	// on the next tick. Used by the "repeater" workstation type.
	ActionRepeat PostResultAction = "repeat"
)

// WorkstationTypeStrategy defines the execution semantics for a workstation type.
// Implementations determine how results are routed after a worker completes.
//
// Adding a new workstation type requires:
//  1. Implementing this interface
//  2. Registering the implementation with NewWorkstationTypeRegistry or Registry.Register
//
// No engine internals need to be modified.
type WorkstationTypeStrategy interface {
	// Kind returns the workstation kind this strategy handles.
	Kind() interfaces.WorkstationKind

	// HandleResult examines a work result and returns the action the engine
	// should take. For "standard" workstations this always returns ActionAdvance.
	HandleResult(result interfaces.WorkResult) PostResultAction
}

// StandardWorkstationType implements the default fire-once behavior.
// Tokens are always routed via the appropriate arc set regardless of outcome.
type StandardWorkstationType struct{}

// Kind returns WorkstationKindStandard.
func (s *StandardWorkstationType) Kind() interfaces.WorkstationKind {
	return interfaces.WorkstationKindStandard
}

// HandleResult always returns ActionAdvance — standard workstations route every result.
func (s *StandardWorkstationType) HandleResult(_ interfaces.WorkResult) PostResultAction {
	return ActionAdvance
}

// WorkstationTypeRegistry maps workstation kinds to their strategy implementations.
// The registry is pre-populated with the "standard" type and can be extended
// with additional types via Register.
type WorkstationTypeRegistry struct {
	strategies map[interfaces.WorkstationKind]WorkstationTypeStrategy
}

// RepeaterWorkstationType re-fires a transition after a non-terminal result.
// CONTINUE outcomes keep the token in its input place (ActionRepeat) so the
// worker is invoked again on the next tick. ACCEPTED, REJECTED, and FAILED
// outcomes advance normally via the appropriate arc set.
type RepeaterWorkstationType struct{}

// Kind returns WorkstationKindRepeater.
func (r *RepeaterWorkstationType) Kind() interfaces.WorkstationKind {
	return interfaces.WorkstationKindRepeater
}

// HandleResult returns ActionRepeat for CONTINUE outcomes (the worker signals
// "not done yet") and ActionAdvance for ACCEPTED, REJECTED, or FAILED.
func (r *RepeaterWorkstationType) HandleResult(result interfaces.WorkResult) PostResultAction {
	if result.Outcome == interfaces.OutcomeContinue {
		return ActionRepeat
	}
	return ActionAdvance
}

// CronWorkstationType identifies service-triggered cron workstations. Cron
// ticks submit through the service ingress; once dispatched, results advance
// through the normal arc sets.
type CronWorkstationType struct{}

// Kind returns WorkstationKindCron.
func (c *CronWorkstationType) Kind() interfaces.WorkstationKind {
	return interfaces.WorkstationKindCron
}

// HandleResult routes cron execution results through normal arc handling.
func (c *CronWorkstationType) HandleResult(_ interfaces.WorkResult) PostResultAction {
	return ActionAdvance
}

// NewWorkstationTypeRegistry creates a registry pre-populated with the "standard"
// "repeater", and "cron" types.
func NewWorkstationTypeRegistry() *WorkstationTypeRegistry {
	r := &WorkstationTypeRegistry{
		strategies: make(map[interfaces.WorkstationKind]WorkstationTypeStrategy),
	}
	r.Register(&StandardWorkstationType{})
	r.Register(&RepeaterWorkstationType{})
	r.Register(&CronWorkstationType{})
	return r
}

// Register adds a workstation type strategy to the registry.
func (r *WorkstationTypeRegistry) Register(s WorkstationTypeStrategy) {
	r.strategies[s.Kind()] = s
}

// Get returns the strategy for the given kind, or (nil, false) if not registered.
func (r *WorkstationTypeRegistry) Get(kind interfaces.WorkstationKind) (WorkstationTypeStrategy, bool) {
	s, ok := r.strategies[kind]
	return s, ok
}

// IsValid returns true if the given kind is registered.
func (r *WorkstationTypeRegistry) IsValid(kind interfaces.WorkstationKind) bool {
	_, ok := r.strategies[kind]
	return ok
}

// Kinds returns all registered workstation kind names.
func (r *WorkstationTypeRegistry) Kinds() []interfaces.WorkstationKind {
	kinds := make([]interfaces.WorkstationKind, 0, len(r.strategies))
	for k := range r.strategies {
		kinds = append(kinds, k)
	}
	return kinds
}
