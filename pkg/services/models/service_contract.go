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
	// ListCatalog returns detached catalog summaries for one open runtime scope.
	// Invalid, stale, closed, and foreign scopes retain their Models-owned
	// classifications; an unavailable catalog returns ErrUnavailable.
	ListCatalog(context.Context, ListModelsRequest) (ListModelsResult, error)
	// GetCatalogModel returns detached identity, binding, source, operation, and
	// catalog status facts for one model in an open runtime scope.
	GetCatalogModel(context.Context, GetModelRequest) (GetModelResult, error)
	// GetModelReadiness returns current detached readiness facts for one scoped
	// model without exposing a catalog assembler, cache, host, or runtime handle.
	GetModelReadiness(context.Context, GetModelReadinessRequest) (GetModelReadinessResult, error)
	// PullModelForScope preserves the established pull result contract while
	// requiring the caller to identify the opened runtime scope explicitly.
	PullModelForScope(context.Context, PullModelRequest) (PullResult, error)
	// PrepareModelAssets makes configured assets available for one scoped model
	// and distinguishes already-available assets from newly prepared assets.
	// Missing/unsupported sources, interrupted preparation, integrity failure,
	// and cancellation retain distinct Models-owned classifications.
	PrepareModelAssets(context.Context, PrepareModelAssetsRequest) (PrepareModelAssetsResult, error)
	// InspectModelAssets returns detached readiness and optional integrity
	// verification facts without exposing cache layout or filesystem handles.
	// Unavailable assets, integrity failure, and cancellation are distinct from
	// runtime-scope failures.
	InspectModelAssets(context.Context, InspectModelAssetsRequest) (InspectModelAssetsResult, error)
	// RemoveModelAssets removes scoped model assets and reports whether removal
	// changed state or the assets were already absent. Cancellation remains a
	// typed Models-owned failure.
	RemoveModelAssets(context.Context, RemoveModelAssetsRequest) (RemoveModelAssetsResult, error)
	// EnsureModelHost starts or reuses the supervised host for one scoped model
	// and waits until it is ready. Host processes, health clients, runtime
	// handles, supervisor slots, and timers remain private implementation
	// details.
	EnsureModelHost(context.Context, EnsureModelHostRequest) (EnsureModelHostResult, error)
	// InspectModelHost returns detached readiness facts for one supervised host.
	InspectModelHost(context.Context, InspectModelHostRequest) (InspectModelHostResult, error)
	// StopModelHost stops or unloads one supervised host and reports whether the
	// request changed its lifecycle state.
	StopModelHost(context.Context, StopModelHostRequest) (StopModelHostResult, error)
	// AcquireModelLease reserves scoped model capacity for a non-empty holder
	// and returns an opaque, detached Models-owned lease capability.
	AcquireModelLease(context.Context, AcquireModelLeaseRequest) (AcquireModelLeaseResult, error)
	// GetModelLease returns detached lease status, including whether an issued
	// lease is active, released, or expired.
	GetModelLease(context.Context, GetModelLeaseRequest) (GetModelLeaseResult, error)
	// ReleaseModelLease safely releases an issued lease and returns its
	// observable released/already-released outcome.
	ReleaseModelLease(context.Context, ReleaseModelLeaseRequest) (ReleaseModelLeaseResult, error)
	// InvokeModelWithLease runs one scoped model operation under an issued Models lease.
	// Results contain only detached Models-owned content, artifact metadata,
	// invocation identity, and lease-disposition facts. Runtime handles,
	// endpoints, processes, and filesystem paths remain private.
	InvokeModelWithLease(context.Context, InvokeModelRequest) (InvokeModelResult, error)
	// CancelInvocation requests cancellation of one scoped invocation. First,
	// repeated, and late cancellation return typed outcomes; context
	// cancellation and explicit cancellation converge on the same cancelled
	// invocation status and released-capacity facts.
	CancelInvocation(context.Context, CancelInvocationRequest) (CancelInvocationResult, error)
	// InvokeLocal accepts or declines local/direct invocation and returns a
	// Models-owned LocalInvocationResult (Handled/not-handled). Missing,
	// loading, failed, and unsupported readiness fail with distinct typed
	// outcomes (ErrMissing, ErrLoading, ErrFailed, ErrUnsupported /
	// InvocationError). Unsupported response modes fail with
	// ErrUnsupportedResponseMode. Infer stays on this singular root Service;
	// peers do not import a nested invoker or local-execution gateway.
	InvokeLocal(context.Context, LocalInvocationRequest) (LocalInvocationResult, error)
}

// RuntimeBinding is the plain runtime-scope binding request consumed by the
// private Runtime Scopes service. It carries session-selected Models data only;
// peers must not need HostProcessLauncher or other local-runtime construction
// ports to supply it.
type RuntimeBinding struct {
	CacheDirectory string
	RuntimeConfig  RuntimeConfigLoader
}
