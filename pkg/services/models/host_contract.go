package models

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrInvalidHostDependencies classifies model-host construction failures.
	ErrInvalidHostDependencies = errors.New("model host dependencies are invalid")
	// ErrHostCancelled reports that a model host operation was cancelled.
	ErrHostCancelled = errors.New("model host operation cancelled")
	// ErrHostUnsupportedRuntime reports that the managed runtime identity is unsupported.
	ErrHostUnsupportedRuntime = errors.New("model host unsupported runtime")
	// ErrHostMissingAssets reports that required local model assets are not installed.
	ErrHostMissingAssets = errors.New("model host missing assets")
	// ErrHostLoadingTimeout reports that readiness did not complete before timeout.
	ErrHostLoadingTimeout = errors.New("model host loading timeout")
	// ErrHostProcessCrash reports that the supervised runtime process exited unexpectedly.
	ErrHostProcessCrash = errors.New("model host process crash")
	// ErrHostCapacityExhausted reports that lease capacity is exhausted.
	ErrHostCapacityExhausted = errors.New("model host capacity exhausted")
	// ErrHostLeaseNotFound reports that a lease identifier is unknown.
	ErrHostLeaseNotFound = errors.New("model host lease not found")
	// ErrHostRuntimeNotReady reports that lease acquisition requires a ready runtime.
	ErrHostRuntimeNotReady = errors.New("model host runtime not ready")
)

// HostIdentity resolves one managed runtime identity for host operations.
type HostIdentity struct {
	Name                string
	Locality            Locality
	SupportedOperations []Operation
	Backend             string
	LoadPolicy          string
	SourceKind          string
	SourceID            string
	ResolverNotes       string
}

// HostReadinessSnapshot carries readiness for one host identity.
type HostReadinessSnapshot struct {
	Identity       HostIdentity
	ReadinessState ReadinessState
	LifecycleState LifecycleState
	FailureClass   HostFailureClass
	Diagnostics    map[string]string
}

// HostReadinessError blocks host operations because the runtime is not ready.
type HostReadinessError struct {
	Snapshot HostReadinessSnapshot
	Cause    error
}

func (e *HostReadinessError) Error() string {
	if e == nil {
		return ""
	}
	action := "resolve model host readiness"
	if e.Snapshot.FailureClass != HostFailureClassNone {
		action = fmt.Sprintf("resolve failure class %s", e.Snapshot.FailureClass)
	}
	return fmt.Sprintf(
		"model host %q readiness is %s (lifecycle %s): %s",
		e.Snapshot.Identity.Name,
		e.Snapshot.ReadinessState,
		e.Snapshot.LifecycleState,
		action,
	)
}

func (e *HostReadinessError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ManagedRuntimeReadinessState exposes host readiness through the Models seam.
func (e *HostReadinessError) ManagedRuntimeReadinessState() ReadinessState {
	if e == nil {
		return ""
	}
	return e.Snapshot.ReadinessState
}

// HostFailureClass is a provider-neutral outcome for model host operations.
type HostFailureClass string

const (
	HostFailureClassNone               HostFailureClass = ""
	HostFailureClassMissingAssets      HostFailureClass = "missing_assets"
	HostFailureClassLoadingTimeout     HostFailureClass = "loading_timeout"
	HostFailureClassProcessCrash       HostFailureClass = "process_crash"
	HostFailureClassUnsupportedRuntime HostFailureClass = "unsupported_runtime"
	HostFailureClassCancelled          HostFailureClass = "cancelled"
	HostFailureClassCapacityExhausted  HostFailureClass = "capacity_exhausted"
)

// LocalRuntimeHooks observes model resource and load lifecycle activity.
type LocalRuntimeHooks struct {
	MarkResourceWaitStarted  func(context.Context, time.Time)
	MarkResourceWaitFinished func(context.Context, time.Time, bool)
	MarkLoadRequested        func(context.Context, time.Time)
	MarkLoadFinished         func(context.Context, time.Time)
	MarkLoadReused           func(context.Context)
}
