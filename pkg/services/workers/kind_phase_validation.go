package workers

import "fmt"

// InvalidKindError reports that a value is not one of the twelve declared
// Kind constants. Zero, unknown, case-variant, whitespace-variant, and
// near-miss values all resolve to this error rather than being normalized.
type InvalidKindError struct {
	Value Kind
}

func (e *InvalidKindError) Error() string {
	return fmt.Sprintf("workers: kind %q is not a declared Kind value", string(e.Value))
}

// InvalidPhaseError reports that a value is not one of the six declared
// Phase constants. Zero, unknown, case-variant, whitespace-variant, and
// near-miss values all resolve to this error rather than being normalized.
type InvalidPhaseError struct {
	Value Phase
}

func (e *InvalidPhaseError) Error() string {
	return fmt.Sprintf("workers: phase %q is not a declared Phase value", string(e.Value))
}

// Validate reports whether k is exactly one of the twelve canonical Kind
// values. Validate is pure, side-effect free, and independent of any
// Kind/Phase pair policy.
func (k Kind) Validate() error {
	switch k {
	case KindSession, KindRun, KindTurn, KindMessage, KindReasoning, KindTool,
		KindFileChange, KindPlan, KindProgress, KindUsage, KindError, KindStreamGap:
		return nil
	default:
		return &InvalidKindError{Value: k}
	}
}

// Validate reports whether p is exactly one of the six canonical Phase
// values. Validate is pure, side-effect free, and independent of any
// Kind/Phase pair policy.
func (p Phase) Validate() error {
	switch p {
	case PhaseStarted, PhaseDelta, PhaseUpdated, PhaseCompleted, PhaseFailed, PhaseCanceled:
		return nil
	default:
		return &InvalidPhaseError{Value: p}
	}
}
