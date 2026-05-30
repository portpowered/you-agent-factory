package validation

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
