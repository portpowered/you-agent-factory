package factorysessions

import "errors"

type LogicalTargetKind string

const (
	LogicalTargetKindDefault  LogicalTargetKind = "default"
	LogicalTargetKindNamed    LogicalTargetKind = "named"
	LogicalTargetKindProvider LogicalTargetKind = "provider"

	LogicalTargetReasonRequired        = "required"
	LogicalTargetReasonInvalidTarget   = "invalid_target"
	LogicalTargetReasonAmbiguousTarget = "ambiguous_target"
)

var (
	ErrLogicalTargetRequired  = errors.New("logical session target field is required")
	ErrLogicalTargetInvalid   = errors.New("logical session target reference is invalid")
	ErrLogicalTargetAmbiguous = errors.New("logical session target reference is ambiguous")
)

type LogicalTargetProviderBoundary struct {
	Provider string
	Kind     string
	Boundary string
}

// LogicalTargetResolveSymlinks is the exact filesystem effect used to
// canonicalize an existing Factory Session folder. Callers inject the effect;
// normalization never selects a host-filesystem implementation.
type LogicalTargetResolveSymlinks func(string) (string, error)

// LogicalTargetReferenceNormalizer is the exact Factory Sessions operation
// consumed by representation boundaries that need a canonical target.
type LogicalTargetReferenceNormalizer func(
	backendScopeID string,
	folderPath string,
	ref TargetRef,
) (CanonicalLogicalTargetReference, error)

type CanonicalLogicalTargetReference struct {
	BackendScopeID string
	FolderPath     string
	Kind           LogicalTargetKind
	NamedTarget    string
	Provider       *LogicalTargetProviderBoundary
}

// --- merged from identity_contract.go ---

// IdentityNormalizeRequest is the plain root request for default, named, and
// folder-scoped logical target resolution. Peers submit already-decoded
// backend scope, folder, and target selection; private identity subservice
// interfaces are not part of this published slice.
type IdentityNormalizeRequest struct {
	BackendScopeID string
	FolderPath     string
	Target         TargetRef
}

// IdentityNormalizeProviderRequest is the plain root request for provider-backed
// logical target resolution. Boundary values identify stable provider workspace
// or account scope and must not carry credentials.
type IdentityNormalizeProviderRequest struct {
	BackendScopeID string
	FolderPath     string
	Boundary       LogicalTargetProviderBoundary
}

// ResolvedIdentity is the plain root result of identity/target resolution.
// Equivalent normalized targets share LogicalSessionKeyID within one backend
// scope; peers consume this detached value without nested identity imports.
type ResolvedIdentity struct {
	Reference           CanonicalLogicalTargetReference
	LogicalSessionKeyID string
	RuntimeTarget       RuntimeLogicalTarget
}

// ErrLogicalTargetNotFound reports that no logical session target matched the
// requested identity within the scoped backend/folder. Peers distinguish it from
// malformed (ErrLogicalTargetInvalid) and ambiguous (ErrLogicalTargetAmbiguous)
// outcomes with errors.Is. ErrLogicalTargetRequired remains the missing-field
// sentinel for incomplete identity requests.
var ErrLogicalTargetNotFound = errors.New("logical session target was not found")
