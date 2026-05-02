package validation

import (
	"github.com/portpowered/infinite-you/pkg/factory/state"
)

// ViolationLevel indicates the severity of a validation violation.
type ViolationLevel string

const (
	// ViolationError is a fatal misconfiguration — the net cannot be executed.
	ViolationError ViolationLevel = "ERROR"
	// ViolationWarning is a potential issue that may cause runtime problems.
	ViolationWarning ViolationLevel = "WARNING"
)

// Violation describes a single validation issue found in a net definition.
type Violation struct {
	Level    ViolationLevel `json:"level"`
	Code     string         `json:"code"`
	Message  string         `json:"message"`
	Location string         `json:"location"` // human-readable location (e.g., "transition:review.input_arc:work")
}

// Validator performs static checks on a net definition.
type Validator interface {
	Validate(n *state.Net) []Violation
}

// CompositeValidator runs all registered validators and collects their violations.
type CompositeValidator struct {
	validators []Validator
}

// NewCompositeValidator creates a CompositeValidator from the given validators.
func NewCompositeValidator(validators ...Validator) *CompositeValidator {
	return &CompositeValidator{validators: validators}
}

// Validate runs all registered validators and returns the combined violations.
func (cv *CompositeValidator) Validate(n *state.Net) []Violation {
	var violations []Violation
	for _, v := range cv.validators {
		violations = append(violations, v.Validate(n)...)
	}
	return violations
}
