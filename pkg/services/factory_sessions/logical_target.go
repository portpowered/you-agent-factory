package factorysessions

import (
	"errors"
)

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
