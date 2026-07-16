package models

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
	"github.com/portpowered/infinite-you/pkg/service"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	"go.uber.org/zap"
)

type offlineNonReadyAssetPuller struct {
	inspection localmodels.RuntimeCacheInspection
}

func (p offlineNonReadyAssetPuller) PullModel(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, _ string) (apisurface.ModelPullResult, error) {
	return apisurface.ModelPullResult{}, nil
}

func (p offlineNonReadyAssetPuller) EnsureModelAvailable(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, _ *workerconfig.Config) error {
	return nil
}

func (p offlineNonReadyAssetPuller) ResolveModelCache(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, _ *workerconfig.Config) (localmodels.CacheLayout, error) {
	return localmodels.CacheLayout{}, nil
}

func (p offlineNonReadyAssetPuller) InspectRuntimeCache(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, modelName string) (localmodels.RuntimeCacheInspection, error) {
	if localmodels.CanonicalModelName(modelName) != localmodels.CanonicalModelName("OMNIVOICE_Q4_K_M") {
		return localmodels.RuntimeCacheInspection{}, nil
	}
	return p.inspection, nil
}

func TestInvoke_OfflineNonReadyLifecycle_ReadinessGatedFailuresWithoutHTTPServer(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for offline non-ready managed-runtime bootstrap invoke")
	}

	tests := []struct {
		name              string
		useEmptyCacheOnly bool
		inspection        localmodels.RuntimeCacheInspection
		wantIs            error
		wantFailureClass  apisurface.InferenceFailureClass
		wantContains      []string
	}{
		{
			name:              "missing",
			useEmptyCacheOnly: true,
			wantIs:            apisurface.ErrManagedRuntimeMissing,
			wantFailureClass:  apisurface.InferenceFailureClassMissingModel,
			wantContains:      []string{"pull or install"},
		},
		{
			name: "loading",
			inspection: localmodels.RuntimeCacheInspection{
				Supported:          true,
				Installed:          false,
				InstalledFileCount: 1,
			},
			wantIs:           apisurface.ErrManagedRuntimeLoading,
			wantFailureClass: apisurface.InferenceFailureClassLoadingModel,
			wantContains:     []string{"still loading", "finish loading"},
		},
		{
			name: "failed",
			inspection: localmodels.RuntimeCacheInspection{
				Supported:        true,
				Installed:        false,
				PartialArtifacts: true,
			},
			wantIs:           apisurface.ErrManagedRuntimeFailed,
			wantFailureClass: apisurface.InferenceFailureClassRuntimeFailure,
			wantContains:     []string{"FAILED", "resolve the managed runtime failure"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preserveModelsBootstrapGlobals(t)
			factoryDir := t.TempDir()
			factoryfixtures.WriteFactoryJSON(t, factoryDir, nonReadyLocalModelFactoryConfig())

			augmentModelsInvokeBootstrapServiceConfig = func(cfg *service.FactoryServiceConfig) {
				cfg.ModelCacheDir = t.TempDir()
				cfg.MockWorkersConfig = factoryconfig.NewEmptyMockWorkersConfig()
				if !tt.useEmptyCacheOnly {
					cfg.ModelAssets = offlineNonReadyAssetPuller{inspection: tt.inspection}
				}
			}

			err := Invoke(InvokeConfig{
				BuildInvocation: testModelInvocationBuilder,
				ModelName:       "OMNIVOICE_Q4_K_M",
				Operation:       "TTS",
				Text:            "hello offline",
				FactoryDir:      factoryDir,
				Server:          failureBaselineUnreachableServer,
				JSON:            true,
				Output:          io.Discard,
				Logger:          zap.NewNop(),
			})
			if err == nil {
				t.Fatal("expected readiness-gated invoke failure")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Fatalf("error = %v, want errors.Is %v", err, tt.wantIs)
			}
			failure, ok := apisurface.AsInferenceFailure(err)
			if !ok || failure.Class != tt.wantFailureClass {
				t.Fatalf("error = %T class=%s, want %s InferenceFailure", err, failure.Class, tt.wantFailureClass)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %q, want substring %q", err.Error(), want)
				}
			}
			if strings.Contains(err.Error(), "models endpoint not reachable") {
				t.Fatalf("error = %q, want bootstrap readiness failure instead of transport failure", err.Error())
			}
			var readinessErr *apisurface.ManagedRuntimeInvocationError
			if !errors.As(err, &readinessErr) {
				t.Fatalf("error = %T, want *ManagedRuntimeInvocationError", err)
			}
			if readinessErr.ReadinessState == managedruntime.ReadinessStateReady {
				t.Fatalf("readiness = READY, want non-ready managed runtime state")
			}
		})
	}
}
