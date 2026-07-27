package operatorsettings

import (
	"errors"
	"fmt"
	"strings"
)

// ErrResolutionInvalidInput reports that effective-resolution inputs are
// incomplete or cannot be resolved (for example unresolved symbolic DEFAULT).
var ErrResolutionInvalidInput = errors.New("operator effective resolution input is invalid")

// ErrResolutionUnsupportedOverride reports that an override layer supplied a
// value the operator settings contract does not support.
var ErrResolutionUnsupportedOverride = errors.New("operator effective resolution override is unsupported")

// ErrResolutionConflict reports that supplied baseline or override facts
// conflict and cannot be reconciled without mutating the operator document.
var ErrResolutionConflict = errors.New("operator effective resolution conflict")

// ResolutionFailureKind classifies effective-resolution failures peers can
// branch on with errors.Is / errors.As.
type ResolutionFailureKind string

const (
	ResolutionFailureKindInvalidInput          ResolutionFailureKind = "invalid_input"
	ResolutionFailureKindUnsupportedOverride   ResolutionFailureKind = "unsupported_override"
	ResolutionFailureKindConflict              ResolutionFailureKind = "conflict"
)

// ResolutionFailure retains normalized effective-resolution failure facts
// without exposing storage, codec, or lifecycle construction ports.
type ResolutionFailure struct {
	Kind    ResolutionFailureKind
	Message string
	Field   string
}

func (failure ResolutionFailure) Error() string {
	message := strings.TrimSpace(failure.Message)
	field := strings.TrimSpace(failure.Field)
	switch {
	case message != "" && field != "":
		return fmt.Sprintf("%s: %s (%s)", sentinelForResolutionFailureKind(failure.Kind).Error(), message, field)
	case message != "":
		return fmt.Sprintf("%s: %s", sentinelForResolutionFailureKind(failure.Kind).Error(), message)
	case field != "":
		return fmt.Sprintf("%s (%s)", sentinelForResolutionFailureKind(failure.Kind).Error(), field)
	default:
		return sentinelForResolutionFailureKind(failure.Kind).Error()
	}
}

func (failure ResolutionFailure) Unwrap() error {
	return sentinelForResolutionFailureKind(failure.Kind)
}

func sentinelForResolutionFailureKind(kind ResolutionFailureKind) error {
	switch kind {
	case ResolutionFailureKindInvalidInput:
		return ErrResolutionInvalidInput
	case ResolutionFailureKindUnsupportedOverride:
		return ErrResolutionUnsupportedOverride
	case ResolutionFailureKindConflict:
		return ErrResolutionConflict
	default:
		return ErrResolutionInvalidInput
	}
}

// EffectivePrecedenceChain describes the operator default precedence order
// for diagnostics on effective selections.
const EffectivePrecedenceChain = "file < env < flag"

// EffectiveLayerSource identifies which precedence layer supplied one effective
// default field after resolution.
type EffectiveLayerSource string

const (
	EffectiveLayerSourceFile EffectiveLayerSource = "file"
	EffectiveLayerSourceEnv  EffectiveLayerSource = "env"
	EffectiveLayerSourceFlag EffectiveLayerSource = "flag"
)

// EffectiveOverrideFacts carries invocation or environment override values as
// detached plain facts. Empty fields are treated as unset overrides.
type EffectiveOverrideFacts struct {
	WorkerModelProvider string
	WorkerModel         string
}

// Clone returns a detached override-facts copy.
func (facts EffectiveOverrideFacts) Clone() EffectiveOverrideFacts {
	return facts
}

// EffectiveSelection is the immutable effective operator default selection
// peers consume from effective resolution without owning precedence rules.
type EffectiveSelection struct {
	WorkerModelProvider       string
	WorkerModel               string
	WorkerModelProviderSource EffectiveLayerSource
	WorkerModelSource         EffectiveLayerSource
	ConfigPath                string
}

// Clone returns a detached effective-selection copy.
func (selection EffectiveSelection) Clone() EffectiveSelection {
	return selection
}

// ResolveEffectiveRequest asks for an effective selection from detached
// document baseline and override facts. Resolution does not mutate the operator
// document; document mutation remains on document operations.
type ResolveEffectiveRequest struct {
	DocumentBaseline         DocumentDefaults
	ExpectedDocumentBaseline *DocumentDefaults
	EnvironmentOverrides     EffectiveOverrideFacts
	InvocationOverrides      EffectiveOverrideFacts
	ConfigPath                 string
}

// Validate checks request fields whose validity does not depend on resolution
// state. Baseline mismatch against ExpectedDocumentBaseline fails before
// precedence resolution.
func (request ResolveEffectiveRequest) Validate() error {
	if request.ExpectedDocumentBaseline == nil {
		return nil
	}
	expected := request.ExpectedDocumentBaseline
	if expected.WorkerModelProvider != request.DocumentBaseline.WorkerModelProvider ||
		expected.WorkerModel != request.DocumentBaseline.WorkerModel {
		return ResolutionFailure{
			Kind:    ResolutionFailureKindConflict,
			Message: "document baseline mismatch",
			Field:   "documentBaseline",
		}
	}
	return nil
}

// ResolveEffectiveResult is the detached outcome of one effective-resolution
// operation.
type ResolveEffectiveResult struct {
	Selection EffectiveSelection
}
