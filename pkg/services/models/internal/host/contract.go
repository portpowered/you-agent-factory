package modelhost

import (
	"context"
	"errors"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
	"strconv"
	"strings"
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

// ReadinessStateForFailureClass maps a failure class to managed-runtime readiness.
func ReadinessStateForFailureClass(class FailureClass) managedruntime.ReadinessState {
	switch class {
	case FailureClassMissingAssets:
		return managedruntime.ReadinessStateMissing
	case FailureClassLoadingTimeout:
		return managedruntime.ReadinessStateLoading
	case FailureClassProcessCrash:
		return managedruntime.ReadinessStateFailed
	case FailureClassCancelled:
		return managedruntime.ReadinessStateFailed
	case FailureClassCapacityExhausted:
		return managedruntime.ReadinessStateFailed
	case FailureClassUnsupportedRuntime:
		return managedruntime.ReadinessStateUnsupported
	default:
		return managedruntime.ReadinessStateUnsupported
	}
}

// FailureClassForReadinessState derives the primary failure class for a readiness state.
func FailureClassForReadinessState(readiness managedruntime.ReadinessState) FailureClass {
	switch readiness {
	case managedruntime.ReadinessStateReady:
		return FailureClassNone
	case managedruntime.ReadinessStateMissing:
		return FailureClassMissingAssets
	case managedruntime.ReadinessStateLoading:
		return FailureClassLoadingTimeout
	case managedruntime.ReadinessStateFailed:
		return FailureClassProcessCrash
	case managedruntime.ReadinessStateUnsupported:
		return FailureClassUnsupportedRuntime
	default:
		return FailureClassUnsupportedRuntime
	}
}

// FailureClassFromError classifies operational errors into provider-neutral classes.
func FailureClassFromError(err error) FailureClass {
	if err == nil {
		return FailureClassNone
	}
	if errors.Is(err, ErrCancelled) {
		return FailureClassCancelled
	}
	if errors.Is(err, ErrUnsupportedRuntime) {
		return FailureClassUnsupportedRuntime
	}
	if errors.Is(err, ErrCapacityExhausted) {
		return FailureClassCapacityExhausted
	}
	if errors.Is(err, ErrMissingAssets) {
		return FailureClassMissingAssets
	}
	if errors.Is(err, ErrLoadingTimeout) {
		return FailureClassLoadingTimeout
	}
	if errors.Is(err, ErrProcessCrash) {
		return FailureClassProcessCrash
	}
	return FailureClassUnsupportedRuntime
}

// ManagedRuntimeFromSnapshot projects one host readiness snapshot into the public contract.
func ManagedRuntimeFromSnapshot(snapshot ReadinessSnapshot) managedruntime.Runtime {
	diagnostics := make(map[string]string, len(snapshot.Diagnostics)+1)
	for key, value := range snapshot.Diagnostics {
		diagnostics[key] = value
	}
	if snapshot.FailureClass != FailureClassNone {
		diagnostics["failureClass"] = string(snapshot.FailureClass)
	}
	var operations []managedruntime.Operation
	if snapshot.Identity.SupportedOperations != nil {
		operations = make([]managedruntime.Operation, len(snapshot.Identity.SupportedOperations))
		for index, operation := range snapshot.Identity.SupportedOperations {
			operations[index] = operation
			operations[index].Inputs = cloneOperationSlots(operation.Inputs)
			operations[index].Outputs = cloneOperationSlots(operation.Outputs)
		}
	}
	return managedruntime.Runtime{
		Identity: snapshot.Identity.Name, ReadinessState: snapshot.ReadinessState,
		LifecycleState: snapshot.LifecycleState, Locality: snapshot.Identity.Locality,
		SupportedOperations: operations, Diagnostics: diagnostics,
	}
}

func cloneOperationSlots(slots []managedruntime.OperationSlot) []managedruntime.OperationSlot {
	if slots == nil {
		return nil
	}
	cloned := make([]managedruntime.OperationSlot, len(slots))
	for index, slot := range slots {
		cloned[index] = slot
		cloned[index].ContentTypes = append([]string(nil), slot.ContentTypes...)
		if slot.Required != nil {
			required := *slot.Required
			cloned[index].Required = &required
		}
	}
	return cloned
}

// ClassifyReadiness maps cache inspection and catalog identity into host readiness.
func ClassifyReadiness(identity Identity, inspection CacheInspection, unsupported bool) ReadinessSnapshot {
	if unsupported {
		return ReadinessSnapshot{
			Identity:       identity,
			ReadinessState: managedruntime.ReadinessStateUnsupported,
			LifecycleState: managedruntime.LifecycleStateNotApplicable,
			FailureClass:   FailureClassUnsupportedRuntime,
			Diagnostics:    managedDiagnostics(identity, managedruntime.ReadinessStateUnsupported, managedruntime.LifecycleStateNotApplicable),
		}
	}
	if inspection.Supported {
		readiness, lifecycle, failureClass := readinessFromCacheInspection(inspection)
		return ReadinessSnapshot{
			Identity:       identity,
			ReadinessState: readiness,
			LifecycleState: lifecycle,
			FailureClass:   failureClass,
			Diagnostics:    mergeDiagnostics(identity, readiness, lifecycle, cacheDiagnostics(inspection)),
		}
	}
	readiness := readinessFromLocality(identity.Locality)
	lifecycle := lifecycleFromLocality(identity.Locality)
	return ReadinessSnapshot{
		Identity:       identity,
		ReadinessState: readiness,
		LifecycleState: lifecycle,
		FailureClass:   FailureClassForReadinessState(readiness),
		Diagnostics:    managedDiagnostics(identity, readiness, lifecycle),
	}
}

func readinessFromCacheInspection(inspection CacheInspection) (
	managedruntime.ReadinessState,
	managedruntime.LifecycleState,
	FailureClass,
) {
	if inspection.Installed {
		return managedruntime.ReadinessStateReady,
			managedruntime.LifecycleStateInstalled,
			FailureClassNone
	}
	if inspection.PartialArtifacts && inspection.InstalledFileCount == 0 {
		return managedruntime.ReadinessStateFailed,
			managedruntime.LifecycleStateNotInstalled,
			FailureClassMissingAssets
	}
	if inspection.InstalledFileCount > 0 || inspection.PartialArtifacts {
		return managedruntime.ReadinessStateLoading,
			managedruntime.LifecycleStateInstalling,
			FailureClassLoadingTimeout
	}
	return managedruntime.ReadinessStateMissing,
		managedruntime.LifecycleStateNotInstalled,
		FailureClassMissingAssets
}

func readinessFromLocality(locality managedruntime.Locality) managedruntime.ReadinessState {
	switch locality {
	case managedruntime.LocalityLocal:
		return managedruntime.ReadinessStateMissing
	default:
		return managedruntime.ReadinessStateReady
	}
}

func lifecycleFromLocality(locality managedruntime.Locality) managedruntime.LifecycleState {
	switch locality {
	case managedruntime.LocalityLocal:
		return managedruntime.LifecycleStateNotInstalled
	default:
		return managedruntime.LifecycleStateNotApplicable
	}
}

func managedDiagnostics(
	identity Identity,
	readiness managedruntime.ReadinessState,
	lifecycle managedruntime.LifecycleState,
) map[string]string {
	diagnostics := map[string]string{
		"readinessState": string(readiness),
		"lifecycleState": string(lifecycle),
		"locality":       string(identity.Locality),
	}
	for key, value := range sourceDiagnostics(identity) {
		diagnostics[key] = value
	}
	if identity.Name != "" {
		diagnostics["identity"] = identity.Name
	}
	return diagnostics
}

func mergeDiagnostics(
	identity Identity,
	readiness managedruntime.ReadinessState,
	lifecycle managedruntime.LifecycleState,
	extra map[string]string,
) map[string]string {
	diagnostics := managedDiagnostics(identity, readiness, lifecycle)
	for key, value := range extra {
		diagnostics[key] = value
	}
	return diagnostics
}

func cloneOperations(operations []managedruntime.Operation) []managedruntime.Operation {
	cloned := make([]managedruntime.Operation, len(operations))
	for index, operation := range operations {
		cloned[index] = operation
		cloned[index].Inputs = cloneOperationSlots(operation.Inputs)
		cloned[index].Outputs = cloneOperationSlots(operation.Outputs)
	}
	return cloned
}

func sourceDiagnostics(identity Identity) map[string]string {
	if identity.SourceKind == "" {
		return nil
	}
	return map[string]string{
		"sourceKind":    identity.SourceKind,
		"sourceId":      identity.SourceID,
		"resolverNotes": identity.ResolverNotes,
	}
}

func cacheDiagnostics(inspection CacheInspection) map[string]string {
	if !inspection.Supported {
		return nil
	}
	diagnostics := make(map[string]string)
	if len(inspection.MissingAssets) > 0 {
		diagnostics["missingAssets"] = strings.Join(inspection.MissingAssets, ",")
	}
	if inspection.Revision != "" {
		diagnostics["revision"] = inspection.Revision
	}
	if inspection.CachePath != "" {
		diagnostics["cachePath"] = inspection.CachePath
	}
	if inspection.Installed {
		diagnostics["installedFileCount"] = strconv.Itoa(inspection.InstalledFileCount)
	}
	return diagnostics
}
