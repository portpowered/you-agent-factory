// Package models defines the public contracts of the Models service family.
package models

import "context"

// Service is the singular cross-service Models root authority. Peer packages
// depend on this one named interface for Models-owned runtime scope, catalog,
// assets, host/lease readiness, and local infer operations rather than nested
// local-runtime implementation interfaces. Workers decide when to ask Models to
// handle an invocation; Models owns the local runtime lifecycle when it does.
type Service interface {
	// OpenRuntimeScope registers detached Models configuration and returns an
	// opaque reference. Implementations must not construct or return another
	// Service, host, runtime, puller, limiter, process, or storage handle while
	// opening the scope.
	OpenRuntimeScope(context.Context, OpenRuntimeScopeRequest) (OpenRuntimeScopeResult, error)
	// CloseRuntimeScope closes one previously opened scope. Invalid, stale,
	// already-closed, and foreign references return distinct Models-owned
	// failures.
	CloseRuntimeScope(context.Context, CloseRuntimeScopeRequest) (CloseRuntimeScopeResult, error)
	// ForRuntime binds this already-constructed service to one Factory Session's
	// runtime values (CacheDirectory plus Models-owned RuntimeConfig projection).
	// Construction and process-launcher ports remain owned by the injected
	// service; Factory Sessions supplies only plain runtime-scope data.
	// Invalid or missing binding inputs fail closed with ErrInvalidRuntimeBinding.
	//
	// Deprecated: target peers should use OpenRuntimeScope and carry its opaque
	// RuntimeScopeRef. ForRuntime remains during the separate consumer-migration
	// and implementation packets.
	ForRuntime(RuntimeBinding) (Service, error)
	// ListModels returns detached Models-owned catalog summaries (Status,
	// LoadState, ManagedRuntime readiness) without exposing nested catalog
	// assemblers. Unavailable catalog scope fails with ErrUnavailable.
	ListModels(context.Context) (List, error)
	// GetModel returns one detached Models-owned catalog Detail. Missing models
	// fail with ErrNotFound; unavailable catalog scope fails with ErrUnavailable.
	GetModel(context.Context, string) (Detail, error)
	// PullModel pulls managed model assets and returns a Models-owned PullResult
	// (downloaded-file and pull-outcome vocabulary). Not-available, pull-
	// unsupported, and source-fetch-failed cases return distinct typed outcomes
	// (ErrNotAvailable, ErrPullUnsupported, ErrSourceFetchFailed / PullError).
	// Asset pull stays on this singular root Service; peers do not import a
	// nested asset-gateway interface.
	PullModel(context.Context, string) (PullResult, error)
	// InspectRuntime returns Models-owned host readiness for one model
	// (ReadinessState / LifecycleState vocabulary). Missing assets and loading
	// timeout fail with distinct typed host outcomes (ErrHostMissingAssets,
	// ErrHostLoadingTimeout). Readiness inspect stays on this singular root
	// Service; peers do not import nested host supervisor types.
	InspectRuntime(context.Context, string) (Runtime, error)
	// AcquireLease acquires Models-owned local capacity and returns a HostLease.
	// Capacity exhausted and runtime-not-ready fail with distinct typed outcomes
	// (ErrHostCapacityExhausted, ErrHostRuntimeNotReady). Lease acquire stays on
	// this singular root Service; peers do not import nested lease-manager types.
	AcquireLease(context.Context, AcquireLeaseRequest) (HostLease, error)
	// ReleaseLease releases one Models-owned HostLease by id. Unknown leases fail
	// with ErrHostLeaseNotFound. Lease release stays on this singular root
	// Service; peers do not import nested host supervisor types.
	ReleaseLease(context.Context, ReleaseLeaseRequest) error
	// InvokeLocal accepts or declines local/direct invocation and returns a
	// Models-owned LocalInvocationResult (Handled/not-handled). Missing,
	// loading, failed, and unsupported readiness fail with distinct typed
	// outcomes (ErrMissing, ErrLoading, ErrFailed, ErrUnsupported /
	// InvocationError). Unsupported response modes fail with
	// ErrUnsupportedResponseMode. Infer stays on this singular root Service;
	// peers do not import a nested invoker or local-execution gateway.
	InvokeLocal(context.Context, LocalInvocationRequest) (LocalInvocationResult, error)
}

// RuntimeBinding is the plain runtime-scope binding request passed to
// ForRuntime. It carries session-selected Models data only; peers must not need
// HostProcessLauncher or other local-runtime construction ports to supply it.
type RuntimeBinding struct {
	CacheDirectory string
	RuntimeConfig  RuntimeConfigLoader
}

// PullMetric records one managed-runtime pull counter.
type PullMetric struct {
	Name   string
	Labels map[string]string
}

// PullMetricsRecorder receives managed-runtime pull counter emissions.
type PullMetricsRecorder interface {
	RecordModelPullMetric(PullMetric)
}
