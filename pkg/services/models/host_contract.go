package models

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrInvalidHostDependencies classifies model-host construction failures.
	ErrInvalidHostDependencies = errors.New("model host dependencies are invalid")
	// ErrHostCancelled reports that a model host operation was cancelled.
	ErrHostCancelled = errors.New("model host operation cancelled")
	// ErrHostUnsupportedRuntime reports that the managed runtime identity is unsupported.
	ErrHostUnsupportedRuntime = errors.New("model host unsupported runtime")
	// ErrHostMissingAssets reports that required local model assets are not
	// installed. Distinct from loading-timeout, capacity, lease-not-found, and
	// runtime-not-ready outcomes on the host/lease root slice.
	ErrHostMissingAssets = errors.New("model host missing assets")
	// ErrHostLoadingTimeout reports that readiness did not complete before
	// timeout. Distinct from missing-assets, capacity, lease-not-found, and
	// runtime-not-ready outcomes on the host/lease root slice.
	ErrHostLoadingTimeout = errors.New("model host loading timeout")
	// ErrHostProcessCrash reports that the supervised runtime process exited unexpectedly.
	ErrHostProcessCrash = errors.New("model host process crash")
	// ErrHostCapacityExhausted reports that lease capacity is exhausted.
	// Distinct from missing-assets, loading-timeout, lease-not-found, and
	// runtime-not-ready outcomes on the host/lease root slice.
	ErrHostCapacityExhausted = errors.New("model host capacity exhausted")
	// ErrHostCapacityContended reports that another holder currently contends
	// for otherwise available model capacity.
	ErrHostCapacityContended = errors.New("model host capacity contended")
	// ErrHostLeaseNotFound reports that a lease identifier is unknown.
	// Distinct from missing-assets, loading-timeout, capacity, and
	// runtime-not-ready outcomes on the host/lease root slice.
	ErrHostLeaseNotFound = errors.New("model host lease not found")
	// ErrHostLeaseExpired reports that an issued lease is no longer usable
	// because its expiry time has passed.
	ErrHostLeaseExpired = errors.New("model host lease expired")
	// ErrHostInvalidHolder reports that lease acquisition omitted a stable
	// holder identity.
	ErrHostInvalidHolder = errors.New("model host lease holder is invalid")
	// ErrHostRuntimeNotReady reports that lease acquisition requires a ready
	// runtime. Distinct from missing-assets, loading-timeout, capacity, and
	// lease-not-found outcomes on the host/lease root slice.
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

// LocalRuntimeHooks observes model resource and load lifecycle activity for
// Wire/construction-time ProcessDependencies. It is not a peer-facing host/lease
// or infer contract; peers use Service.InspectRuntime, AcquireLease,
// ReleaseLease, and InvokeLocal with plain request/result vocabulary instead.
type LocalRuntimeHooks struct {
	MarkResourceWaitStarted  func(context.Context, time.Time)
	MarkResourceWaitFinished func(context.Context, time.Time, bool)
	MarkLoadRequested        func(context.Context, time.Time)
	MarkLoadFinished         func(context.Context, time.Time)
	MarkLoadReused           func(context.Context)
}

// ModelHostSnapshot contains detached peer-required host readiness facts.
// Supervisor slots, processes, health clients, timers, eviction policy, and
// runtime handles deliberately remain private.
type ModelHostSnapshot struct {
	Scope          RuntimeScopeRef
	ModelName      string
	ReadinessState ReadinessState
	LifecycleState LifecycleState
	Diagnostics    map[string]string
}

// Clone returns a detached host snapshot safe for a peer to retain or mutate.
func (snapshot ModelHostSnapshot) Clone() ModelHostSnapshot {
	snapshot.Diagnostics = cloneStringMap(snapshot.Diagnostics)
	return snapshot
}

// HostEnsureOutcome classifies whether ensuring readiness reused or started a
// supervised host.
type HostEnsureOutcome string

const (
	HostEnsureAlreadyReady HostEnsureOutcome = "ALREADY_READY"
	HostEnsureBecameReady  HostEnsureOutcome = "BECAME_READY"
)

// HostStopOutcome classifies whether stopping a host changed state.
type HostStopOutcome string

const (
	HostStopStopped        HostStopOutcome = "STOPPED"
	HostStopAlreadyStopped HostStopOutcome = "ALREADY_STOPPED"
)

// EnsureModelHostRequest asks Models to make one scoped host ready.
type EnsureModelHostRequest struct {
	Scope RuntimeScopeRef
	Name  string
}

// Validate checks the plain ensure-host request without touching runtime
// machinery.
func (request EnsureModelHostRequest) Validate() error {
	return validateScopedModelHostRequest(request.Scope, request.Name)
}

// EnsureModelHostResult reports detached readiness and whether work was needed.
type EnsureModelHostResult struct {
	Host    ModelHostSnapshot
	Outcome HostEnsureOutcome
}

// InspectModelHostRequest asks for current scoped host readiness.
type InspectModelHostRequest struct {
	Scope RuntimeScopeRef
	Name  string
}

// Validate checks the plain inspect-host request.
func (request InspectModelHostRequest) Validate() error {
	return validateScopedModelHostRequest(request.Scope, request.Name)
}

// InspectModelHostResult reports detached current host readiness.
type InspectModelHostResult struct {
	Host ModelHostSnapshot
}

// StopModelHostRequest asks Models to stop or unload one scoped host.
type StopModelHostRequest struct {
	Scope RuntimeScopeRef
	Name  string
}

// Validate checks the plain stop-host request.
func (request StopModelHostRequest) Validate() error {
	return validateScopedModelHostRequest(request.Scope, request.Name)
}

// StopModelHostResult reports detached final host state and whether stopping
// changed state.
type StopModelHostResult struct {
	Host    ModelHostSnapshot
	Outcome HostStopOutcome
}

func validateScopedModelHostRequest(scope RuntimeScopeRef, name string) error {
	if scope.IsZero() {
		return ErrRuntimeScopeInvalid
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: empty model name", ErrNotFound)
	}
	return nil
}

// ModelLeaseRef is an opaque Models-owned lease capability reference. Peers
// may compare, serialize, and carry it, but cannot inspect its representation.
type ModelLeaseRef struct {
	value string
}

// Parse restores an opaque lease reference received from a trusted boundary.
func (ModelLeaseRef) Parse(value string) (ModelLeaseRef, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return ModelLeaseRef{}, ErrHostLeaseNotFound
	}
	return ModelLeaseRef{value: value}, nil
}

// String returns the opaque serialized lease value.
func (ref ModelLeaseRef) String() string {
	return ref.value
}

// IsZero reports whether no lease reference was supplied.
func (ref ModelLeaseRef) IsZero() bool {
	return strings.TrimSpace(ref.value) == ""
}

// ModelLeaseStatus names the observable lifecycle of an issued lease.
type ModelLeaseStatus string

const (
	ModelLeaseStatusActive   ModelLeaseStatus = "ACTIVE"
	ModelLeaseStatusReleased ModelLeaseStatus = "RELEASED"
	ModelLeaseStatusExpired  ModelLeaseStatus = "EXPIRED"
)

// ModelLease is a detached Models-owned capacity capability. It contains only
// peer-required identity, association, expiry, and readiness facts.
type ModelLease struct {
	Lease         ModelLeaseRef
	Scope         RuntimeScopeRef
	ModelName     string
	Holder        string
	ExpiresAt     time.Time
	Status        ModelLeaseStatus
	HostReadiness ReadinessState
}

// AcquireModelLeaseRequest asks Models to reserve capacity for one holder.
type AcquireModelLeaseRequest struct {
	Scope  RuntimeScopeRef
	Name   string
	Holder string
}

// Validate checks the plain lease-acquisition request.
func (request AcquireModelLeaseRequest) Validate() error {
	if err := validateScopedModelHostRequest(request.Scope, request.Name); err != nil {
		return err
	}
	if strings.TrimSpace(request.Holder) == "" {
		return ErrHostInvalidHolder
	}
	return nil
}

// AcquireModelLeaseResult returns the issued detached lease capability.
type AcquireModelLeaseResult struct {
	Lease ModelLease
}

// GetModelLeaseRequest asks Models for current status of an issued lease.
type GetModelLeaseRequest struct {
	Scope RuntimeScopeRef
	Lease ModelLeaseRef
}

// Validate checks the plain lease-status request.
func (request GetModelLeaseRequest) Validate() error {
	return validateModelLeaseRequest(request.Scope, request.Lease)
}

// GetModelLeaseResult returns detached current lease facts.
type GetModelLeaseResult struct {
	Lease ModelLease
}

// ReleaseModelLeaseRequest asks Models to release an issued lease.
type ReleaseModelLeaseRequest struct {
	Scope RuntimeScopeRef
	Lease ModelLeaseRef
}

// Validate checks the plain lease-release request.
func (request ReleaseModelLeaseRequest) Validate() error {
	return validateModelLeaseRequest(request.Scope, request.Lease)
}

// ModelLeaseReleaseOutcome classifies whether release changed lease state.
type ModelLeaseReleaseOutcome string

const (
	ModelLeaseReleased        ModelLeaseReleaseOutcome = "RELEASED"
	ModelLeaseAlreadyReleased ModelLeaseReleaseOutcome = "ALREADY_RELEASED"
)

// ReleaseModelLeaseResult reports the detached released state.
type ReleaseModelLeaseResult struct {
	Lease   ModelLease
	Outcome ModelLeaseReleaseOutcome
}

func validateModelLeaseRequest(scope RuntimeScopeRef, lease ModelLeaseRef) error {
	if scope.IsZero() {
		return ErrRuntimeScopeInvalid
	}
	if lease.IsZero() {
		return ErrHostLeaseNotFound
	}
	return nil
}

// InspectRuntimeRequest is the plain host readiness-inspect request. Peers
// identify a model by Name without importing models/internal/host.
type InspectRuntimeRequest struct {
	Scope RuntimeScopeRef
	Name  string
}

// ValidateInspectRuntimeRequest checks the plain readiness-inspect request.
// Empty names fail closed as ErrNotFound without touching nested host packages.
func ValidateInspectRuntimeRequest(request InspectRuntimeRequest) error {
	if strings.TrimSpace(request.Name) == "" {
		return fmt.Errorf("%w: empty model name", ErrNotFound)
	}
	return nil
}

// AcquireLeaseRequest is the plain host/lease acquire request. Peers identify a
// model and optional holder without importing nested host supervisor or
// lease-manager implementation types.
type AcquireLeaseRequest struct {
	Scope     RuntimeScopeRef
	ModelName string
	Holder    string
}

// ValidateAcquireLeaseRequest checks the plain lease-acquire request. Empty
// model names fail closed as ErrNotFound without touching nested host packages.
func ValidateAcquireLeaseRequest(request AcquireLeaseRequest) error {
	if strings.TrimSpace(request.ModelName) == "" {
		return fmt.Errorf("%w: empty model name", ErrNotFound)
	}
	return nil
}

// ReleaseLeaseRequest is the plain host/lease release request. Peers identify a
// lease by LeaseID without importing nested host supervisor types.
type ReleaseLeaseRequest struct {
	Scope   RuntimeScopeRef
	LeaseID string
}

// ValidateReleaseLeaseRequest checks the plain lease-release request. Empty
// lease identifiers fail closed as ErrHostLeaseNotFound.
func ValidateReleaseLeaseRequest(request ReleaseLeaseRequest) error {
	if strings.TrimSpace(request.LeaseID) == "" {
		return fmt.Errorf("%w: empty lease id", ErrHostLeaseNotFound)
	}
	return nil
}

// HostLeaseOptions configures lease acquisition on the Models root host/lease
// slice. Peers supply Holder without nested lease-manager types.
type HostLeaseOptions struct {
	Holder string
}

// HostLease grants disposable call capacity for one loaded managed runtime.
// Peers consume this Models-owned vocabulary without importing
// models/internal/host supervisor, process, or lease-manager types.
type HostLease struct {
	ID       string
	Identity HostIdentity
	Endpoint string
	Holder   string
}
