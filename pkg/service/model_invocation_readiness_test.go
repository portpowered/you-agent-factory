package service

import (
	"context"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/localmodels"
)

func TestRuntimeModelService_PullThenInvoke_UsesManagedRuntimeReadiness(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: interfaces.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}
	cache := localModelTestCacheLayout(t)
	puller := &managedPullMetricsAssetPuller{
		result: apisurface.ModelPullResult{
			ModelName:        "OMNIVOICE_Q4_K_M",
			ProviderLocality: interfaces.ModelLocalityLocal,
			Outcome:          "PULLED",
			CachePath:        cache.CachePath,
			Revision:         cache.Revision,
		},
		inspection: localmodels.RuntimeCacheInspection{
			Supported:          true,
			Installed:          true,
			CachePath:          cache.CachePath,
			Revision:           cache.Revision,
			InstalledFileCount: len(cache.Files),
		},
		cache: cache,
	}

	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", localModelFactoryConfig(), localModelRuntimeWorkers(), nil)
	svc := &FactoryService{
		policy:      serviceCoordinatorPolicyFromConfig(&FactoryServiceConfig{}),
		modelAssets: puller,
	}
	bindServiceStartupRuntime(svc, &factoryRuntimeBundle{
		runtimeCfg:  runtimeCfg,
		modelAssets: puller,
		localModels: newManagedLocalModelManager(puller, runtime),
	})

	pullResult, err := svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	if pullResult.ReadinessState != "READY" {
		t.Fatalf("pull readiness = %q, want READY", pullResult.ReadinessState)
	}

	mode := factoryapi.AUDIOSTREAM
	result, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content: &factoryapi.WorkContent{
			mustGeneratedServiceTextPart(t, "hello after pull"),
		},
		Options: &factoryapi.ModelInvocationOptions{ResponseMode: &mode},
	})
	if err != nil {
		t.Fatalf("InvokeModel after pull: %v", err)
	}
	if result.ModelName != "OMNIVOICE_Q4_K_M" || result.StreamFile != audioPath {
		t.Fatalf("result = %#v, want OMNIVOICE audio stream at %s", result, audioPath)
	}
	if runtime.invocationCount() != 1 {
		t.Fatalf("invocation count = %d, want 1", runtime.invocationCount())
	}
}
