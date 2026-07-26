package models

import (
	"errors"
	"fmt"
)

var (
	// ErrNotFound reports that a requested model is absent from configuration.
	ErrNotFound = errors.New("model not found")
	// ErrMissing reports that infer/local invocation requires uninstalled
	// runtime assets. Distinct from ErrLoading, ErrFailed, and ErrUnsupported.
	ErrMissing = errors.New("managed runtime missing")
	// ErrLoading reports that infer/local invocation must wait for runtime
	// preparation. Distinct from ErrMissing, ErrFailed, and ErrUnsupported.
	ErrLoading = errors.New("managed runtime loading")
	// ErrFailed reports that infer/local invocation is blocked by a failed
	// runtime. Distinct from ErrMissing, ErrLoading, and ErrUnsupported.
	ErrFailed = errors.New("managed runtime failed")
	// ErrUnsupported reports that infer/local invocation targets an unsupported
	// runtime. Distinct from ErrMissing, ErrLoading, ErrFailed, and
	// ErrUnsupportedResponseMode.
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

func cloneOperations(operations []Operation) []Operation {
	cloned := make([]Operation, len(operations))
	for i, operation := range operations {
		cloned[i] = operation
		cloned[i].Inputs = cloneOperationSlots(operation.Inputs)
		cloned[i].Outputs = cloneOperationSlots(operation.Outputs)
	}
	return cloned
}

func cloneOperationSlots(slots []OperationSlot) []OperationSlot {
	cloned := make([]OperationSlot, len(slots))
	for i, slot := range slots {
		cloned[i] = slot
		cloned[i].ContentTypes = append([]string(nil), slot.ContentTypes...)
		if slot.Required != nil {
			required := *slot.Required
			cloned[i].Required = &required
		}
	}
	return cloned
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

// Clone returns detached readiness facts safe for a peer to retain or mutate.
func (runtime Runtime) Clone() Runtime {
	runtime.SupportedOperations = cloneOperations(runtime.SupportedOperations)
	runtime.Diagnostics = cloneStringMap(runtime.Diagnostics)
	return runtime
}

// InvocationError carries managed-runtime readiness context without exposing a
// transport-specific projection or error type.
type InvocationError struct {
	Identity       string
	ReadinessState ReadinessState
	LifecycleState LifecycleState
	Cause          error
}

func (e *InvocationError) Error() string {
	if e == nil {
		return ""
	}
	action := invocationAction(e.ReadinessState)
	if action == "" {
		action = "resolve managed runtime readiness before invoking"
	}
	return fmt.Sprintf(
		"managed runtime %q readiness is %s (lifecycle %s): %s",
		e.Identity,
		e.ReadinessState,
		e.LifecycleState,
		action,
	)
}

func (e *InvocationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ManagedRuntimeReadinessState exposes readiness through the narrow model seam.
func (e *InvocationError) ManagedRuntimeReadinessState() ReadinessState {
	if e == nil {
		return ""
	}
	return e.ReadinessState
}

// InvocationError classifies a model-owned runtime projection. A
// ready runtime returns nil.
func (runtime Runtime) InvocationError() error {
	var cause error
	switch runtime.ReadinessState {
	case ReadinessStateReady:
		return nil
	case ReadinessStateMissing:
		cause = ErrMissing
	case ReadinessStateLoading:
		cause = ErrLoading
	case ReadinessStateFailed:
		cause = ErrFailed
	default:
		cause = ErrUnsupported
	}
	return &InvocationError{
		Identity:       runtime.Identity,
		ReadinessState: runtime.ReadinessState,
		LifecycleState: runtime.LifecycleState,
		Cause:          cause,
	}
}

func invocationAction(readiness ReadinessState) string {
	switch readiness {
	case ReadinessStateMissing:
		return "pull or install the managed runtime before invoking"
	case ReadinessStateLoading:
		return "wait for the managed runtime to finish loading before invoking"
	case ReadinessStateFailed:
		return "resolve the managed runtime failure before invoking"
	case ReadinessStateUnsupported:
		return "use a supported managed runtime for invocation"
	default:
		return ""
	}
}

// InvocationReadinessError exposes model readiness to consumers without
// coupling them to a transport-specific error representation.
type InvocationReadinessError interface {
	error
	ManagedRuntimeReadinessState() ReadinessState
}
