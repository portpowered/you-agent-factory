package factorycontracts

import (
	"context"
	"fmt"
	"strings"
)

// ValidationSeverity classifies how a Factory Definition validation target
// should be treated by callers.
type ValidationSeverity string

const (
	ValidationSeverityError   ValidationSeverity = "error"
	ValidationSeverityWarning ValidationSeverity = "warning"
	ValidationSeverityHint    ValidationSeverity = "hint"
)

// ValidationSubjectType identifies the Factory Definition component a target
// refers to.
type ValidationSubjectType string

const (
	ValidationSubjectTypeFactory     ValidationSubjectType = "FACTORY"
	ValidationSubjectTypeWorkstation ValidationSubjectType = "WORKSTATION"
	ValidationSubjectTypeWorkType    ValidationSubjectType = "WORK_TYPE"
	ValidationSubjectTypeWorkState   ValidationSubjectType = "WORK_STATE"
	ValidationSubjectTypeWorker      ValidationSubjectType = "WORKER"
	ValidationSubjectTypeResource    ValidationSubjectType = "RESOURCE"
	ValidationSubjectTypeRoute       ValidationSubjectType = "ROUTE"
)

// ValidationSubjectLocation identifies the Factory Definition location within
// a validation subject.
type ValidationSubjectLocation string

const (
	ValidationSubjectLocationOnRejection ValidationSubjectLocation = "ON_REJECTION"
	ValidationSubjectLocationOnFailure   ValidationSubjectLocation = "ON_FAILURE"
	ValidationSubjectLocationOutputs     ValidationSubjectLocation = "OUTPUTS"
	ValidationSubjectLocationInputs      ValidationSubjectLocation = "INPUTS"
	ValidationSubjectLocationStates      ValidationSubjectLocation = "STATES"
	ValidationSubjectLocationTerminal    ValidationSubjectLocation = "TERMINAL"
	ValidationSubjectLocationReference   ValidationSubjectLocation = "REFERENCE"
	ValidationSubjectLocationDefinition  ValidationSubjectLocation = "DEFINITION"
)

// ValidationSubject identifies the affected Factory Definition component.
type ValidationSubject struct {
	Type     ValidationSubjectType     `json:"type"`
	ID       string                    `json:"id"`
	Location ValidationSubjectLocation `json:"location"`
}

// ValidationTarget is one canonical Factory Definition validation issue.
type ValidationTarget struct {
	Code     string             `json:"code"`
	Severity ValidationSeverity `json:"severity"`
	Message  string             `json:"message"`
	Subject  ValidationSubject  `json:"subject"`
	// Path is retained for config delegation and compatibility adapters.
	Path string `json:"-"`
}

const (
	ValidationCodeFactorySessionField                    = "factory.session.field"
	ValidationCodeFactorySessionTarget                   = "factory.session.target"
	ValidationCodeFactoryPayloadInvalid                  = "factory.payload.invalid"
	ValidationCodeFactoryNameInvalid                     = "factory.name.invalid"
	ValidationCodeFactoryVersionStale                    = "factory.version.stale"
	ValidationCodeFactoryRuntimeNotIdle                  = "factory.runtime.notIdle"
	ValidationCodeLayoutUnknownNodeReference             = "factory.layout.unknownNodeReference"
	ValidationCodeWorkerWorkstationBehaviorCompatibility = "workstation-worker-behavior-compatibility"
)

// ValidationProfile selects the depth of Factory Definition validation.
type ValidationProfile string

const (
	ValidationProfileTopology   ValidationProfile = "topology"
	ValidationProfilePrePersist ValidationProfile = "pre_persist"
)

func ResolveValidationProfile(profile ValidationProfile) ValidationProfile {
	if profile == "" {
		return ValidationProfileTopology
	}
	return profile
}

// SubmittedDefinitionTaxonomy is the lossless, representation-neutral taxonomy
// carried across a public definition boundary. Mapping adapters copy these
// values; Factory Definitions alone decides whether a pairing is valid.
type SubmittedDefinitionTaxonomy struct {
	Workers      []SubmittedWorkerTaxonomy
	Workstations []SubmittedWorkstationTaxonomy
}

type SubmittedWorkerTaxonomy struct {
	Name string
	Type string
}

type SubmittedWorkstationTaxonomy struct {
	Name     string
	Type     string
	Behavior WorkstationKind
	Worker   string
	Index    int
}

// DefinitionValidationRequest is the complete input to the Factory
// Definitions-owned validation operation. Representation adapters map public
// payloads into detached values and never contribute policy findings.
type DefinitionValidationRequest struct {
	Profile                ValidationProfile
	Config                 *FactoryConfig
	CanonicalPayload       []byte
	CanonicalFactoryLoader CanonicalFactoryJSONLoader
	WorkstationLoader      WorkstationLoader
	WorkflowSourceReader   WorkflowSourceReader
	SubmittedTaxonomy      SubmittedDefinitionTaxonomy
}

// DefinitionValidationOperation owns validation-profile selection, canonical
// load checks, blocking-load ordering, and topology validation. Transports map
// requests and invoke this one operation.
type DefinitionValidationOperation interface {
	ValidateDefinition(context.Context, DefinitionValidationRequest) (ValidationResult, error)
}

// EditableFactoryValidationRequestMapper converts a detached snapshot into the
// pre-persist request consumed by Factory Definitions. It is a representation
// adapter only and must not invoke validation.
type EditableFactoryValidationRequestMapper func(
	*FactorySnapshot,
	WorkstationLoader,
) (DefinitionValidationRequest, error)

// SubmittedDefinitionValidationRequest is the representation-neutral input to
// the explicit customer validation operation. Its topology profile is fixed by
// Factory Definitions and is not selectable by a transport.
type SubmittedDefinitionValidationRequest struct {
	Config               *FactoryConfig
	WorkflowSourceReader WorkflowSourceReader
	Taxonomy             SubmittedDefinitionTaxonomy
}

// SubmittedDefinitionValidationOperation is the exact Factory Definitions
// role exposed to validation endpoints.
type SubmittedDefinitionValidationOperation interface {
	ValidateSubmittedDefinition(context.Context, SubmittedDefinitionValidationRequest) (ValidationResult, error)
}

// EffectiveDefinitionValidationRequest is the complete effective definition
// input used by prompt-driven invocation. Factory Definitions fixes the
// required-default-work-type policy for this operation.
type EffectiveDefinitionValidationRequest struct {
	Config               *FactoryConfig
	WorkflowSourceReader WorkflowSourceReader
}

// EffectiveDefinitionValidationOperation is the exact Factory Definitions
// role exposed to callers that execute an already-loaded effective definition.
type EffectiveDefinitionValidationOperation interface {
	ValidateEffectiveDefinition(context.Context, EffectiveDefinitionValidationRequest) (ValidationResult, error)
}

// WorkflowSourceReader resolves file-backed JavaScript workflow source during
// Factory Definition validation.
type WorkflowSourceReader interface {
	ReadWorkflowSource(sourceRef string) (string, error)
}

// RequiredToolCheckResult captures the availability result for one declarative
// external tool dependency.
type RequiredToolCheckResult struct {
	ResolvedPath string
	FailureKind  RequiredToolFailureKind
	Err          error
}

// RequiredToolFailureKind classifies the source of a required-tool failure.
type RequiredToolFailureKind string

const (
	RequiredToolFailureKindNone         RequiredToolFailureKind = ""
	RequiredToolFailureKindMissing      RequiredToolFailureKind = "missing"
	RequiredToolFailureKindVersionProbe RequiredToolFailureKind = "version-probe"
)

// RequiredToolChecker verifies declarative external tool dependencies.
type RequiredToolChecker interface {
	Check(tool RequiredToolConfig) RequiredToolCheckResult
}

// OrchestratorDefinitionValidator validates runtime-owned orchestrator
// semantics while returning Factory Definition-owned targets.
type OrchestratorDefinitionValidator interface {
	ValidateJavaScriptFactoryDefinition(
		context.Context,
		*FactoryOrchestratorJavaScriptConfig,
		WorkflowSourceReader,
	) []ValidationTarget
}

// Validator is the public Factory Definition validation boundary.
type Validator interface {
	Validate(
		context.Context,
		*FactoryConfig,
		WorkflowSourceReader,
	) ValidationResult
	ValidateBlockingLoad(context.Context, *FactoryConfig) ValidationResult
	ValidateTopology(
		context.Context,
		*FactoryConfig,
		RequiredToolChecker,
	) TopologyValidationResult
	WorkerWorkstationBehaviorCompatibility(
		context.Context,
		*FactoryConfig,
	) []ValidationTarget
	WorkTypeHandlingBehavior(
		context.Context,
		*FactoryConfig,
		bool,
	) []ValidationTarget
	PruneLayout(
		context.Context,
		*FactoryConfig,
		PendingFactoryGraphTopology,
	) ValidationResult
}

// TopologyFinding is one canonical operational or structural configuration
// finding produced by Factory Definitions topology validation.
type TopologyFinding struct {
	Severity ValidationSeverity
	Path     string
	Message  string
	Rule     string
}

// TopologyValidationResult aggregates canonical topology findings.
type TopologyValidationResult struct {
	Findings []TopologyFinding
}

func (r TopologyValidationResult) HasErrors() bool {
	for _, finding := range r.Findings {
		if finding.Severity == ValidationSeverityError {
			return true
		}
	}
	return false
}

func (r TopologyValidationResult) Errors() []TopologyFinding {
	var errors []TopologyFinding
	for _, finding := range r.Findings {
		if finding.Severity == ValidationSeverityError {
			errors = append(errors, finding)
		}
	}
	return errors
}

func (r TopologyValidationResult) Error() string {
	errors := r.Errors()
	if len(errors) == 0 {
		return ""
	}
	var message strings.Builder
	fmt.Fprintf(&message, "validation failed: %d errors", len(errors))
	for _, finding := range errors {
		fmt.Fprintf(
			&message,
			"\n- [%s] %s: %s",
			finding.Rule,
			finding.Path,
			finding.Message,
		)
	}
	return message.String()
}

// ValidationResult aggregates canonical validation targets.
type ValidationResult struct {
	Targets []ValidationTarget
}

func (r ValidationResult) HasTargets() bool { return len(r.Targets) > 0 }

func (r ValidationResult) BlockingTargets() []ValidationTarget {
	if len(r.Targets) == 0 {
		return nil
	}
	blocking := make([]ValidationTarget, 0, len(r.Targets))
	for _, target := range r.Targets {
		if target.Severity != ValidationSeverityError {
			continue
		}
		if IsLayoutValidationCode(target.Code) &&
			!(target.Code == ValidationCodeLayoutUnknownNodeReference &&
				IsBundledFileGraphNodeID(target.Subject.ID)) {
			continue
		}
		blocking = append(blocking, target)
	}
	return blocking
}

func (r ValidationResult) HasBlockingTargets() bool {
	return len(r.BlockingTargets()) > 0
}

func IsLayoutValidationCode(code string) bool {
	return strings.HasPrefix(code, "factory.layout.")
}

const DefaultTopologyValidationMessage = "Factory topology contains invalid graph references."

// ValidationTopologyError reports save-blocking Factory validation targets.
type ValidationTopologyError struct {
	Message string
	Targets []ValidationTarget
}

func (e *ValidationTopologyError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return DefaultTopologyValidationMessage
}

func NewValidationTopologyError(message string, targets []ValidationTarget) *ValidationTopologyError {
	if message == "" {
		message = DefaultTopologyValidationMessage
	}
	return &ValidationTopologyError{
		Message: message,
		Targets: append([]ValidationTarget(nil), targets...),
	}
}

func FormFactoryPayloadValidationTarget() ValidationTarget {
	return definitionValidationTarget(
		ValidationCodeFactoryPayloadInvalid,
		"Factory request payload is invalid.",
		"",
		ValidationSubjectLocationDefinition,
	)
}

func InvalidFactoryNameValidationTarget() ValidationTarget {
	return definitionValidationTarget(
		ValidationCodeFactoryNameInvalid,
		"Factory name must be a safe directory segment without path separators and cannot be the reserved current-factory identifier.",
		"",
		ValidationSubjectLocationDefinition,
	)
}

func StaleFactoryVersionValidationTarget() ValidationTarget {
	return definitionValidationTarget(
		ValidationCodeFactoryVersionStale,
		"Current factory definition is stale. Refresh the graph before saving.",
		"",
		ValidationSubjectLocationDefinition,
	)
}

func FactoryRuntimeNotIdleValidationTarget() ValidationTarget {
	return definitionValidationTarget(
		ValidationCodeFactoryRuntimeNotIdle,
		"Current factory runtime must be idle before activation.",
		"",
		ValidationSubjectLocationDefinition,
	)
}

func FactorySessionFieldValidationTarget(reason, field, message string) ValidationTarget {
	return definitionValidationTarget(
		ValidationCodeFactorySessionField+"."+reason,
		message,
		field,
		ValidationSubjectLocationReference,
	)
}

func FactorySessionTargetValidationTarget(reason, targetID, message string) ValidationTarget {
	return definitionValidationTarget(
		ValidationCodeFactorySessionTarget+"."+reason,
		message,
		targetID,
		ValidationSubjectLocationReference,
	)
}

func definitionValidationTarget(
	code string,
	message string,
	subjectID string,
	location ValidationSubjectLocation,
) ValidationTarget {
	return ValidationTarget{
		Code:     code,
		Severity: ValidationSeverityError,
		Message:  message,
		Subject: ValidationSubject{
			Type:     ValidationSubjectTypeFactory,
			ID:       subjectID,
			Location: location,
		},
	}
}
