package service_test

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/service"
)

const (
	story002ModelName       = "gemma-4-E4B-it-Q4_K_M.gguf"
	story002ModelBytes      = int64(4977171584)
	story002ModelSHA256     = "85a896a047553e842f25297ee5b031d64ff30147d9c4af17b1e4b394cd1fab87"
	story002ProjectorName   = "mmproj-F16.gguf"
	story002ProjectorBytes  = int64(990372672)
	story002ProjectorSHA256 = "ddf46c21d7078e95338cfc22306b19b276a29a5ad089023449dd54d4b6170a51"
)

func TestManagedBuiltinLLMPropagatesVerifiedProjectorPath(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "story-002-propagation")
	ref := openScope(t, scopes, cacheDirectory, story002BuiltinLLMConfig())
	launcher := &controlledProcessLauncher{}
	protocol := &testProtocolNegotiator{}
	assets := &backendRuntimeInspectionAssets{
		Service: mustAssetsService(t, scopes),
		inspection: story002BuiltinLLMInspection(cacheDirectory, []models.AssetArtifact{
			{Name: story002ProjectorName, Bytes: story002ProjectorBytes, SHA256: story002ProjectorSHA256},
			{Name: story002ModelName, Bytes: story002ModelBytes, SHA256: story002ModelSHA256},
		}),
	}
	host := internalservice.NewWithHostTestConfig(
		scopes, assets, launcher, http.DefaultClient, realHostClock{}, nil, nil,
		internalservice.SupervisorTestConfig{}, internalservice.HostPolicyTestConfig{},
		runtimehost.Options{
			Platform:             managedHostPlatform(),
			CompatibilityChecker: &testCompatibilityChecker{},
			ProtocolNegotiator:   protocol,
		},
	)
	t.Cleanup(func() { _ = internalservice.ShutdownHost(context.Background(), host) })

	if _, err := host.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  models.BuiltInModelNameLLM,
	}); err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	wantModelPath := filepath.Join(cacheDirectory, story002ModelName)
	wantProjectorPath := filepath.Join(cacheDirectory, story002ProjectorName)
	spec := launcher.spec(0)
	if spec.ModelPath != wantModelPath || spec.MMProjPath != wantProjectorPath ||
		len(spec.ModelFiles) != 2 || spec.ModelFiles[0] != wantModelPath || spec.ModelFiles[1] != wantProjectorPath {
		t.Fatalf("host start paths = model %q projector %q files %#v, want %q/%q in canonical order", spec.ModelPath, spec.MMProjPath, spec.ModelFiles, wantModelPath, wantProjectorPath)
	}
	call := protocol.call()
	if call.request.ModelPath != wantModelPath || call.request.MMProjPath != wantProjectorPath ||
		len(call.request.ModelFiles) != 2 || call.request.ModelFiles[0] != wantModelPath || call.request.ModelFiles[1] != wantProjectorPath {
		t.Fatalf("protocol paths = model %q projector %q files %#v, want %q/%q in canonical order", call.request.ModelPath, call.request.MMProjPath, call.request.ModelFiles, wantModelPath, wantProjectorPath)
	}
}

func TestManagedBuiltinLLMRejectsInvalidProjectorBeforeHostEffects(t *testing.T) {
	t.Parallel()

	baseObserved := []models.AssetArtifact{
		{Name: story002ModelName, Bytes: story002ModelBytes, SHA256: story002ModelSHA256},
		{Name: story002ProjectorName, Bytes: story002ProjectorBytes, SHA256: story002ProjectorSHA256},
	}
	for _, testCase := range []struct {
		name   string
		mutate func([]models.AssetArtifact) []models.AssetArtifact
	}{
		{name: "missing projector", mutate: func(observed []models.AssetArtifact) []models.AssetArtifact {
			return observed[:1]
		}},
		{name: "corrupt projector metadata", mutate: func(observed []models.AssetArtifact) []models.AssetArtifact {
			observed[1].Bytes++
			return observed
		}},
		{name: "duplicate projector", mutate: func(observed []models.AssetArtifact) []models.AssetArtifact {
			return append(observed, observed[1])
		}},
		{name: "escaping projector", mutate: func(observed []models.AssetArtifact) []models.AssetArtifact {
			observed[1].Name = "../" + story002ProjectorName
			return observed
		}},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			cacheDirectory := t.TempDir()
			scopes := newScopes(t, "story-002-invalid-"+testCase.name)
			ref := openScope(t, scopes, cacheDirectory, story002BuiltinLLMConfig())
			launcher := &controlledProcessLauncher{}
			protocol := &testProtocolNegotiator{}
			observed := append([]models.AssetArtifact(nil), baseObserved...)
			assets := &backendRuntimeInspectionAssets{
				Service:    mustAssetsService(t, scopes),
				inspection: story002BuiltinLLMInspection(cacheDirectory, testCase.mutate(observed)),
			}
			host := internalservice.NewWithHostTestConfig(
				scopes, assets, launcher, http.DefaultClient, realHostClock{}, nil, nil,
				internalservice.SupervisorTestConfig{}, internalservice.HostPolicyTestConfig{},
				runtimehost.Options{
					Platform:             managedHostPlatform(),
					CompatibilityChecker: &testCompatibilityChecker{},
					ProtocolNegotiator:   protocol,
				},
			)
			t.Cleanup(func() { _ = internalservice.ShutdownHost(context.Background(), host) })

			_, err := host.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
				Scope: ref,
				Name:  models.BuiltInModelNameLLM,
			})
			if !errors.Is(err, models.ErrHostMissingAssets) {
				t.Fatalf("EnsureModelHost error = %v, want ErrHostMissingAssets", err)
			}
			if launcher.startCount() != 0 || protocol.call().endpoint != "" {
				t.Fatalf("host effects = launcher starts %d, protocol call %#v, want no launcher or negotiator effect", launcher.startCount(), protocol.call())
			}
		})
	}
}

func story002BuiltinLLMConfig() models.RuntimeConfig {
	config := managedLocalAIConfig(models.LoadPolicyOnDemand)
	config.Resources[0].Name = "llm-cache"
	config.Resources[0].Model = models.BuiltInModelNameLLM
	config.Workers[0].Name = "llm-worker"
	config.Workers[0].Model = models.BuiltInModelNameLLM
	return config
}

func story002BuiltinLLMInspection(
	cacheDirectory string,
	observed []models.AssetArtifact,
) scopedassets.RuntimeCacheInspection {
	return scopedassets.RuntimeCacheInspection{
		Supported: true, Installed: true, CachePath: cacheDirectory,
		ManifestPresent: true, ManifestValid: true, IntegrityVerified: true,
		ExpectedArtifacts: []models.AssetRequirement{
			{Name: story002ModelName, Bytes: story002ModelBytes, SHA256: story002ModelSHA256},
			{Name: story002ProjectorName, Bytes: story002ProjectorBytes, SHA256: story002ProjectorSHA256},
		},
		ObservedArtifacts: observed,
	}
}

var _ modelseffects.HostProtocolNegotiator = (*testProtocolNegotiator)(nil)
