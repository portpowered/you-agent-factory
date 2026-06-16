package modelhost

import (
	"context"
	"errors"
	"fmt"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
)

var (
	// ErrCancelled reports that a model host operation was cancelled.
	ErrCancelled = errors.New("model host operation cancelled")
	// ErrUnsupportedRuntime reports that the managed runtime identity is unsupported.
	ErrUnsupportedRuntime = errors.New("model host unsupported runtime")
	// ErrMissingAssets reports that required local model assets are not installed.
	ErrMissingAssets = errors.New("model host missing assets")
	// ErrLoadingTimeout reports that readiness did not complete before timeout.
	ErrLoadingTimeout = errors.New("model host loading timeout")
	// ErrProcessCrash reports that the supervised runtime process exited unexpectedly.
	ErrProcessCrash = errors.New("model host process crash")
	// ErrCapacityExhausted reports that lease capacity is exhausted.
	ErrCapacityExhausted = errors.New("model host capacity exhausted")
	// ErrLeaseNotFound reports that a lease identifier is unknown.
	ErrLeaseNotFound = errors.New("model host lease not found")
	// ErrRuntimeNotReady reports that lease acquisition requires a ready runtime.
	ErrRuntimeNotReady = errors.New("model host runtime not ready")
)

// Host is the process-wide model host contract for local managed runtime capacity.
type Host interface {
	ResolveIdentity(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (Identity, error)
	InspectReadiness(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (ReadinessSnapshot, error)
	Pull(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (PullSnapshot, error)
	AcquireLease(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string, opts LeaseOptions) (Lease, error)
	ReleaseLease(ctx context.Context, leaseID string) error
	Unload(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) error
}

// Identity resolves one managed runtime identity for host operations.
type Identity struct {
	Name                 string
	Locality             factoryapi.WorkerModelLocality
	SupportedOperations  []factoryapi.ModelOperation
	Backend              string
	LoadPolicy           string
	SourceKind           string
	SourceID             string
	ResolverNotes        string
}

// CacheInspection reports installed managed-runtime assets from local cache.
type CacheInspection struct {
	Supported          bool
	Installed          bool
	Revision           string
	CachePath          string
	InstalledFileCount int
	MissingAssets      []string
	PartialArtifacts   bool
}

// ReadinessSnapshot carries managed-runtime-compatible readiness for one identity.
type ReadinessSnapshot struct {
	Identity       Identity
	ReadinessState factoryapi.ManagedRuntimeReadinessState
	LifecycleState factoryapi.ManagedRuntimeLifecycleState
	FailureClass   FailureClass
	Diagnostics    map[string]string
}

// PullDownloadedFile carries one pulled asset entry for managed-runtime pull responses.
type PullDownloadedFile struct {
	Path   string
	Bytes  int64
	SHA256 string
}

// PullSnapshot carries managed-runtime-compatible pull outcomes.
type PullSnapshot struct {
	ReadinessSnapshot
	PullOutcome     factoryapi.ManagedRuntimePullOutcome
	LegacyOutcome   string
	CachePath       string
	Revision        string
	DownloadedFiles []PullDownloadedFile
}

// LeaseOptions configures lease acquisition.
type LeaseOptions struct {
	Holder string
}

// Lease grants disposable call capacity for one loaded managed runtime.
type Lease struct {
	ID       string
	Identity Identity
	Endpoint string
	Holder   string
}

// Options configures catalog-backed host construction.
type Options struct {
	SourceResolver    SourceResolver
	Supervisor        SupervisorConfig
	IdleUnloadAfter   time.Duration
	MaxLoadedRuntimes int
}

// SourceResolution classifies which backend source satisfies one managed runtime.
type SourceResolution struct {
	SourceKind    string
	SourceID      string
	ResolverNotes string
}

// SourceResolver selects a backend source for one managed runtime identity.
type SourceResolver interface {
	Resolve(modelName string, backend string, loadPolicy string, provider string) SourceResolution
}

// AssetPullResult carries pull metadata projected through the model host boundary.
type AssetPullResult struct {
	PullOutcome     factoryapi.ManagedRuntimePullOutcome
	Snapshot        ReadinessSnapshot
	LegacyOutcome   string
	CachePath       string
	Revision        string
	DownloadedFiles []PullDownloadedFile
}

// AssetGateway integrates pull and cache inspection for the model host boundary.
type AssetGateway interface {
	PullModel(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (AssetPullResult, error)
	InspectRuntimeCache(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (CacheInspection, error)
}

// ReadinessError blocks host operations because the runtime is not ready.
type ReadinessError struct {
	Snapshot ReadinessSnapshot
	Cause    error
}

func (e *ReadinessError) Error() string {
	if e == nil {
		return ""
	}
	action := "resolve model host readiness"
	if e.Snapshot.FailureClass != FailureClassNone {
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

func (e *ReadinessError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
