package service_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
	compatibility := &testCompatibilityChecker{}
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
			CompatibilityChecker: compatibility,
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
	if spec.Configuration.ModelPath != wantModelPath || spec.Configuration.MMProjPath != wantProjectorPath ||
		len(spec.Configuration.ModelFiles) != 2 || spec.Configuration.ModelFiles[0] != wantModelPath || spec.Configuration.ModelFiles[1] != wantProjectorPath {
		t.Fatalf("host start paths = model %q projector %q files %#v, want %q/%q in canonical order", spec.Configuration.ModelPath, spec.Configuration.MMProjPath, spec.Configuration.ModelFiles, wantModelPath, wantProjectorPath)
	}
	call := protocol.call()
	if call.request.Configuration.ModelPath != wantModelPath || call.request.Configuration.MMProjPath != wantProjectorPath ||
		len(call.request.Configuration.ModelFiles) != 2 || call.request.Configuration.ModelFiles[0] != wantModelPath || call.request.Configuration.ModelFiles[1] != wantProjectorPath {
		t.Fatalf("protocol paths = model %q projector %q files %#v, want %q/%q in canonical order", call.request.Configuration.ModelPath, call.request.Configuration.MMProjPath, call.request.Configuration.ModelFiles, wantModelPath, wantProjectorPath)
	}
}

func TestResolvedHostConfigurationHandoffReachesHostAndProtocolExactly(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "story-002-private-handoff")
	ref := openScope(t, scopes, cacheDirectory, story002BuiltinLLMConfig())
	launcher := &controlledProcessLauncher{}
	protocol := &testProtocolNegotiator{}
	compatibility := &testCompatibilityChecker{}
	assets := &backendRuntimeInspectionAssets{
		Service: mustAssetsService(t, scopes),
		inspection: story002BuiltinLLMInspection(cacheDirectory, []models.AssetArtifact{
			{Name: story002ModelName, Bytes: story002ModelBytes, SHA256: story002ModelSHA256},
			{Name: story002ProjectorName, Bytes: story002ProjectorBytes, SHA256: story002ProjectorSHA256},
		}),
	}
	host := internalservice.NewWithHostTestConfig(
		scopes, assets, launcher, http.DefaultClient, realHostClock{}, nil, nil,
		internalservice.SupervisorTestConfig{}, internalservice.HostPolicyTestConfig{},
		runtimehost.Options{
			Platform:             managedHostPlatform(),
			CompatibilityChecker: compatibility,
			ProtocolNegotiator:   protocol,
		},
	)
	t.Cleanup(func() { _ = internalservice.ShutdownHost(context.Background(), host) })

	configuration := modelseffects.ResolvedHostConfiguration{
		Scope:            ref,
		ModelName:        models.BuiltInModelNameLLM,
		Source:           models.ModelReference{NameOrURI: "hf://operator/exact-model.gguf@revision-2"},
		Revision:         "revision-2",
		Backend:          "localai-llamacpp",
		Platform:         managedHostPlatform(),
		ProtocolVersion:  modelseffects.PinnedHostProtocolVersion,
		ModelCachePath:   cacheDirectory,
		BackendCachePath: filepath.Join(cacheDirectory, "backend"),
		ModelPath:        filepath.Join(cacheDirectory, story002ModelName),
		MMProjPath:       filepath.Join(cacheDirectory, story002ProjectorName),
		ModelFiles: []string{
			filepath.Join(cacheDirectory, story002ModelName),
			filepath.Join(cacheDirectory, story002ProjectorName),
		},
		BackendFiles: []string{filepath.Join(cacheDirectory, "backend", "backend.zip")},
		BackendArtifact: modelseffects.BackendArtifactSelection{
			Name: "backend.zip", Location: "https://example.invalid/backend.zip", Bytes: 17,
			SHA256: strings.Repeat("a", 64),
		},
	}
	handoff, ok := host.(interface {
		EnsureModelHostWithConfiguration(
			context.Context,
			modelseffects.ResolvedHostConfiguration,
		) (models.EnsureModelHostResult, error)
	})
	if !ok {
		t.Fatal("Runtime Host does not expose the parent-private configuration handoff")
	}
	if _, err := handoff.EnsureModelHostWithConfiguration(context.Background(), configuration); err != nil {
		t.Fatalf("EnsureModelHostWithConfiguration: %v", err)
	}
	if got := launcher.spec(0).Configuration; !reflect.DeepEqual(got, configuration) {
		t.Fatalf("process configuration = %#v, want exact %#v", got, configuration)
	}
	if got := protocol.call().request.Configuration; !reflect.DeepEqual(got, configuration) {
		t.Fatalf("protocol configuration = %#v, want exact %#v", got, configuration)
	}
	compatibility.mu.Lock()
	defer compatibility.mu.Unlock()
	if len(compatibility.requests) != 1 || !reflect.DeepEqual(compatibility.requests[0].Configuration, configuration) {
		t.Fatalf("compatibility configuration = %#v, want exact %#v", compatibility.requests, configuration)
	}
}

func TestResolvedHostConfigurationHandoffRejectsEscapingModelSymlinkBeforeEffects(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	outsideDirectory := t.TempDir()
	outsideModel := filepath.Join(outsideDirectory, story002ModelName)
	if err := os.WriteFile(outsideModel, []byte("outside model"), 0o600); err != nil {
		t.Fatalf("write outside model: %v", err)
	}
	escapingModel := filepath.Join(cacheDirectory, story002ModelName)
	if err := os.Symlink(outsideModel, escapingModel); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	projector := filepath.Join(cacheDirectory, story002ProjectorName)
	if err := os.WriteFile(projector, []byte("projector"), 0o600); err != nil {
		t.Fatalf("write projector: %v", err)
	}

	scopes := newScopes(t, "story-002-handoff-symlink-escape")
	ref := openScope(t, scopes, cacheDirectory, story002BuiltinLLMConfig())
	launcher := &controlledProcessLauncher{}
	compatibility := &testCompatibilityChecker{}
	protocol := &testProtocolNegotiator{}
	assets := &backendRuntimeInspectionAssets{
		Service: mustAssetsService(t, scopes),
		inspection: story002BuiltinLLMInspection(cacheDirectory, []models.AssetArtifact{
			{Name: story002ModelName, Bytes: story002ModelBytes, SHA256: story002ModelSHA256},
			{Name: story002ProjectorName, Bytes: story002ProjectorBytes, SHA256: story002ProjectorSHA256},
		}),
	}
	host := internalservice.NewWithHostTestConfig(
		scopes, assets, launcher, http.DefaultClient, realHostClock{}, nil, nil,
		internalservice.SupervisorTestConfig{}, internalservice.HostPolicyTestConfig{},
		runtimehost.Options{
			Platform:             managedHostPlatform(),
			CompatibilityChecker: compatibility,
			ProtocolNegotiator:   protocol,
			ResolveSymlinks:      filepath.EvalSymlinks,
		},
	)
	t.Cleanup(func() { _ = internalservice.ShutdownHost(context.Background(), host) })

	configuration := modelseffects.ResolvedHostConfiguration{
		Scope:           ref,
		ModelName:       models.BuiltInModelNameLLM,
		Source:          models.ModelReference{NameOrURI: "hf://operator/symlink-model.gguf@revision-escape"},
		Revision:        "revision-escape",
		Backend:         "localai-llamacpp",
		Platform:        managedHostPlatform(),
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		ModelCachePath:  cacheDirectory,
		ModelPath:       escapingModel,
		MMProjPath:      projector,
		ModelFiles:      []string{escapingModel, projector},
	}
	handoff, ok := host.(interface {
		EnsureModelHostWithConfiguration(
			context.Context,
			modelseffects.ResolvedHostConfiguration,
		) (models.EnsureModelHostResult, error)
	})
	if !ok {
		t.Fatal("Runtime Host does not expose the parent-private configuration handoff")
	}
	_, err := handoff.EnsureModelHostWithConfiguration(context.Background(), configuration)
	if !errors.Is(err, models.ErrHostMissingAssets) {
		t.Fatalf("escaping handoff error = %v, want ErrHostMissingAssets", err)
	}
	if launcher.startCount() != 0 {
		t.Fatalf("launcher starts = %d, want no process effect", launcher.startCount())
	}
	if call := protocol.call(); call.endpoint != "" {
		t.Fatalf("protocol call = %#v, want no protocol effect", call)
	}
	compatibility.mu.Lock()
	compatibilityCalls := compatibility.calls
	compatibility.mu.Unlock()
	if compatibilityCalls != 0 {
		t.Fatalf("compatibility calls = %d, want no compatibility effect", compatibilityCalls)
	}
	lease, leaseErr := (models.ModelLeaseRef{}).Parse("model-lease-1")
	if leaseErr != nil {
		t.Fatalf("parse lease probe: %v", leaseErr)
	}
	if _, leaseErr = internalservice.LeasesService(host).GetModelLease(
		context.Background(), models.GetModelLeaseRequest{Scope: ref, Lease: lease},
	); !errors.Is(leaseErr, models.ErrHostLeaseNotFound) {
		t.Fatalf("lease probe error = %v, want no lease side effect", leaseErr)
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

func TestOperatorLLMSourceOverrideUsesGenericVerifiedModelPath(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "story-002-source-override")
	config := story002BuiltinLLMConfig()
	config.Workers[0].Command = ""
	customSource := "hf://operator/custom-model/custom-model.gguf@" + strings.Repeat("a", 40)
	ref := openScopeWithOverlays(t, scopes, cacheDirectory, config, map[string]models.ModelOverlay{
		models.BuiltInModelNameLLM: {Source: &customSource},
	})
	launcher := &controlledProcessLauncher{}
	protocol := &testProtocolNegotiator{}
	const customModelName = "custom-model.gguf"
	const customModelBytes = int64(17)
	const customModelSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	assets := &backendRuntimeInspectionAssets{
		Service: mustAssetsService(t, scopes),
		inspection: scopedassets.RuntimeCacheInspection{
			Supported: true, Installed: true, CachePath: cacheDirectory,
			ManifestPresent: true, ManifestValid: true, IntegrityVerified: true,
			BackendRequired: true, BackendFiles: []string{"backend.zip"},
			ExpectedArtifacts: []models.AssetRequirement{{
				Name: customModelName, Bytes: customModelBytes, SHA256: customModelSHA256,
			}},
			ObservedArtifacts: []models.AssetArtifact{{
				Name: customModelName, Bytes: customModelBytes, SHA256: customModelSHA256,
			}},
		},
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
	wantModelPath := filepath.Join(cacheDirectory, customModelName)
	spec := launcher.spec(0)
	if spec.Configuration.ModelPath != wantModelPath || spec.Configuration.MMProjPath != "" ||
		len(spec.Configuration.ModelFiles) != 1 || spec.Configuration.ModelFiles[0] != wantModelPath {
		t.Fatalf("override host paths = model %q projector %q files %#v, want generic model path %q without projector", spec.Configuration.ModelPath, spec.Configuration.MMProjPath, spec.Configuration.ModelFiles, wantModelPath)
	}
	call := protocol.call()
	if call.request.Configuration.ModelPath != wantModelPath || call.request.Configuration.MMProjPath != "" ||
		len(call.request.Configuration.ModelFiles) != 1 || call.request.Configuration.ModelFiles[0] != wantModelPath {
		t.Fatalf("override protocol paths = model %q projector %q files %#v, want generic model path %q without projector", call.request.Configuration.ModelPath, call.request.Configuration.MMProjPath, call.request.Configuration.ModelFiles, wantModelPath)
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
