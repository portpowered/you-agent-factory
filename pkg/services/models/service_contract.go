// Package models defines the public contracts of the Models service family.
package models

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

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
	// ResolveModelReference resolves one configured name or supported source
	// reference without inspecting assets or starting a backend. The result is
	// detached and contains only safe provenance and effective model policy.
	ResolveModelReference(context.Context, ResolveModelReferenceRequest) (ResolveModelReferenceResult, error)
	// PullModelForScope preserves the established pull result contract while
	// requiring the caller to identify the opened runtime scope explicitly.
	PullModelForScope(context.Context, PullModelRequest) (PullResult, error)
	// PreflightModelAssets resolves cache-aware download requirements and byte
	// estimates without downloading asset content.
	PreflightModelAssets(context.Context, PrepareModelAssetsRequest) (PreflightModelAssetsResult, error)
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
	// InvokeModel owns the complete provider-neutral invocation transaction. It
	// validates and resolves the request, prepares model assets, makes a
	// compatible host ready, acquires capacity, delegates the prepared call to
	// InvokeModelWithLease, and returns only detached outputs and failures.
	InvokeModel(context.Context, InvokeModelRequest) (InvokeModelResult, error)
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
	OperatorModels map[string]ModelOverlay
}

var (
	// ErrModelReferenceInvalid classifies a source reference that cannot be
	// interpreted as a configured name or supported local/Hugging Face source.
	ErrModelReferenceInvalid = errors.New("model reference is invalid")
	// ErrModelReferenceUnknown classifies an otherwise symbolic name that is
	// not present in the effective model catalog.
	ErrModelReferenceUnknown = errors.New("model reference name is unknown")
	// ErrModelRevisionUnresolved classifies a Hugging Face source whose revision
	// was not proven immutable before acquisition.
	ErrModelRevisionUnresolved = errors.New("model source revision is not immutable")
	// ErrModelConfigurationInvalid classifies one invalid operator model entry.
	ErrModelConfigurationInvalid = errors.New("model configuration is invalid")
)

// ModelReferenceSourceKind identifies the safe source provenance of a
// resolved reference. It intentionally does not expose a cache or host path.
type ModelReferenceSourceKind string

const (
	ModelReferenceSourceNamed       ModelReferenceSourceKind = "NAMED"
	ModelReferenceSourceLocalPath   ModelReferenceSourceKind = "LOCAL_PATH"
	ModelReferenceSourceFileURI     ModelReferenceSourceKind = "FILE_URI"
	ModelReferenceSourceHuggingFace ModelReferenceSourceKind = "HUGGING_FACE"
)

// ModelReferenceProvenance is the detached, safe provenance of one model
// reference. Local paths are represented only by kind and a stable local
// label; the original path never crosses the Models boundary.
type ModelReferenceProvenance struct {
	Kind              ModelReferenceSourceKind
	SourceKind        ModelReferenceSourceKind
	Name              string
	Owner             string
	Repository        string
	File              string
	Revision          string
	ImmutableRevision string
}

// Clone returns a detached provenance value.
func (provenance ModelReferenceProvenance) Clone() ModelReferenceProvenance {
	return provenance
}

// ResolvedModelReference is the safe result of resolving a model reference.
// Runtime readiness is a pre-acquisition projection and therefore does not
// imply that weights or a backend have been loaded.
type ResolvedModelReference struct {
	Definition ModelDefinition
	Provenance ModelReferenceProvenance
	Readiness  ReadinessState
}

// Clone returns a detached resolved reference.
func (resolved ResolvedModelReference) Clone() ResolvedModelReference {
	resolved.Definition = resolved.Definition.Clone()
	resolved.Provenance = resolved.Provenance.Clone()
	return resolved
}

// ResolveModelReferenceRequest identifies one scoped reference. Scope-owned
// operator overlays are captured when the runtime scope opens.
type ResolveModelReferenceRequest struct {
	Scope     RuntimeScopeRef
	Reference ModelReference
}

// Validate checks fields that do not require scope or catalog state.
func (request ResolveModelReferenceRequest) Validate() error {
	if request.Scope.IsZero() {
		return ErrRuntimeScopeInvalid
	}
	if request.Reference.IsZero() {
		return &InvocationFailure{
			Class:   InvocationFailureClassInvalidModelReference,
			Message: "model name or source reference is required",
			Cause:   ErrModelReferenceInvalid,
		}
	}
	return nil
}

// ResolveModelReferenceResult contains one detached effective definition.
type ResolveModelReferenceResult struct {
	Resolved ResolvedModelReference
}

// Clone returns a detached resolution result.
func (result ResolveModelReferenceResult) Clone() ResolveModelReferenceResult {
	result.Resolved = result.Resolved.Clone()
	return result
}

// ModelOverlay is the Models-owned representation of one optional operator
// configuration entry. Nil fields preserve the corresponding built-in field.
type ModelOverlay struct {
	Source     *string
	Backend    *string
	LoadPolicy *LoadPolicy
	Operations []string
}

// ModelConfiguration and OperatorModelConfiguration are descriptive aliases
// for callers that use the configuration vocabulary at different boundaries.
type ModelConfiguration = ModelOverlay
type OperatorModelConfiguration = ModelOverlay

// Clone returns a detached overlay.
func (overlay ModelOverlay) Clone() ModelOverlay {
	cloned := overlay
	if overlay.Source != nil {
		value := *overlay.Source
		cloned.Source = &value
	}
	if overlay.Backend != nil {
		value := *overlay.Backend
		cloned.Backend = &value
	}
	if overlay.LoadPolicy != nil {
		value := *overlay.LoadPolicy
		cloned.LoadPolicy = &value
	}
	cloned.Operations = append([]string(nil), overlay.Operations...)
	return cloned
}

// ModelConfigurationFailure identifies the exact operator model entry and
// field that is invalid while retaining no storage or path details.
type ModelConfigurationFailure struct {
	ModelName string
	Field     string
	Message   string
}

func (failure ModelConfigurationFailure) Error() string {
	message := strings.TrimSpace(failure.Message)
	if message == "" {
		message = "invalid value"
	}
	return fmt.Sprintf("model configuration %q field %q is invalid: %s", failure.ModelName, failure.Field, message)
}

func (failure ModelConfigurationFailure) Unwrap() error {
	return ErrModelConfigurationInvalid
}
