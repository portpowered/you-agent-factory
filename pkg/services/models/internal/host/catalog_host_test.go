package modelhost

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	apisurface "github.com/portpowered/infinite-you/pkg/services/models"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
)

type testHostOptions struct {
	SourceResolver    SourceResolver
	Supervisor        SupervisorConfig
	Diagnostics       Diagnostics
	IdleUnloadAfter   time.Duration
	MaxLoadedRuntimes int
}

type testHostDependencies struct {
	AssetPuller     AssetPuller
	CacheInspector  CacheInspector
	ProcessLauncher ProcessLauncher
	Options         testHostOptions
}

func newTestHost(deps testHostDependencies) (*CatalogHost, error) {
	if deps.Options.SourceResolver == nil {
		deps.Options.SourceResolver = DefaultManagedRuntimeSourceResolverAdapter()
	}
	if deps.Options.Supervisor.ReadinessTimeout <= 0 {
		deps.Options.Supervisor.ReadinessTimeout = DefaultReadinessTimeout
	}
	if deps.Options.Supervisor.HealthCheckInterval <= 0 {
		deps.Options.Supervisor.HealthCheckInterval = DefaultHealthCheckInterval
	}
	if deps.Options.Supervisor.HealthCheckPath == "" {
		deps.Options.Supervisor.HealthCheckPath = DefaultHealthCheckPath
	}
	if deps.Options.Supervisor.HealthChecker == nil {
		deps.Options.Supervisor.HealthChecker = alwaysHealthyChecker{}
	}
	if deps.Options.Supervisor.Clock == nil {
		deps.Options.Supervisor.Clock = hostTestClock{}
	}
	if deps.Options.Supervisor.ServerStartBuilder == nil {
		deps.Options.Supervisor.ServerStartBuilder = DefaultServerStartBuilder
	}
	return NewHost(
		deps.AssetPuller,
		deps.CacheInspector,
		deps.ProcessLauncher,
		deps.Options.SourceResolver,
		deps.Options.Supervisor.ReadinessTimeout,
		deps.Options.Supervisor.HealthCheckInterval,
		deps.Options.Supervisor.HealthCheckPath,
		deps.Options.Supervisor.HealthChecker,
		deps.Options.Supervisor.Clock,
		deps.Options.Supervisor.ServerStartBuilder,
		deps.Options.Diagnostics,
		deps.Options.IdleUnloadAfter,
		deps.Options.MaxLoadedRuntimes,
	)
}

func mustNewCatalogHost(t *testing.T, assets AssetGateway, opts testHostOptions) *CatalogHost {
	t.Helper()
	launcher := opts.Supervisor.ProcessLauncher
	if launcher == nil {
		launcher = &fakeProcessLauncher{}
	}
	host, err := newTestHost(testHostDependencies{
		AssetPuller: assets, CacheInspector: assets,
		ProcessLauncher: launcher, Options: opts,
	})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	return host
}

func TestNewHost_ValidatesRequiredDependenciesWithoutLaunchingProcess(t *testing.T) {
	assets := stubAssetGateway{}
	launcher := &fakeProcessLauncher{}

	tests := []struct {
		name string
		deps testHostDependencies
		want string
	}{
		{
			name: "asset puller",
			deps: testHostDependencies{CacheInspector: assets, ProcessLauncher: launcher},
			want: "asset puller is required",
		},
		{
			name: "cache inspector",
			deps: testHostDependencies{AssetPuller: assets, ProcessLauncher: launcher},
			want: "cache inspector is required",
		},
		{
			name: "process launcher",
			deps: testHostDependencies{AssetPuller: assets, CacheInspector: assets},
			want: "process launcher is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			host, err := newTestHost(tc.deps)
			if host != nil {
				t.Fatal("host constructed with a missing required dependency")
			}
			if !errors.Is(err, ErrInvalidDependencies) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want classified error containing %q", err, tc.want)
			}
		})
	}

	if len(launcher.starts) != 0 {
		t.Fatalf("process starts during validation = %d, want 0", len(launcher.starts))
	}
}

type hostTestClock struct{}

func (hostTestClock) Now() time.Time { return time.Now() }
func (hostTestClock) NewTimer(duration time.Duration) Timer {
	return hostTestTimer{Timer: time.NewTimer(duration)}
}

type hostTestTimer struct{ *time.Timer }

func (timer hostTestTimer) C() <-chan time.Time { return timer.Timer.C }

func TestNewHost_UsesExplicitPullCacheAndProcessEdges(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, supervisedCatalogFactoryConfig())
	assets := &recordingConstructorAssets{
		inspection: CacheInspection{
			Supported:          true,
			Installed:          true,
			InstalledFileCount: 2,
			CachePath:          t.TempDir(),
		},
	}
	launcher := &fakeProcessLauncher{
		newProcess: func(spec ProcessStartSpec) *fakeManagedProcess {
			return newFakeManagedProcess(spec.HealthEndpoint, nil)
		},
	}
	host, err := newTestHost(testHostDependencies{
		AssetPuller:     assets,
		CacheInspector:  assets,
		ProcessLauncher: launcher,
		Options: testHostOptions{Supervisor: SupervisorConfig{
			ReadinessTimeout:    100 * time.Millisecond,
			HealthCheckInterval: time.Millisecond,
			HealthChecker:       alwaysHealthyChecker{},
		}},
	})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	if len(launcher.starts) != 0 {
		t.Fatalf("process starts during construction = %d, want 0", len(launcher.starts))
	}

	if _, err := host.Pull(context.Background(), loaded, "OMNIVOICE_Q4_K_M"); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	lease, err := host.AcquireLease(context.Background(), loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	if assets.pulls != 1 || assets.inspections == 0 {
		t.Fatalf("edge calls = pulls:%d inspections:%d, want supplied pull and cache edges", assets.pulls, assets.inspections)
	}
	if len(launcher.starts) != 1 {
		t.Fatalf("process starts after lease = %d, want 1", len(launcher.starts))
	}
	if err := host.ReleaseLease(context.Background(), lease.ID); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestNewHost_ClassifiesSuppliedProcessLaunchFailure(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, supervisedCatalogFactoryConfig())
	assets := stubAssetGateway{byModel: map[string]CacheInspection{
		"OMNIVOICE_Q4_K_M": {Supported: true, Installed: true, InstalledFileCount: 2, CachePath: t.TempDir()},
	}}
	host, err := newTestHost(testHostDependencies{
		AssetPuller:     assets,
		CacheInspector:  assets,
		ProcessLauncher: &fakeProcessLauncher{},
	})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}

	_, err = host.AcquireLease(context.Background(), loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	if !errors.Is(err, ErrProcessCrash) || FailureClassFromError(err) != FailureClassProcessCrash {
		t.Fatalf("AcquireLease error = %v, want process_crash classification", err)
	}
}

type recordingConstructorAssets struct {
	pulls       int
	inspections int
	inspection  CacheInspection
}

func (a *recordingConstructorAssets) PullModel(context.Context, *modelRuntimeConfig, string) (AssetPullResult, error) {
	a.pulls++
	return AssetPullResult{
		PullOutcome: managedruntime.PullOutcomeInstalledSuccessfully,
		Snapshot: ReadinessSnapshot{
			Identity:       Identity{Name: "OMNIVOICE_Q4_K_M", Locality: managedruntime.LocalityLocal},
			ReadinessState: managedruntime.ReadinessStateReady,
			LifecycleState: managedruntime.LifecycleStateInstalled,
		},
	}, nil
}

func (a *recordingConstructorAssets) InspectRuntimeCache(context.Context, *modelRuntimeConfig, string) (CacheInspection, error) {
	a.inspections++
	return a.inspection, nil
}

func TestClassifyReadiness_CoversReadyMissingLoadingFailedUnsupported(t *testing.T) {
	identity := Identity{
		Name:     "OMNIVOICE_Q4_K_M",
		Locality: managedruntime.LocalityLocal,
	}

	cases := []struct {
		name        string
		inspection  CacheInspection
		unsupported bool
		readiness   managedruntime.ReadinessState
		lifecycle   managedruntime.LifecycleState
		failure     FailureClass
	}{
		{
			name: "ready",
			inspection: CacheInspection{
				Supported:          true,
				Installed:          true,
				InstalledFileCount: 2,
			},
			readiness: managedruntime.ReadinessStateReady,
			lifecycle: managedruntime.LifecycleStateInstalled,
			failure:   FailureClassNone,
		},
		{
			name: "missing",
			inspection: CacheInspection{
				Supported:     true,
				MissingAssets: []string{"model.gguf"},
			},
			readiness: managedruntime.ReadinessStateMissing,
			lifecycle: managedruntime.LifecycleStateNotInstalled,
			failure:   FailureClassMissingAssets,
		},
		{
			name: "loading",
			inspection: CacheInspection{
				Supported:          true,
				InstalledFileCount: 1,
			},
			readiness: managedruntime.ReadinessStateLoading,
			lifecycle: managedruntime.LifecycleStateInstalling,
			failure:   FailureClassLoadingTimeout,
		},
		{
			name: "failed",
			inspection: CacheInspection{
				Supported:        true,
				PartialArtifacts: true,
			},
			readiness: managedruntime.ReadinessStateFailed,
			lifecycle: managedruntime.LifecycleStateNotInstalled,
			failure:   FailureClassMissingAssets,
		},
		{
			name:        "unsupported",
			unsupported: true,
			readiness:   managedruntime.ReadinessStateUnsupported,
			lifecycle:   managedruntime.LifecycleStateNotApplicable,
			failure:     FailureClassUnsupportedRuntime,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := ClassifyReadiness(identity, tc.inspection, tc.unsupported)
			if snapshot.ReadinessState != tc.readiness {
				t.Fatalf("readiness = %s, want %s", snapshot.ReadinessState, tc.readiness)
			}
			if snapshot.LifecycleState != tc.lifecycle {
				t.Fatalf("lifecycle = %s, want %s", snapshot.LifecycleState, tc.lifecycle)
			}
			if snapshot.FailureClass != tc.failure {
				t.Fatalf("failure class = %s, want %s", snapshot.FailureClass, tc.failure)
			}
		})
	}
}

func TestFailureClassFromError_ClassifiesCancelled(t *testing.T) {
	if got := FailureClassFromError(errors.Join(ErrCancelled, context.Canceled)); got != FailureClassCancelled {
		t.Fatalf("failure class = %s, want %s", got, FailureClassCancelled)
	}
}

func TestFailureClassForReadinessState_MapsPublicContractStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		readiness managedruntime.ReadinessState
		want      FailureClass
	}{
		{name: "ready", readiness: managedruntime.ReadinessStateReady, want: FailureClassNone},
		{name: "missing", readiness: managedruntime.ReadinessStateMissing, want: FailureClassMissingAssets},
		{name: "loading", readiness: managedruntime.ReadinessStateLoading, want: FailureClassLoadingTimeout},
		{name: "failed", readiness: managedruntime.ReadinessStateFailed, want: FailureClassProcessCrash},
		{name: "unsupported", readiness: managedruntime.ReadinessStateUnsupported, want: FailureClassUnsupportedRuntime},
		{name: "unknown", readiness: managedruntime.ReadinessState("UNKNOWN"), want: FailureClassUnsupportedRuntime},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := FailureClassForReadinessState(test.readiness); got != test.want {
				t.Fatalf("failure class = %s, want %s", got, test.want)
			}
		})
	}
}

func TestManagedRuntimeFromSnapshot_PreservesPublicVocabulary(t *testing.T) {
	snapshot := ReadinessSnapshot{
		Identity: Identity{
			Name:     "OMNIVOICE_Q4_K_M",
			Locality: managedruntime.LocalityLocal,
			SupportedOperations: []managedruntime.Operation{{
				Name: "TTS",
			}},
		},
		ReadinessState: managedruntime.ReadinessStateReady,
		LifecycleState: managedruntime.LifecycleStateInstalled,
		FailureClass:   FailureClassNone,
		Diagnostics: map[string]string{
			"readinessState": "READY",
			"lifecycleState": "INSTALLED",
			"locality":       "LOCAL",
		},
	}

	managed := ManagedRuntimeFromSnapshot(snapshot)
	if managed.Identity != snapshot.Identity.Name {
		t.Fatalf("identity = %q, want %q", managed.Identity, snapshot.Identity.Name)
	}
	if managed.ReadinessState != managedruntime.ReadinessStateReady {
		t.Fatalf("readiness = %s, want READY", managed.ReadinessState)
	}
	if managed.LifecycleState != managedruntime.LifecycleStateInstalled {
		t.Fatalf("lifecycle = %s, want INSTALLED", managed.LifecycleState)
	}
	if len(managed.SupportedOperations) != 1 || managed.SupportedOperations[0].Name != "TTS" {
		t.Fatalf("operations = %#v, want one TTS operation", managed.SupportedOperations)
	}
}

func TestCatalogHost_InspectReadinessAndLeaseLifecycle(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	host := mustNewCatalogHost(t, stubAssetGateway{
		byModel: map[string]CacheInspection{
			"OMNIVOICE_Q4_K_M": {
				Supported:          true,
				Installed:          true,
				InstalledFileCount: 2,
			},
		},
	}, testHostOptions{SourceResolver: DefaultManagedRuntimeSourceResolverAdapter()})

	ctx := context.Background()
	ready, err := host.InspectReadiness(ctx, loaded, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("InspectReadiness: %v", err)
	}
	if ready.ReadinessState != managedruntime.ReadinessStateReady {
		t.Fatalf("ready state = %s, want READY", ready.ReadinessState)
	}

	lease, err := host.AcquireLease(ctx, loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{Holder: "test"})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	if lease.ID == "" {
		t.Fatal("lease id is empty")
	}
	if err := host.Unload(ctx, loaded, "OMNIVOICE_Q4_K_M"); !errors.Is(err, ErrCapacityExhausted) {
		t.Fatalf("Unload with active lease = %v, want capacity exhausted", err)
	}
	if err := host.ReleaseLease(ctx, lease.ID); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
	if err := host.Unload(ctx, loaded, "OMNIVOICE_Q4_K_M"); err != nil {
		t.Fatalf("Unload after release: %v", err)
	}
}

func TestCatalogHost_BlocksLeaseForNonReadyStates(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	host := mustNewCatalogHost(t, stubAssetGateway{
		byModel: map[string]CacheInspection{
			"OMNIVOICE_Q4_K_M": {Supported: true, InstalledFileCount: 1},
		},
	}, testHostOptions{})

	_, err := host.AcquireLease(context.Background(), loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	var readinessErr *ReadinessError
	if !errors.As(err, &readinessErr) {
		t.Fatalf("error = %v, want *ReadinessError", err)
	}
	if readinessErr.Snapshot.ReadinessState != managedruntime.ReadinessStateLoading {
		t.Fatalf("readiness = %s, want LOADING", readinessErr.Snapshot.ReadinessState)
	}
	if !errors.Is(err, ErrLoadingTimeout) {
		t.Fatalf("error = %v, want ErrLoadingTimeout", err)
	}
}

func TestCatalogHost_InspectReadinessHonoursCancellation(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	host := mustNewCatalogHost(t, stubAssetGateway{}, testHostOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := host.InspectReadiness(ctx, loaded, "OMNIVOICE_Q4_K_M")
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("error = %v, want ErrCancelled", err)
	}
	if FailureClassFromError(err) != FailureClassCancelled {
		t.Fatalf("failure class = %s, want cancelled", FailureClassFromError(err))
	}
}

type stubAssetGateway struct {
	byModel map[string]CacheInspection
}

func (s stubAssetGateway) PullModel(context.Context, *modelRuntimeConfig, string) (AssetPullResult, error) {
	return AssetPullResult{}, apisurface.ErrNotFound
}

func (s stubAssetGateway) InspectRuntimeCache(_ context.Context, _ *modelRuntimeConfig, modelName string) (CacheInspection, error) {
	if inspection, ok := s.byModel[modelName]; ok {
		return inspection, nil
	}
	return CacheInspection{}, nil
}

func mustLoadedCatalogConfig(t *testing.T, factoryCfg *testFactoryConfig) *modelRuntimeConfig {
	t.Helper()
	return projectTestModelsRuntimeConfig(t.TempDir(), factoryCfg)
}

func catalogFactoryConfig(includeResource bool) *testFactoryConfig {
	worker := modelRuntimeWorker{
		Name:          "voice-local",
		Type:          apisurface.RuntimeWorkerTypeModel,
		Model:         "OMNIVOICE_Q4_K_M",
		ModelLocality: apisurface.RuntimeModelLocalityLocal,
		Operations: []apisurface.RuntimeOperation{{
			Name: "TTS",
		}},
	}
	if includeResource {
		worker.Resources = []modelRuntimeResource{{Name: "omnivoice-cache", Capacity: 1}}
	}
	cfg := &testFactoryConfig{
		Name:    "factory",
		Workers: []modelRuntimeWorker{worker},
	}
	if includeResource {
		cfg.Resources = []modelRuntimeResource{{
			Name:       "omnivoice-cache",
			Type:       apisurface.RuntimeResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "GGUF",
			LoadPolicy: "ON_DEMAND",
		}}
	}
	return cfg
}
