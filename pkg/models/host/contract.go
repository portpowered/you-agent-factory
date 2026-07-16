package modelhost

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
	Locality            factoryapi.WorkerModelLocality
	SupportedOperations []factoryapi.ModelOperation
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
	PullOutcome     factoryapi.ManagedRuntimePullOutcome
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
func ReadinessStateForFailureClass(class FailureClass) factoryapi.ManagedRuntimeReadinessState {
	switch class {
	case FailureClassMissingAssets:
		return factoryapi.ManagedRuntimeReadinessStateMISSING
	case FailureClassLoadingTimeout:
		return factoryapi.ManagedRuntimeReadinessStateLOADING
	case FailureClassProcessCrash:
		return factoryapi.ManagedRuntimeReadinessStateFAILED
	case FailureClassCancelled:
		return factoryapi.ManagedRuntimeReadinessStateFAILED
	case FailureClassCapacityExhausted:
		return factoryapi.ManagedRuntimeReadinessStateFAILED
	case FailureClassUnsupportedRuntime:
		return factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED
	default:
		return factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED
	}
}

// FailureClassForReadinessState derives the primary failure class for a readiness state.
func FailureClassForReadinessState(readiness factoryapi.ManagedRuntimeReadinessState) FailureClass {
	switch readiness {
	case factoryapi.ManagedRuntimeReadinessStateREADY:
		return FailureClassNone
	case factoryapi.ManagedRuntimeReadinessStateMISSING:
		return FailureClassMissingAssets
	case factoryapi.ManagedRuntimeReadinessStateLOADING:
		return FailureClassLoadingTimeout
	case factoryapi.ManagedRuntimeReadinessStateFAILED:
		return FailureClassProcessCrash
	case factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED:
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
func ManagedRuntimeFromSnapshot(snapshot ReadinessSnapshot) factoryapi.ManagedRuntime {
	diagnostics := factoryapi.StringMap{}
	for key, value := range snapshot.Diagnostics {
		diagnostics[key] = value
	}
	if snapshot.FailureClass != FailureClassNone {
		diagnostics["failureClass"] = string(snapshot.FailureClass)
	}
	return factoryapi.ManagedRuntime{
		Identity:            snapshot.Identity.Name,
		ReadinessState:      snapshot.ReadinessState,
		LifecycleState:      snapshot.LifecycleState,
		Locality:            snapshot.Identity.Locality,
		SupportedOperations: append([]factoryapi.ModelOperation(nil), snapshot.Identity.SupportedOperations...),
		Diagnostics:         &diagnostics,
	}
}

// ClassifyReadiness maps cache inspection and catalog identity into host readiness.
func ClassifyReadiness(identity Identity, inspection CacheInspection, unsupported bool) ReadinessSnapshot {
	if unsupported {
		return ReadinessSnapshot{
			Identity:       identity,
			ReadinessState: factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED,
			LifecycleState: factoryapi.ManagedRuntimeLifecycleStateNOTAPPLICABLE,
			FailureClass:   FailureClassUnsupportedRuntime,
			Diagnostics:    managedDiagnostics(identity, factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED, factoryapi.ManagedRuntimeLifecycleStateNOTAPPLICABLE),
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
	factoryapi.ManagedRuntimeReadinessState,
	factoryapi.ManagedRuntimeLifecycleState,
	FailureClass,
) {
	if inspection.Installed {
		return factoryapi.ManagedRuntimeReadinessStateREADY,
			factoryapi.ManagedRuntimeLifecycleStateINSTALLED,
			FailureClassNone
	}
	if inspection.PartialArtifacts && inspection.InstalledFileCount == 0 {
		return factoryapi.ManagedRuntimeReadinessStateFAILED,
			factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			FailureClassMissingAssets
	}
	if inspection.InstalledFileCount > 0 || inspection.PartialArtifacts {
		return factoryapi.ManagedRuntimeReadinessStateLOADING,
			factoryapi.ManagedRuntimeLifecycleStateINSTALLING,
			FailureClassLoadingTimeout
	}
	return factoryapi.ManagedRuntimeReadinessStateMISSING,
		factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
		FailureClassMissingAssets
}

func readinessFromLocality(locality factoryapi.WorkerModelLocality) factoryapi.ManagedRuntimeReadinessState {
	switch locality {
	case factoryapi.WorkerModelLocalityLocal:
		return factoryapi.ManagedRuntimeReadinessStateMISSING
	default:
		return factoryapi.ManagedRuntimeReadinessStateREADY
	}
}

func lifecycleFromLocality(locality factoryapi.WorkerModelLocality) factoryapi.ManagedRuntimeLifecycleState {
	switch locality {
	case factoryapi.WorkerModelLocalityLocal:
		return factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED
	default:
		return factoryapi.ManagedRuntimeLifecycleStateNOTAPPLICABLE
	}
}

func managedDiagnostics(
	identity Identity,
	readiness factoryapi.ManagedRuntimeReadinessState,
	lifecycle factoryapi.ManagedRuntimeLifecycleState,
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
	readiness factoryapi.ManagedRuntimeReadinessState,
	lifecycle factoryapi.ManagedRuntimeLifecycleState,
	extra map[string]string,
) map[string]string {
	diagnostics := managedDiagnostics(identity, readiness, lifecycle)
	for key, value := range extra {
		diagnostics[key] = value
	}
	return diagnostics
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
