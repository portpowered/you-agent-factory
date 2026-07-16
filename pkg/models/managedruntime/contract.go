// Package managedruntime owns provider-neutral managed model runtime contracts.
package managedruntime

import "errors"

var (
	// ErrNotFound reports that a requested model is absent from configuration.
	ErrNotFound = errors.New("model not found")
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

// LifecycleState names the durable install/load position of a managed runtime.
type LifecycleState string

const (
	LifecycleStateNotInstalled  LifecycleState = "NOT_INSTALLED"
	LifecycleStateInstalling    LifecycleState = "INSTALLING"
	LifecycleStateInstalled     LifecycleState = "INSTALLED"
	LifecycleStateLoading       LifecycleState = "LOADING"
	LifecycleStateLoaded        LifecycleState = "LOADED"
	LifecycleStateNotApplicable LifecycleState = "NOT_APPLICABLE"
)

// PullOutcome classifies the source-agnostic result of an asset pull.
type PullOutcome string

const (
	PullOutcomeAlreadyPresent        PullOutcome = "ALREADY_PRESENT"
	PullOutcomeAlreadyReady          PullOutcome = "ALREADY_READY"
	PullOutcomeInstalledSuccessfully PullOutcome = "INSTALLED_SUCCESSFULLY"
	PullOutcomeSourceFetchFailed     PullOutcome = "SOURCE_FETCH_FAILED"
	PullOutcomeStillLoading          PullOutcome = "STILL_LOADING"
	PullOutcomeTimedOut              PullOutcome = "TIMED_OUT"
	PullOutcomeUnsupportedRuntime    PullOutcome = "UNSUPPORTED_RUNTIME"
)

// Locality identifies whether a model executes locally or through a remote provider.
type Locality string

const (
	LocalityLocal Locality = "LOCAL"
	LocalityCloud Locality = "CLOUD"
)

// Operation describes one provider-neutral model capability.
type Operation struct {
	Name    string
	Inputs  []OperationSlot
	Outputs []OperationSlot
}

// OperationSlot describes one named input or output of a model operation.
type OperationSlot struct {
	Name         string
	ContentTypes []string
	Required     *bool
}

// Runtime is the model-owned readiness projection consumed by service and
// transport adapters.
type Runtime struct {
	Identity            string
	ReadinessState      ReadinessState
	LifecycleState      LifecycleState
	Locality            Locality
	SupportedOperations []Operation
	Diagnostics         map[string]string
}

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
