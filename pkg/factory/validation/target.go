package validation

import (
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

// Severity classifies how a validation target should be treated by callers.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityHint    Severity = "hint"
)

// SubjectType identifies the factory component a target refers to.
type SubjectType string

const (
	SubjectTypeFactory     SubjectType = "FACTORY"
	SubjectTypeWorkstation SubjectType = "WORKSTATION"
	SubjectTypeWorkType    SubjectType = "WORK_TYPE"
	SubjectTypeWorkState   SubjectType = "WORK_STATE"
	SubjectTypeWorker      SubjectType = "WORKER"
	SubjectTypeResource    SubjectType = "RESOURCE"
	SubjectTypeRoute       SubjectType = "ROUTE"
)

// SubjectLocation identifies the factory-domain location within a subject.
type SubjectLocation string

const (
	SubjectLocationOnRejection SubjectLocation = "ON_REJECTION"
	SubjectLocationOnFailure   SubjectLocation = "ON_FAILURE"
	SubjectLocationOutputs     SubjectLocation = "OUTPUTS"
	SubjectLocationInputs      SubjectLocation = "INPUTS"
	SubjectLocationStates      SubjectLocation = "STATES"
	SubjectLocationTerminal    SubjectLocation = "TERMINAL"
	SubjectLocationReference   SubjectLocation = "REFERENCE"
	SubjectLocationDefinition  SubjectLocation = "DEFINITION"
)

// Subject identifies the affected factory component without UI-specific identifiers.
type Subject struct {
	Type     SubjectType     `json:"type"`
	ID       string          `json:"id"`
	Location SubjectLocation `json:"location"`
}

// Target is one canonical factory validation issue.
type Target struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Subject  Subject  `json:"subject"`
	// Path is a legacy config-style location retained for config delegation and adapters.
	Path string `json:"-"`
}

// Result aggregates canonical validation targets for one factory definition.
type Result struct {
	Targets []Target
}

// HasTargets reports whether the result contains one or more validation targets.
func (r Result) HasTargets() bool {
	return len(r.Targets) > 0
}

// BlockingTargets returns topology and other error-severity targets that should
// block save and runtime load. Recoverable layout warnings are excluded.
func (r Result) BlockingTargets() []Target {
	if len(r.Targets) == 0 {
		return nil
	}
	blocking := make([]Target, 0, len(r.Targets))
	for _, target := range r.Targets {
		if target.Severity != SeverityError {
			continue
		}
		if IsLayoutTargetCode(target.Code) && !isBlockingLayoutTarget(target) {
			continue
		}
		blocking = append(blocking, target)
	}
	return blocking
}

// HasBlockingTargets reports whether the result contains save-blocking targets.
func (r Result) HasBlockingTargets() bool {
	return len(r.BlockingTargets()) > 0
}

// DefaultTopologyValidationMessage is the operator-facing save rejection message
// when callers do not supply a custom topology validation message.
const DefaultTopologyValidationMessage = "Factory topology contains invalid graph references."

func isBlockingLayoutTarget(target Target) bool {
	return target.Code == CodeLayoutEmptyStateUnknownNodeReference ||
		target.Code == CodeLayoutInvalidValue ||
		target.Code == CodeLayoutInvalidGeometry ||
		target.Code == CodeLayoutImageBudgetExceeded ||
		(target.Code == CodeLayoutUnknownNodeReference && interfaces.IsBundledFileGraphNodeID(target.Subject.ID))
}
