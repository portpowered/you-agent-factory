package apisurface

import (
	"errors"

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
type ManagedRuntimeInvocationError = managedruntime.InvocationError

// InvocationErrorFromManagedRuntime maps one managed-runtime readiness projection
// into an invocation error. READY returns nil.
func InvocationErrorFromManagedRuntime(managed factoryapi.ManagedRuntime) error {
	return managedruntime.InvocationErrorForRuntime(managedRuntimeFromAPI(managed))
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

func managedRuntimeFromAPI(managed factoryapi.ManagedRuntime) managedruntime.Runtime {
	return managedruntime.Runtime{
		Identity:       managed.Identity,
		ReadinessState: managedruntime.ReadinessState(managed.ReadinessState),
		LifecycleState: managedruntime.LifecycleState(managed.LifecycleState),
	}
}
