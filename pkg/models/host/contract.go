package modelhost

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
)

var (
	// ErrInvalidDependencies classifies model-host construction failures.
	ErrInvalidDependencies = errors.New("model host dependencies are invalid")
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

// Dependencies carries the required process, pull, and cache edges for a
// catalog-backed model host. Source resolution, health checking, server-start
// building, diagnostics, and lease policy are package-local optional behavior
// configured through Options.
type Dependencies struct {
	AssetPuller     AssetPuller
	CacheInspector  CacheInspector
	ProcessLauncher ProcessLauncher
	Options         Options
}

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
	Name                string
	Locality            managedruntime.Locality
	SupportedOperations []managedruntime.Operation
	Backend             string
	LoadPolicy          string
	SourceKind          string
	SourceID            string
	ResolverNotes       string
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
	ReadinessState managedruntime.ReadinessState
	LifecycleState managedruntime.LifecycleState
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
	PullOutcome     managedruntime.PullOutcome
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
	Diagnostics       Diagnostics
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
	PullOutcome     managedruntime.PullOutcome
	Snapshot        ReadinessSnapshot
	LegacyOutcome   string
	CachePath       string
	Revision        string
	DownloadedFiles []PullDownloadedFile
}

// AssetPuller performs managed-runtime asset pulls for the model host boundary.
type AssetPuller interface {
	PullModel(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (AssetPullResult, error)
}

// CacheInspector inspects installed managed-runtime assets for the model host boundary.
type CacheInspector interface {
	InspectRuntimeCache(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (CacheInspection, error)
}

// AssetGateway is the legacy combined pull and cache boundary.
type AssetGateway interface {
	AssetPuller
	CacheInspector
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

// FailureClass is a provider-neutral outcome for model host operations.
type FailureClass string

const (
	FailureClassNone               FailureClass = ""
	FailureClassMissingAssets      FailureClass = "missing_assets"
	FailureClassLoadingTimeout     FailureClass = "loading_timeout"
	FailureClassProcessCrash       FailureClass = "process_crash"
	FailureClassUnsupportedRuntime FailureClass = "unsupported_runtime"
	FailureClassCancelled          FailureClass = "cancelled"
	FailureClassCapacityExhausted  FailureClass = "capacity_exhausted"
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
	diagnostics := map[string]string{}
	for key, value := range snapshot.Diagnostics {
		diagnostics[key] = value
	}
	if snapshot.FailureClass != FailureClassNone {
		diagnostics["failureClass"] = string(snapshot.FailureClass)
	}
	return managedruntime.Runtime{
		Identity:            snapshot.Identity.Name,
		ReadinessState:      snapshot.ReadinessState,
		LifecycleState:      snapshot.LifecycleState,
		Locality:            snapshot.Identity.Locality,
		SupportedOperations: cloneOperations(snapshot.Identity.SupportedOperations),
		Diagnostics:         diagnostics,
	}
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

func cloneOperationSlots(slots []managedruntime.OperationSlot) []managedruntime.OperationSlot {
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
