// Package managedruntime owns provider-neutral managed model runtime contracts.
package managedruntime

import "errors"

var (
	// ErrMissing reports that invocation requires uninstalled runtime assets.
	ErrMissing = errors.New("managed runtime missing")
	// ErrLoading reports that invocation must wait for runtime preparation.
	ErrLoading = errors.New("managed runtime loading")
	// ErrFailed reports that invocation is blocked by a failed runtime.
	ErrFailed = errors.New("managed runtime failed")
	// ErrUnsupported reports that invocation targets an unsupported runtime.
	ErrUnsupported = errors.New("managed runtime unsupported")
)

// ReadinessState names the managed-runtime readiness vocabulary.
type ReadinessState string

const (
	ReadinessStateReady       ReadinessState = "READY"
	ReadinessStateMissing     ReadinessState = "MISSING"
	ReadinessStateLoading     ReadinessState = "LOADING"
	ReadinessStateFailed      ReadinessState = "FAILED"
	ReadinessStateUnsupported ReadinessState = "UNSUPPORTED"
)

// InvocationReadinessError exposes model readiness to consumers without
// coupling them to a transport-specific error representation.
type InvocationReadinessError interface {
	error
	ManagedRuntimeReadinessState() ReadinessState
}

// ReadinessStateFromError returns managed-runtime readiness carried either by
// the narrow readiness seam or by one of the canonical blocking sentinels.
func ReadinessStateFromError(err error) (ReadinessState, bool) {
	var readinessErr InvocationReadinessError
	if errors.As(err, &readinessErr) {
		return readinessErr.ManagedRuntimeReadinessState(), true
	}
	switch {
	case errors.Is(err, ErrMissing):
		return ReadinessStateMissing, true
	case errors.Is(err, ErrLoading):
		return ReadinessStateLoading, true
	case errors.Is(err, ErrFailed):
		return ReadinessStateFailed, true
	case errors.Is(err, ErrUnsupported):
		return ReadinessStateUnsupported, true
	default:
		return "", false
	}
}
