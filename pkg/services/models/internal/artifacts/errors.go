package artifacts

import (
	"errors"
	"fmt"
)

// FailureKind identifies the actionable reason an artifact manifest or
// selection request was rejected.
type FailureKind string

const (
	FailureMalformedManifest       FailureKind = "malformed_manifest"
	FailureInvalidDigest           FailureKind = "invalid_digest"
	FailureInvalidSize             FailureKind = "invalid_size"
	FailureUnsafeLocation          FailureKind = "unsafe_location"
	FailureUnknownBackend          FailureKind = "unknown_backend"
	FailureUnsupportedPlatform     FailureKind = "unsupported_platform"
	FailureIncompatibleAccelerator FailureKind = "incompatible_accelerator"
	FailureIncompatibleProtocol    FailureKind = "incompatible_protocol"
	FailureMissingMatch            FailureKind = "missing_match"
	FailureDuplicateMatch          FailureKind = "duplicate_match"
	FailureDuplicateArtifact       FailureKind = "duplicate_artifact"
	FailureInvalidSelection        FailureKind = "invalid_selection"
	FailureIntegrityMismatch       FailureKind = "integrity_mismatch"
)

var (
	// ErrManifestMalformed reports invalid JSON, schema, or provenance facts.
	ErrManifestMalformed = errors.New("artifact manifest is malformed")
	// ErrInvalidDigest reports a checksum that is not a lowercase SHA-256.
	ErrInvalidDigest = errors.New("artifact digest is invalid")
	// ErrInvalidSize reports a missing, zero, or negative artifact size.
	ErrInvalidSize = errors.New("artifact size is invalid")
	// ErrUnsafeLocation reports an archive URL outside the pinned publication.
	ErrUnsafeLocation = errors.New("artifact location is not approved")
	// ErrUnknownBackend reports a backend outside the supported closed set.
	ErrUnknownBackend = errors.New("artifact backend is unknown")
	// ErrUnsupportedPlatform reports a target outside the supported closed set.
	ErrUnsupportedPlatform = errors.New("artifact platform is unsupported")
	// ErrIncompatibleAccelerator reports a request or target mismatch.
	ErrIncompatibleAccelerator = errors.New("artifact accelerator is incompatible")
	// ErrIncompatibleProtocol reports a protocol revision mismatch.
	ErrIncompatibleProtocol = errors.New("artifact protocol revision is incompatible")
	// ErrMissingMatch reports that no compatible artifact exists.
	ErrMissingMatch = errors.New("no compatible backend artifact was found")
	// ErrDuplicateMatch reports more than one compatible artifact.
	ErrDuplicateMatch = errors.New("more than one compatible backend artifact was found")
	// ErrDuplicateArtifact reports duplicate artifact identity in one manifest.
	ErrDuplicateArtifact = errors.New("artifact manifest contains duplicate identity")
	// ErrInvalidSelection reports a selection request with missing or malformed fields.
	ErrInvalidSelection = errors.New("artifact selection request is invalid")
	// ErrIntegrityMismatch reports fake or downloaded bytes that do not match the descriptor.
	ErrIntegrityMismatch = errors.New("artifact integrity verification failed")
)

// Failure is the typed, actionable error returned by this Models-private
// boundary. ArtifactError, CapabilityError, and Error are aliases kept for
// callers that want to classify the same failure with a more specific name.
type Failure struct {
	Kind   FailureKind
	Field  string
	Value  string
	Detail string
	cause  error
}

// ArtifactError is an alias for the typed failure used while decoding facts.
type ArtifactError = Failure

// CapabilityError is an alias for the typed failure used while selecting facts.
type CapabilityError = Failure

// Error is a concise alias for callers that do not need to distinguish the
// decoding and selection phases.
type Error = Failure

func (e *Failure) Error() string {
	if e == nil {
		return ""
	}
	message := string(e.Kind)
	if e.Field != "" {
		message += " for " + e.Field
	}
	if e.Value != "" {
		message += " (" + e.Value + ")"
	}
	if e.Detail != "" {
		message += ": " + e.Detail
	}
	return message
}

func (e *Failure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func failure(kind FailureKind, field, value, detail string) error {
	return &Failure{
		Kind:   kind,
		Field:  field,
		Value:  value,
		Detail: detail,
		cause:  failureSentinel(kind),
	}
}

func failuref(kind FailureKind, field, value, format string, args ...any) error {
	return failure(kind, field, value, fmt.Sprintf(format, args...))
}

func failureSentinel(kind FailureKind) error {
	switch kind {
	case FailureMalformedManifest:
		return ErrManifestMalformed
	case FailureInvalidDigest:
		return ErrInvalidDigest
	case FailureInvalidSize:
		return ErrInvalidSize
	case FailureUnsafeLocation:
		return ErrUnsafeLocation
	case FailureUnknownBackend:
		return ErrUnknownBackend
	case FailureUnsupportedPlatform:
		return ErrUnsupportedPlatform
	case FailureIncompatibleAccelerator:
		return ErrIncompatibleAccelerator
	case FailureIncompatibleProtocol:
		return ErrIncompatibleProtocol
	case FailureMissingMatch:
		return ErrMissingMatch
	case FailureDuplicateMatch:
		return ErrDuplicateMatch
	case FailureDuplicateArtifact:
		return ErrDuplicateArtifact
	case FailureInvalidSelection:
		return ErrInvalidSelection
	case FailureIntegrityMismatch:
		return ErrIntegrityMismatch
	default:
		return nil
	}
}
