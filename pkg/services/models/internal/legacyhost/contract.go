package modelhost

import (
	"context"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
)

var (
	// ErrInvalidDependencies classifies model-host construction failures.
	ErrInvalidDependencies = models.ErrInvalidHostDependencies
	// ErrCancelled reports that a model host operation was cancelled.
	ErrCancelled = models.ErrHostCancelled
	// ErrUnsupportedRuntime reports that the managed runtime identity is unsupported.
	ErrUnsupportedRuntime = models.ErrHostUnsupportedRuntime
	// ErrMissingAssets reports that required local model assets are not installed.
	ErrMissingAssets = models.ErrHostMissingAssets
	// ErrLoadingTimeout reports that readiness did not complete before timeout.
	ErrLoadingTimeout = models.ErrHostLoadingTimeout
	// ErrProcessCrash reports that the supervised runtime process exited unexpectedly.
	ErrProcessCrash = models.ErrHostProcessCrash
	// ErrCapacityExhausted reports that lease capacity is exhausted.
	ErrCapacityExhausted = models.ErrHostCapacityExhausted
	// ErrLeaseNotFound reports that a lease identifier is unknown.
	ErrLeaseNotFound = models.ErrHostLeaseNotFound
	// ErrRuntimeNotReady reports that lease acquisition requires a ready runtime.
	ErrRuntimeNotReady = models.ErrHostRuntimeNotReady
)

// Host is the process-wide model host contract for local managed runtime capacity.
type Host interface {
	ResolveIdentity(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string) (Identity, error)
	InspectReadiness(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string) (ReadinessSnapshot, error)
	Pull(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string) (PullSnapshot, error)
	AcquireLease(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string, opts LeaseOptions) (Lease, error)
	ReleaseLease(ctx context.Context, leaseID string) error
	Unload(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string) error
}

// Identity resolves one managed runtime identity for host operations.
type Identity = models.HostIdentity

// CacheInspection reports installed managed-runtime assets from local cache.
type CacheInspection struct {
	Supported          bool
	Installed          bool
	Revision           string
	CachePath          string
	InstalledFileCount int
	MissingAssets      []string
	PartialArtifacts   bool
	ManifestPresent    bool
	ManifestValid      bool
	ExpectedArtifacts  []models.AssetRequirement
	ObservedArtifacts  []models.AssetArtifact
	ActivePull         bool
	IntegrityVerified  bool
	FailureReason      string
}

// ReadinessSnapshot carries managed-runtime-compatible readiness for one identity.
type ReadinessSnapshot = models.HostReadinessSnapshot

// PullDownloadedFile carries one pulled asset entry for managed-runtime pull responses.
type PullDownloadedFile struct {
	Path   string
	Bytes  int64
	SHA256 string
}

// PullSnapshot carries managed-runtime-compatible pull outcomes.
type PullSnapshot struct {
	ReadinessSnapshot
	PullOutcome     managedruntime.PullOutcome
	LegacyOutcome   string
	CachePath       string
	Revision        string
	DownloadedFiles []PullDownloadedFile
}

// LeaseOptions configures lease acquisition.
type LeaseOptions = models.HostLeaseOptions

// Lease grants disposable call capacity for one loaded managed runtime.
type Lease = models.HostLease

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
	PullOutcome     managedruntime.PullOutcome
	Snapshot        ReadinessSnapshot
	LegacyOutcome   string
	CachePath       string
	Revision        string
	DownloadedFiles []PullDownloadedFile
}

// AssetPuller performs managed-runtime asset pulls for the model host boundary.
type AssetPuller interface {
	PullModel(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string) (AssetPullResult, error)
}

// CacheInspector inspects installed managed-runtime assets for the model host boundary.
type CacheInspector interface {
	InspectRuntimeCache(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string) (CacheInspection, error)
}

// AssetGateway is the legacy combined pull and cache boundary.
type AssetGateway interface {
	AssetPuller
	CacheInspector
}

// ReadinessError blocks host operations because the runtime is not ready.
type ReadinessError = models.HostReadinessError

// FailureClass is a provider-neutral outcome for model host operations.
type FailureClass = models.HostFailureClass

const (
	FailureClassNone               = models.HostFailureClassNone
	FailureClassMissingAssets      = models.HostFailureClassMissingAssets
	FailureClassLoadingTimeout     = models.HostFailureClassLoadingTimeout
	FailureClassProcessCrash       = models.HostFailureClassProcessCrash
	FailureClassUnsupportedRuntime = models.HostFailureClassUnsupportedRuntime
	FailureClassCancelled          = models.HostFailureClassCancelled
	FailureClassCapacityExhausted  = models.HostFailureClassCapacityExhausted
)
