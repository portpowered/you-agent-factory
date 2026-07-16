package apisurface

import (
	"errors"
	"fmt"

	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ErrManagedRuntimeMissing reports that invocation requires a managed runtime
// that is not installed in the managed cache yet.
var ErrManagedRuntimeMissing = managedruntime.ErrMissing

// ErrManagedRuntimeLoading reports that invocation must wait for managed
// runtime load or preparation to finish.
var ErrManagedRuntimeLoading = managedruntime.ErrLoading

// ErrManagedRuntimeFailed reports that the managed runtime is in a failed state
// and must be recovered before invocation can proceed.
var ErrManagedRuntimeFailed = managedruntime.ErrFailed

// ErrManagedRuntimeUnsupported reports that the managed runtime identity is not
// supported for invocation in the current factory configuration.
var ErrManagedRuntimeUnsupported = managedruntime.ErrUnsupported

// ManagedRuntimeInvocationError carries managed-runtime readiness context for
// invocation surfaces without exposing backend-specific cache vocabulary.
type ManagedRuntimeInvocationError struct {
	Identity       string
	ReadinessState factoryapi.ManagedRuntimeReadinessState
	LifecycleState factoryapi.ManagedRuntimeLifecycleState
	Cause          error
}

func (e *ManagedRuntimeInvocationError) Error() string {
	if e == nil {
		return ""
	}
	action := managedRuntimeInvocationAction(e.ReadinessState)
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

func (e *ManagedRuntimeInvocationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ManagedRuntimeReadinessState exposes the domain readiness carried by this
// transport-boundary error.
func (e *ManagedRuntimeInvocationError) ManagedRuntimeReadinessState() managedruntime.ReadinessState {
	if e == nil {
		return ""
	}
	return managedruntime.ReadinessState(e.ReadinessState)
}

// InvocationErrorFromManagedRuntime maps one managed-runtime readiness projection
// into an invocation error. READY returns nil.
func InvocationErrorFromManagedRuntime(managed factoryapi.ManagedRuntime) error {
	switch managed.ReadinessState {
	case factoryapi.ManagedRuntimeReadinessStateREADY:
		return nil
	case factoryapi.ManagedRuntimeReadinessStateMISSING:
		return &ManagedRuntimeInvocationError{
			Identity:       managed.Identity,
			ReadinessState: managed.ReadinessState,
			LifecycleState: managed.LifecycleState,
			Cause:          ErrManagedRuntimeMissing,
		}
	case factoryapi.ManagedRuntimeReadinessStateLOADING:
		return &ManagedRuntimeInvocationError{
			Identity:       managed.Identity,
			ReadinessState: managed.ReadinessState,
			LifecycleState: managed.LifecycleState,
			Cause:          ErrManagedRuntimeLoading,
		}
	case factoryapi.ManagedRuntimeReadinessStateFAILED:
		return &ManagedRuntimeInvocationError{
			Identity:       managed.Identity,
			ReadinessState: managed.ReadinessState,
			LifecycleState: managed.LifecycleState,
			Cause:          ErrManagedRuntimeFailed,
		}
	default:
		return &ManagedRuntimeInvocationError{
			Identity:       managed.Identity,
			ReadinessState: managed.ReadinessState,
			LifecycleState: managed.LifecycleState,
			Cause:          ErrManagedRuntimeUnsupported,
		}
	}
}

// IsManagedRuntimeInvocationBlocked reports whether err blocks invocation because
// the managed runtime is not ready.
func IsManagedRuntimeInvocationBlocked(err error) bool {
	return errors.Is(err, ErrManagedRuntimeMissing) ||
		errors.Is(err, ErrManagedRuntimeLoading) ||
		errors.Is(err, ErrManagedRuntimeFailed) ||
		errors.Is(err, ErrManagedRuntimeUnsupported)
}

// IsManagedRuntimeMissing reports whether err indicates the runtime must be
// pulled or installed before invocation.
func IsManagedRuntimeMissing(err error) bool {
	return errors.Is(err, ErrManagedRuntimeMissing)
}

func managedRuntimeInvocationAction(readiness factoryapi.ManagedRuntimeReadinessState) string {
	switch readiness {
	case factoryapi.ManagedRuntimeReadinessStateMISSING:
		return "pull or install the managed runtime before invoking"
	case factoryapi.ManagedRuntimeReadinessStateLOADING:
		return "wait for the managed runtime to finish loading before invoking"
	case factoryapi.ManagedRuntimeReadinessStateFAILED:
		return "resolve the managed runtime failure before invoking"
	case factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED:
		return "use a supported managed runtime for invocation"
	default:
		return ""
	}
}
