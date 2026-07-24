// Package models defines the public contracts of the Models service family.
package models

import "context"

// Service is the singular cross-service Models root authority. Peer packages
// depend on this one named interface for Models-owned runtime scope, catalog,
// assets, host/lease readiness, and local infer operations rather than nested
// local-runtime implementation interfaces. Workers decide when to ask Models to
// handle an invocation; Models owns the local runtime lifecycle when it does.
type Service interface {
	// ForRuntime binds this already-constructed service to one Factory Session's
	// runtime values (CacheDirectory plus Models-owned RuntimeConfig projection).
	// Construction and process-launcher ports remain owned by the injected
	// service; Factory Sessions supplies only plain runtime-scope data.
	// Invalid or missing binding inputs fail closed with ErrInvalidRuntimeBinding.
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
	InspectRuntime(context.Context, string) (Runtime, error)
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
