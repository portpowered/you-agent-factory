// Package models defines the public contracts of the Models service family.
package models

import "context"

// Service exposes the complete Models capability, including managed local
// invocation. Workers decides when to ask Models to handle an invocation; the
// Models service owns the local runtime lifecycle when it does.
type Service interface {
	// ForRuntime binds this already-constructed service to one Factory Session's
	// runtime values. Construction dependencies remain owned by the injected
	// service; Factory Sessions supplies only its runtime data.
	ForRuntime(RuntimeBinding) (Service, error)
	ListModels(context.Context) (List, error)
	GetModel(context.Context, string) (Detail, error)
	PullModel(context.Context, string) (PullResult, error)
	InspectRuntime(context.Context, string) (Runtime, error)
	InvokeLocal(context.Context, LocalInvocationRequest) (LocalInvocationResult, error)
}

// RuntimeBinding contains the Models-owned runtime data selected while a
// Factory Session is opened. It is data passed to the injected service, not a
// deferred service constructor.
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
