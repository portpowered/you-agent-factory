package local

import (
	"context"
	"errors"
	"testing"

	apisurface "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestPullModelWithOptions_ProjectsManagedRuntimeOutcomes(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	puller := &managedPullTestAssetPuller{
		result: apisurface.PullResult{
			ModelName:        "OMNIVOICE_Q4_K_M",
			ProviderLocality: apisurface.RuntimeModelLocalityLocal,
			Outcome:          legacyPullOutcomeAlreadyPresent,
			CachePath:        "/tmp/models/OMNIVOICE_Q4_K_M/rev1",
			Revision:         "rev1",
		},
	}
	opts := PullOptions{
		RuntimeCacheInspector: stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
			"OMNIVOICE_Q4_K_M": {
				Supported:          true,
				Installed:          true,
				Revision:           "rev1",
				CachePath:          "/tmp/models/OMNIVOICE_Q4_K_M/rev1",
				InstalledFileCount: 2,
			},
		}},
		SourceResolver: DefaultManagedRuntimeSourceResolver(),
	}

	result, err := PullModelWithOptions(puller, context.Background(), loaded, "OMNIVOICE_Q4_K_M", opts)
	if err != nil {
		t.Fatalf("PullModelWithOptions: %v", err)
	}
	if result.ManagedPullOutcome != managedPullOutcomeAlreadyReady {
		t.Fatalf("managed pull outcome = %q, want ALREADY_READY", result.ManagedPullOutcome)
	}
	if result.ReadinessState != managedReadinessReady || result.LifecycleState != managedLifecycleInstalled {
		t.Fatalf("readiness/lifecycle = (%q, %q), want READY INSTALLED", result.ReadinessState, result.LifecycleState)
	}
	if result.SourceKind != ManagedRuntimeSourceKindUpstreamRepository {
		t.Fatalf("source kind = %q, want upstream repository", result.SourceKind)
	}
}

func TestPullModelWithOptions_ClassifiesUnsupportedLocalModel(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, &testFactoryConfig{
		Name: "factory",
		Workers: []modelRuntimeWorker{{
			Name:          "cloud-model",
			Type:          apisurface.RuntimeWorkerTypeModel,
			Model:         "CLOUD_ONLY",
			ModelLocality: apisurface.RuntimeModelLocalityCloud,
			Operations:    []apisurface.RuntimeOperation{{Name: "TTS"}},
		}},
	})
	_, err := PullModelWithOptions(&managedPullTestAssetPuller{}, context.Background(), loaded, "CLOUD_ONLY", PullOptions{})
	if !errors.Is(err, apisurface.ErrPullUnsupported) {
		t.Fatalf("PullModelWithOptions error = %v, want ErrModelPullUnsupported", err)
	}
}

func TestPullModelWithOptions_UsesCanonicalResolvedFallback(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		apisurface.BuiltInModelNameASR,
		apisurface.BuiltInModelNameTTS,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			definition, ok := (apisurface.BuiltInCatalog{}).ModelDefinitionFor(name)
			if !ok {
				t.Fatalf("built-in definition %q is missing", name)
			}
			puller := &managedPullTestAssetPuller{result: apisurface.PullResult{
				ModelName: name,
				Outcome:   legacyPullOutcomePulled,
			}}
			resolved := &apisurface.ResolvedModelReference{
				Definition: definition,
			}

			result, err := PullModelWithOptions(
				puller,
				context.Background(),
				&apisurface.RuntimeConfig{},
				name,
				PullOptions{ResolvedReference: resolved},
			)
			if err != nil {
				t.Fatalf("PullModelWithOptions(%q): %v", name, err)
			}
			if result.ManagedPullOutcome != managedPullOutcomeInstalledSuccessfully ||
				result.ReadinessState != managedReadinessReady {
				t.Fatalf("pull result = %#v, want successful managed-runtime projection", result)
			}
			if len(puller.calls) != 1 || puller.calls[0] != name {
				t.Fatalf("pull calls = %#v, want one call for %q", puller.calls, name)
			}
		})
	}
}

func TestPullModelWithOptions_UnknownModelStillReturnsCatalogMiss(t *testing.T) {
	t.Parallel()

	puller := &managedPullTestAssetPuller{}
	_, err := PullModelWithOptions(
		puller,
		context.Background(),
		&apisurface.RuntimeConfig{},
		"unknown-model",
		PullOptions{},
	)
	if !errors.Is(err, apisurface.ErrNotFound) {
		t.Fatalf("unknown pull error = %v, want ErrNotFound", err)
	}
	if len(puller.calls) != 0 {
		t.Fatalf("unknown pull calls = %#v, want none", puller.calls)
	}
}

func TestPullModelWithOptions_ReportsVerificationFailure(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	verificationErr := apisurface.ErrAssetIntegrityFailed
	result, err := PullModelWithOptions(
		&managedPullTestAssetPuller{result: apisurface.PullResult{
			ModelName: "OMNIVOICE_Q4_K_M", Outcome: legacyPullOutcomePulled,
		}},
		context.Background(), loaded, "OMNIVOICE_Q4_K_M",
		PullOptions{
			RuntimeCacheInspector: stubRuntimeCacheInspector{err: verificationErr},
		},
	)
	if !errors.Is(err, verificationErr) {
		t.Fatalf("PullModelWithOptions error = %v, want verification failure", err)
	}
	var pullErr *apisurface.PullError
	if !errors.As(err, &pullErr) || result.ManagedPullOutcome != managedPullOutcomeIntegrityVerificationFailed ||
		result.Outcome != legacyPullOutcomeFailed || result.ReadinessState != managedReadinessFailed ||
		result.LifecycleState != managedLifecycleNotInstalled ||
		result.FailureStage != apisurface.PullStageIntegrityVerification {
		t.Fatalf("pull result = %#v, error = %v, want classified terminal verification failure", result, err)
	}
}

func TestPullModelWithOptions_ClassifiesPostDownloadCacheFailure(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	postDownloadErr := apisurface.WrapPullStage(
		apisurface.PullStageCacheInstallation,
		"OMNIVOICE_Q4_K_M",
		"resolve managed runtime cache",
		"",
		apisurface.ErrNotAvailable,
	)
	result, err := PullModelWithOptions(
		&managedPullTestAssetPuller{
			result: apisurface.PullResult{
				ModelName: "OMNIVOICE_Q4_K_M", Outcome: legacyPullOutcomePulled,
				DownloadedFiles: []apisurface.DownloadedFile{{
					Path: "model.gguf", Bytes: 4, SHA256: "deadbeef",
				}},
			},
			err: postDownloadErr,
		},
		context.Background(), loaded, "OMNIVOICE_Q4_K_M", PullOptions{},
	)
	if !errors.Is(err, apisurface.ErrNotAvailable) {
		t.Fatalf("PullModelWithOptions error = %v, want post-download cache failure", err)
	}
	var pullErr *apisurface.PullError
	var stageErr *apisurface.PullStageError
	if !errors.As(err, &pullErr) || !errors.As(err, &stageErr) ||
		result.ManagedPullOutcome != managedPullOutcomeCacheInstallationFailed ||
		result.ManagedPullOutcome == managedPullOutcomeSourceFetchFailed ||
		result.FailureStage != apisurface.PullStageCacheInstallation || stageErr.Cause == nil ||
		len(result.DownloadedFiles) != 1 {
		t.Fatalf("pull result = %#v, error = %v, want cache-installation failure with downloaded file facts", result, err)
	}
}

func TestPullModelWithOptions_ReportsSourceFetchFailureWithoutSuccessProjection(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	result, err := PullModelWithOptions(
		&managedPullTestAssetPuller{
			result: apisurface.PullResult{
				ModelName: "OMNIVOICE_Q4_K_M", Outcome: legacyPullOutcomePulled,
			},
			err: apisurface.ErrSourceFetchFailed,
		},
		context.Background(), loaded, "OMNIVOICE_Q4_K_M", PullOptions{},
	)
	if !errors.Is(err, apisurface.ErrSourceFetchFailed) {
		t.Fatalf("PullModelWithOptions error = %v, want source-fetch failure", err)
	}
	var pullErr *apisurface.PullError
	if !errors.As(err, &pullErr) || result.Outcome != legacyPullOutcomeFailed ||
		result.ManagedPullOutcome != managedPullOutcomeSourceFetchFailed ||
		result.ReadinessState != managedReadinessFailed || result.LifecycleState != managedLifecycleNotInstalled ||
		result.FailureStage != apisurface.PullStageSourceFetch {
		t.Fatalf("pull result = %#v, error = %v, want FAILED/SOURCE_FETCH_FAILED/FAILED/NOT_INSTALLED", result, err)
	}
}

func TestPullModelWithOptions_DoesNotSucceedAfterCallerCancellation(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	ctx, cancel := context.WithCancel(context.Background())
	result, err := PullModelWithOptions(
		&managedPullTestAssetPuller{result: apisurface.PullResult{
			ModelName: "OMNIVOICE_Q4_K_M", Outcome: legacyPullOutcomePulled,
		}},
		ctx, loaded, "OMNIVOICE_Q4_K_M",
		PullOptions{
			RuntimeCacheInspector: stubRuntimeCacheInspector{
				byModel: map[string]RuntimeCacheInspection{
					"OMNIVOICE_Q4_K_M": {Supported: true, Installed: true},
				},
				cancel: cancel,
			},
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PullModelWithOptions error = %v, want caller cancellation", err)
	}
	var pullErr *apisurface.PullError
	if !errors.As(err, &pullErr) || result.ReadinessState != managedReadinessFailed {
		t.Fatalf("pull result = %#v, error = %v, want classified cancellation failure", result, err)
	}
}

type managedPullTestAssetPuller struct {
	result apisurface.PullResult
	err    error
	calls  []string
}

func (p *managedPullTestAssetPuller) PullModel(_ context.Context, _ *modelRuntimeConfig, name string) (apisurface.PullResult, error) {
	p.calls = append(p.calls, name)
	return p.result, p.err
}

func (p *managedPullTestAssetPuller) EnsureModelAvailable(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) error {
	return nil
}

func (p *managedPullTestAssetPuller) ResolveModelCache(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) (CacheLayout, error) {
	return CacheLayout{}, nil
}

func (p *managedPullTestAssetPuller) InspectRuntimeCache(context.Context, *modelRuntimeConfig, string) (RuntimeCacheInspection, error) {
	return RuntimeCacheInspection{}, nil
}

func TestPullModelWithOptions_ModelScopeMirrorSourceDiagnostics(t *testing.T) {
	t.Parallel()
	cfg := catalogFactoryConfig(true)
	cfg.Resources[0].Provider = "MODELSCOPE"
	loaded := mustLoadedCatalogConfig(t, cfg)
	result, err := PullModelWithOptions(&managedPullTestAssetPuller{
		result: apisurface.PullResult{
			ModelName: "OMNIVOICE_Q4_K_M",
			Outcome:   legacyPullOutcomePulled,
		},
	}, context.Background(), loaded, "OMNIVOICE_Q4_K_M", PullOptions{
		RuntimeCacheInspector: stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
			"OMNIVOICE_Q4_K_M": {Supported: true, Installed: true},
		}},
		SourceResolver: DefaultManagedRuntimeSourceResolver(),
	})
	if err != nil {
		t.Fatalf("PullModelWithOptions: %v", err)
	}
	if result.SourceKind != ManagedRuntimeSourceKindManagedMirror {
		t.Fatalf("source kind = %q, want managed mirror", result.SourceKind)
	}
	if result.ManagedPullOutcome != managedPullOutcomeInstalledSuccessfully {
		t.Fatalf("pull outcome = %q, want INSTALLED_SUCCESSFULLY", result.ManagedPullOutcome)
	}
	if factoryapi.ManagedRuntimeReadinessState(result.ReadinessState) != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("readiness = %q, want READY", result.ReadinessState)
	}
}
