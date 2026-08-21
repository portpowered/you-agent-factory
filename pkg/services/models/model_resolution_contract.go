package models

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrModelReferenceInvalid classifies a source reference that cannot be
	// interpreted as a configured name or supported local/Hugging Face source.
	ErrModelReferenceInvalid = errors.New("model reference is invalid")
	// ErrModelReferenceUnknown classifies an otherwise symbolic name that is
	// not present in the effective model catalog.
	ErrModelReferenceUnknown = errors.New("model reference name is unknown")
	// ErrModelRevisionUnresolved classifies a Hugging Face source whose revision
	// was not proven immutable before acquisition.
	ErrModelRevisionUnresolved = errors.New("model source revision is not immutable")
	// ErrModelConfigurationInvalid classifies one invalid operator model entry.
	ErrModelConfigurationInvalid = errors.New("model configuration is invalid")
)

// ModelReferenceSourceKind identifies the safe source provenance of a
// resolved reference. It intentionally does not expose a cache or host path.
type ModelReferenceSourceKind string

const (
	ModelReferenceSourceNamed       ModelReferenceSourceKind = "NAMED"
	ModelReferenceSourceLocalPath   ModelReferenceSourceKind = "LOCAL_PATH"
	ModelReferenceSourceFileURI     ModelReferenceSourceKind = "FILE_URI"
	ModelReferenceSourceHuggingFace ModelReferenceSourceKind = "HUGGING_FACE"
)

// ModelReferenceProvenance is the detached, safe provenance of one model
// reference. Local paths are represented only by kind and a stable local
// label; the original path never crosses the Models boundary.
type ModelReferenceProvenance struct {
	Kind              ModelReferenceSourceKind
	SourceKind        ModelReferenceSourceKind
	Name              string
	Owner             string
	Repository        string
	File              string
	Revision          string
	ImmutableRevision string
}

// Clone returns a detached provenance value.
func (provenance ModelReferenceProvenance) Clone() ModelReferenceProvenance {
	return provenance
}

// ResolvedModelReference is the safe result of resolving a model reference.
// Runtime readiness is a pre-acquisition projection and therefore does not
// imply that weights or a backend have been loaded.
type ResolvedModelReference struct {
	Definition ModelDefinition
	Provenance ModelReferenceProvenance
	Readiness  ReadinessState
}

// Clone returns a detached resolved reference.
func (resolved ResolvedModelReference) Clone() ResolvedModelReference {
	resolved.Definition = resolved.Definition.Clone()
	resolved.Provenance = resolved.Provenance.Clone()
	return resolved
}

// ResolveModelReferenceRequest identifies one scoped reference. Scope-owned
// operator overlays are captured when the runtime scope opens.
type ResolveModelReferenceRequest struct {
	Scope     RuntimeScopeRef
	Reference ModelReference
}

// Validate checks fields that do not require scope or catalog state.
func (request ResolveModelReferenceRequest) Validate() error {
	if request.Scope.IsZero() {
		return ErrRuntimeScopeInvalid
	}
	if request.Reference.IsZero() {
		return &InvocationFailure{
			Class:   InvocationFailureClassInvalidModelReference,
			Message: "model name or source reference is required",
			Cause:   ErrModelReferenceInvalid,
		}
	}
	return nil
}

// ResolveModelReferenceResult contains one detached effective definition.
type ResolveModelReferenceResult struct {
	Resolved ResolvedModelReference
}

// Clone returns a detached resolution result.
func (result ResolveModelReferenceResult) Clone() ResolveModelReferenceResult {
	result.Resolved = result.Resolved.Clone()
	return result
}

// ModelOverlay is the Models-owned representation of one optional operator
// configuration entry. Nil fields preserve the corresponding built-in field.
type ModelOverlay struct {
	Source     *string
	Backend    *string
	LoadPolicy *LoadPolicy
	Operations []string
}

// ModelConfiguration and OperatorModelConfiguration are descriptive aliases
// for callers that use the configuration vocabulary at different boundaries.
type ModelConfiguration = ModelOverlay
type OperatorModelConfiguration = ModelOverlay

// Clone returns a detached overlay.
func (overlay ModelOverlay) Clone() ModelOverlay {
	cloned := overlay
	if overlay.Source != nil {
		value := *overlay.Source
		cloned.Source = &value
	}
	if overlay.Backend != nil {
		value := *overlay.Backend
		cloned.Backend = &value
	}
	if overlay.LoadPolicy != nil {
		value := *overlay.LoadPolicy
		cloned.LoadPolicy = &value
	}
	cloned.Operations = append([]string(nil), overlay.Operations...)
	return cloned
}

// ModelConfigurationFailure identifies the exact operator model entry and
// field that is invalid while retaining no storage or path details.
type ModelConfigurationFailure struct {
	ModelName string
	Field     string
	Message   string
}

func (failure ModelConfigurationFailure) Error() string {
	message := strings.TrimSpace(failure.Message)
	if message == "" {
		message = "invalid value"
	}
	return fmt.Sprintf("model configuration %q field %q is invalid: %s", failure.ModelName, failure.Field, message)
}

func (failure ModelConfigurationFailure) Unwrap() error {
	return ErrModelConfigurationInvalid
}
