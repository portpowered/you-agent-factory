package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

import models "github.com/portpowered/infinite-you/pkg/services/models"
import modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
import modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/legacyhost"
import localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
import scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"

func TestBuiltInASRRequiresRecognitionAndPinnedAlignmentAssets(t *testing.T) {
	definition, _ := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameASR)
	requirements, err := joinedModelAssetRequirements(definition, definition.Source)
	if err != nil || len(requirements) != 2 {
		t.Fatalf("ASR bundle=%#v error=%v", requirements, err)
	}
	if requirements[0].Name != "qwen3-asr-0.6b-q8_0.gguf" || requirements[0].Bytes != 1354082624 ||
		requirements[1].Name != "qwen3-forced-aligner-0.6b-q8_0.gguf" || requirements[1].Bytes != 994404608 {
		t.Fatalf("ASR must require both complete immutable assets: %#v", requirements)
	}
	for _, requirement := range requirements {
		if len(requirement.SHA256) != 64 {
			t.Fatalf("ASR artifact missing hash: %#v", requirement)
		}
	}
}

type modelRuntimeConfig = models.RuntimeConfig
type modelRuntimeWorker = models.RuntimeWorker
type modelRuntimeResource = models.RuntimeResource

type testFactoryConfig struct {
	Name             string
	Workers          []modelRuntimeWorker
	Resources        []modelRuntimeResource
	ResourceManifest *testResourceManifest
}
type testResourceManifest struct{ RequiredTools []testRequiredTool }
type testRequiredTool struct{ Name, Command string }

func projectTestModelsRuntimeConfig(factoryDir string, cfg *testFactoryConfig) *modelRuntimeConfig {
	if cfg == nil {
		return nil
	}
	result := &modelRuntimeConfig{FactoryDirectory: factoryDir}
	result.Resources = projectTestModelsResources(cfg.Resources)
	result.Workers = make([]modelRuntimeWorker, len(cfg.Workers))
	for i, worker := range cfg.Workers {
		result.Workers[i] = modelRuntimeWorker{
			Name: worker.Name, Type: worker.Type, Model: worker.Model, ModelProvider: worker.ModelProvider,
			ModelLocality: worker.ModelLocality, Command: worker.Command, Args: append([]string(nil), worker.Args...),
			Resources:  projectTestModelsResources(worker.Resources),
			Operations: projectTestModelsOperations(worker.Operations),
		}
	}
	return result
}

func TestRootLegacyOperationsClassifyMissingRuntimeBinding(t *testing.T) {
	t.Parallel()

	root := &Root{}
	if _, err := root.ListModels(context.Background()); !errors.Is(err, ErrInvalidDependencies) {
		t.Fatalf("ListModels error = %v, want missing runtime binding", err)
	}
	if _, err := root.GetModel(context.Background(), "voice"); !errors.Is(err, ErrInvalidDependencies) {
		t.Fatalf("GetModel error = %v, want missing runtime binding", err)
	}
	if _, err := root.PullModel(context.Background(), "voice"); !errors.Is(err, ErrInvalidDependencies) {
		t.Fatalf("PullModel error = %v, want missing runtime binding", err)
	}
	if _, err := root.InspectRuntime(context.Background(), "voice"); !errors.Is(err, ErrInvalidDependencies) {
		t.Fatalf("InspectRuntime error = %v, want missing runtime binding", err)
	}
	if _, err := root.AcquireLease(context.Background(), models.AcquireLeaseRequest{ModelName: "voice"}); !errors.Is(err, ErrInvalidDependencies) {
		t.Fatalf("AcquireLease error = %v, want missing runtime binding", err)
	}
	if err := root.ReleaseLease(context.Background(), models.ReleaseLeaseRequest{LeaseID: "lease"}); !errors.Is(err, ErrInvalidDependencies) {
		t.Fatalf("ReleaseLease error = %v, want missing runtime binding", err)
	}
}

func projectTestModelsResources(resources []modelRuntimeResource) []modelRuntimeResource {
	result := make([]modelRuntimeResource, len(resources))
	for i, resource := range resources {
		result[i] = modelRuntimeResource{
			ID: resource.ID, Name: resource.Name, Type: resource.Type, Capacity: resource.Capacity,
			Model: resource.Model, Backend: resource.Backend, LoadPolicy: resource.LoadPolicy, Provider: resource.Provider,
		}
	}
	return result
}

func projectTestModelsOperations(operations []models.RuntimeOperation) []models.RuntimeOperation {
	result := make([]models.RuntimeOperation, len(operations))
	for i, operation := range operations {
		result[i].Name = operation.Name
		result[i].Inputs = projectTestModelsOperationSlots(operation.Inputs)
		result[i].Outputs = projectTestModelsOperationSlots(operation.Outputs)
	}
	return result
}

func projectTestModelsOperationSlots(slots []models.RuntimeOperationSlot) []models.RuntimeOperationSlot {
	result := make([]models.RuntimeOperationSlot, len(slots))
	for i, slot := range slots {
		result[i] = models.RuntimeOperationSlot{
			Name: slot.Name, ContentTypes: append([]string(nil), slot.ContentTypes...), Required: slot.Required,
		}
	}
	return result
}

func TestJoinedHostConfigurationRetainsResolvedSourceAndPlatformFacts(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("configuration-test-scope")
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	platform := models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"}
	tests := []struct {
		name     string
		request  models.InvokeModelRequest
		resolved models.ResolvedModelReference
		want     modelseffects.ResolvedHostConfiguration
	}{
		{
			name: "built in source",
			request: models.InvokeModelRequest{
				Scope: scope, Model: models.ModelReference{NameOrURI: "llm"},
			},
			resolved: models.ResolvedModelReference{Definition: models.ModelDefinition{
				Name: "llm", Source: "hf://owner/repository/model.gguf@revision-1", Backend: "localai-llamacpp",
			}},
			want: modelseffects.ResolvedHostConfiguration{
				Scope: scope, ModelName: "llm",
				Source:   models.ModelReference{NameOrURI: "hf://owner/repository/model.gguf@revision-1"},
				Revision: "revision-1", Backend: "localai-llamacpp", Platform: platform,
				ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
			},
		},
		{
			name: "operator local source keeps private request reference",
			request: models.InvokeModelRequest{
				Scope: scope, Model: models.ModelReference{NameOrURI: "operator-llm"},
			},
			resolved: models.ResolvedModelReference{
				Definition: models.ModelDefinition{
					Name: "operator-llm", Source: "file://operator-bundle", Backend: "localai-llamacpp",
				},
				Provenance: models.ModelReferenceProvenance{
					SourceKind: models.ModelReferenceSourceFileURI,
				},
			},
			want: modelseffects.ResolvedHostConfiguration{
				Scope: scope, ModelName: "operator-llm",
				Source:  models.ModelReference{NameOrURI: "operator-llm"},
				Backend: "localai-llamacpp", Platform: platform,
				ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := joinedHostConfiguration(test.request, test.resolved, platform)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("joined host configuration = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestResolvedHostConfigurationCloneDetachesPrivateFiles(t *testing.T) {
	t.Parallel()

	configuration := modelseffects.ResolvedHostConfiguration{
		Source:       models.ModelReference{NameOrURI: "hf://owner/repository/model.gguf@revision-1"},
		ModelFiles:   []string{"model.gguf", "mmproj-F16.gguf"},
		BackendFiles: []string{"backend.tar.gz"},
	}
	clone := configuration.Clone()
	clone.Source.NameOrURI = "mutated-source"
	clone.ModelFiles[0] = "mutated-model"
	clone.BackendFiles[0] = "mutated-backend"

	if configuration.Source.NameOrURI != "hf://owner/repository/model.gguf@revision-1" ||
		configuration.ModelFiles[0] != "model.gguf" || configuration.BackendFiles[0] != "backend.tar.gz" {
		t.Fatalf("configuration changed through clone mutation: %#v", configuration)
	}
}

func TestResolvedHostConfigurationEnrichesVerifiedCacheFacts(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("enrichment-test-scope")
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	selection := modelseffects.BackendArtifactSelection{
		Name: "localai-backend.tar.gz", Location: "https://example.invalid/backend.tar.gz",
		Bytes: 42, SHA256: "10a84e67d02d078f711608accf13cb80b6724a4c03dc4acae5ba936831801172",
	}
	layout := scopedassets.RuntimeCacheLayout{
		CachePath:        "cache/models/revision-1",
		Revision:         "revision-1",
		Files:            []string{"cache/models/revision-1/tokenizer.gguf", "cache/models/revision-1/mmproj-F16.gguf", "cache/models/revision-1/vibevoice-model.gguf"},
		BackendCachePath: "cache/backends/revision-1",
		BackendFiles:     []string{"cache/backends/revision-1/localai-backend.tar.gz"},
	}
	configuration := modelseffects.ResolvedHostConfiguration{
		Scope: scope, ModelName: "tts", Source: models.ModelReference{NameOrURI: "tts"},
		Backend: "localai-vibevoice", Platform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion, BackendArtifact: selection,
	}
	assets := &resolvedHostLayoutAssets{layout: layout}
	root := &Root{assets: assets}

	if err := root.enrichJoinedHostConfiguration(context.Background(), &configuration); err != nil {
		t.Fatalf("enrichJoinedHostConfiguration: %v", err)
	}
	if configuration.ModelCachePath != layout.CachePath || configuration.BackendCachePath != layout.BackendCachePath ||
		configuration.Revision != layout.Revision || configuration.ModelPath != layout.Files[2] ||
		configuration.MMProjPath != layout.Files[1] || len(configuration.ModelFiles) != len(layout.Files) ||
		len(configuration.BackendFiles) != len(layout.BackendFiles) ||
		!reflect.DeepEqual(configuration.BackendArtifact, selection) {
		t.Fatalf("enriched configuration = %#v, want selected cache and artifact facts", configuration)
	}

	layout.Files[0] = "mutated-after-enrichment"
	layout.BackendFiles[0] = "mutated-backend-after-enrichment"
	if configuration.ModelFiles[0] == layout.Files[0] || configuration.BackendFiles[0] == layout.BackendFiles[0] {
		t.Fatal("enriched file slices alias the asset layout")
	}
}

func TestResolvedHostConfigurationRejectsIncompleteSelectedCache(t *testing.T) {
	t.Parallel()

	validModelFiles := []string{"cache/model.gguf"}
	validBackendFiles := []string{"cache/backend.tar.gz"}
	selection := modelseffects.BackendArtifactSelection{
		Name: "backend.tar.gz", Location: "https://example.invalid/backend.tar.gz",
		Bytes: 1, SHA256: "10a84e67d02d078f711608accf13cb80b6724a4c03dc4acae5ba936831801172",
	}
	tests := []struct {
		name   string
		layout scopedassets.RuntimeCacheLayout
	}{
		{name: "missing model cache", layout: scopedassets.RuntimeCacheLayout{Files: validModelFiles, BackendCachePath: "cache/backend", BackendFiles: validBackendFiles}},
		{name: "missing model files", layout: scopedassets.RuntimeCacheLayout{CachePath: "cache/model", BackendCachePath: "cache/backend", BackendFiles: validBackendFiles}},
		{name: "missing backend cache", layout: scopedassets.RuntimeCacheLayout{CachePath: "cache/model", Files: validModelFiles}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configuration := modelseffects.ResolvedHostConfiguration{
				ModelName: "llm", Backend: "localai-llamacpp", BackendArtifact: selection,
			}
			root := &Root{assets: &resolvedHostLayoutAssets{layout: test.layout}}
			err := root.enrichJoinedHostConfiguration(context.Background(), &configuration)
			if !errors.Is(err, models.ErrHostMissingAssets) {
				t.Fatalf("enrichment error = %v, want ErrHostMissingAssets", err)
			}
		})
	}
}

func TestResolveJoinedBackendArtifactReceivesOneDetachedConfiguration(t *testing.T) {
	t.Parallel()

	configuration := modelseffects.ResolvedHostConfiguration{
		Scope: scopeForResolvedHostTest(t), ModelName: "llm",
		Source:   models.ModelReference{NameOrURI: "hf://owner/repository/model.gguf@revision-1"},
		Revision: "revision-1", Backend: "localai-llamacpp",
		Platform:        models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		ModelCachePath:  "cache/model", ModelFiles: []string{"cache/model/model.gguf"},
	}
	var observed modelseffects.ResolvedHostConfiguration
	root := &Root{
		resolveBackendArtifact: func(_ context.Context, request modelseffects.ResolvedHostConfiguration, _ bool) (modelseffects.BackendArtifactSelection, error) {
			observed = request.Clone()
			request.ModelFiles[0] = "mutated-by-selector"
			request.Source.NameOrURI = "mutated-by-selector"
			return modelseffects.BackendArtifactSelection{
				Name: "backend.tar.gz", Location: "https://example.invalid/backend.tar.gz",
				Bytes: 1, SHA256: "10a84e67d02d078f711608accf13cb80b6724a4c03dc4acae5ba936831801172",
			}, nil
		},
	}
	selection, err := root.resolveJoinedBackendArtifact(context.Background(), configuration, false)
	if err != nil {
		t.Fatalf("resolveJoinedBackendArtifact: %v", err)
	}
	if observed.Scope != configuration.Scope || observed.Source.NameOrURI != configuration.Source.NameOrURI ||
		observed.Backend != configuration.Backend || observed.Platform != configuration.Platform ||
		observed.ProtocolVersion != configuration.ProtocolVersion || observed.ModelFiles[0] != configuration.ModelFiles[0] {
		t.Fatalf("selector observed configuration = %#v, want exact detached facts", observed)
	}
	if configuration.ModelFiles[0] != "cache/model/model.gguf" || configuration.Source.NameOrURI != "hf://owner/repository/model.gguf@revision-1" {
		t.Fatalf("selector mutated caller configuration: %#v", configuration)
	}
	if selection.Name == "" || selection.Location == "" || selection.Bytes <= 0 || selection.SHA256 == "" {
		t.Fatalf("selection = %#v, want detached artifact identity", selection)
	}
}

func TestRootInvokeUsesConfigurationForArtifactAndOfflineAssetProjection(t *testing.T) {
	t.Parallel()

	var events []string
	var assetRequests []models.PrepareModelAssetsRequest
	inference := &joinedInferenceService{
		events: &events,
		result: joinedCompletedResult(t),
	}
	inference.result.LeaseDisposition = models.InvocationLeaseRetained
	root, scope, host := newJoinedInvocationRootWithModel(
		t, &events, inference,
		"hf://operator/repository/model.gguf@0123456789abcdef0123456789abcdef01234567",
		"localai-llamacpp",
		&assetRequests,
	)
	platform := models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"}
	var observed modelseffects.ResolvedHostConfiguration
	var observedOffline bool
	root.backendArtifactPlatform = platform
	root.resolveBackendArtifact = func(_ context.Context, configuration modelseffects.ResolvedHostConfiguration, offline bool) (modelseffects.BackendArtifactSelection, error) {
		observed = configuration.Clone()
		observedOffline = offline
		return modelseffects.BackendArtifactSelection{
			Name: "backend.tar.gz", Location: "https://example.invalid/backend.tar.gz",
			Bytes: 1, SHA256: "10a84e67d02d078f711608accf13cb80b6724a4c03dc4acae5ba936831801172",
		}, nil
	}
	request := joinedInvocationRequest(scope)
	request.Offline = true
	if _, err := root.InvokeModel(context.Background(), request); err != nil {
		t.Fatalf("InvokeModel: %v", err)
	}
	if observed.Scope != scope || observed.ModelName != "joined-model" ||
		observed.Source.NameOrURI != "hf://operator/repository/model.gguf@0123456789abcdef0123456789abcdef01234567" ||
		observed.Backend != "localai-llamacpp" || observed.Platform != platform ||
		observed.ProtocolVersion != modelseffects.PinnedHostProtocolVersion {
		t.Fatalf("artifact resolver configuration = %#v, want resolved source/backend/platform/protocol", observed)
	}
	if !observedOffline {
		t.Fatal("artifact resolver did not receive offline policy")
	}
	if len(assetRequests) != 1 || !assetRequests[0].Offline ||
		assetRequests[0].BackendReference.NameOrURI != "https://example.invalid/backend.tar.gz" ||
		len(assetRequests[0].BackendArtifacts) != 1 {
		t.Fatalf("asset preparation requests = %#v, want selected artifact and offline policy", assetRequests)
	}
	if len(events) == 0 || host.releaseCalls != 1 {
		t.Fatalf("invocation lifecycle events = %#v, release calls = %d, want host invocation and one release", events, host.releaseCalls)
	}
}

func TestResolveJoinedBackendArtifactRejectsInvalidFactsBeforeHostStart(t *testing.T) {
	t.Parallel()

	base := modelseffects.ResolvedHostConfiguration{
		Backend: "localai-llamacpp", Platform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
	}
	tests := []struct {
		name      string
		selection modelseffects.BackendArtifactSelection
	}{
		{name: "missing name", selection: modelseffects.BackendArtifactSelection{Location: "https://example.invalid/backend.tar.gz", Bytes: 1, SHA256: "10a84e67d02d078f711608accf13cb80b6724a4c03dc4acae5ba936831801172"}},
		{name: "missing location", selection: modelseffects.BackendArtifactSelection{Name: "backend.tar.gz", Bytes: 1, SHA256: "10a84e67d02d078f711608accf13cb80b6724a4c03dc4acae5ba936831801172"}},
		{name: "missing digest", selection: modelseffects.BackendArtifactSelection{Name: "backend.tar.gz", Location: "https://example.invalid/backend.tar.gz", Bytes: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := &Root{resolveBackendArtifact: func(context.Context, modelseffects.ResolvedHostConfiguration, bool) (modelseffects.BackendArtifactSelection, error) {
				return test.selection, nil
			}}
			_, err := root.resolveJoinedBackendArtifact(context.Background(), base, false)
			if !errors.Is(err, models.ErrHostMissingAssets) {
				t.Fatalf("resolve error = %v, want ErrHostMissingAssets before host start", err)
			}
		})
	}
}

func TestResolvedHostConfigurationDoesNotCarryOfflinePolicy(t *testing.T) {
	t.Parallel()

	if _, ok := reflect.TypeOf(modelseffects.ResolvedHostConfiguration{}).FieldByName("Offline"); ok {
		t.Fatal("ResolvedHostConfiguration carries offline policy")
	}
	request, err := prepareJoinedAssetFixture(
		models.InvokeModelRequest{Model: models.ModelReference{NameOrURI: "llm"}, Offline: true},
		"llm",
		models.ResolvedModelReference{Definition: models.ModelDefinition{
			Name: "llm", Source: "hf://owner/repository/model.gguf@revision-1", Backend: "localai-llamacpp",
		}},
	)
	if err != nil {
		t.Fatalf("joinedAssetPreparationRequest: %v", err)
	}
	if !request.Offline {
		t.Fatal("offline policy was not retained on PrepareModelAssetsRequest")
	}
}

func scopeForResolvedHostTest(t *testing.T) models.RuntimeScopeRef {
	t.Helper()
	scope, err := (models.RuntimeScopeRef{}).Parse("resolver-test-scope")
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	return scope
}

type resolvedHostLayoutAssets struct {
	layout scopedassets.RuntimeCacheLayout
}

func (assets *resolvedHostLayoutAssets) PreflightModelAssets(context.Context, models.PrepareModelAssetsRequest) (models.PreflightModelAssetsResult, error) {
	return models.PreflightModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (assets *resolvedHostLayoutAssets) PrepareModelAssets(context.Context, models.PrepareModelAssetsRequest) (models.PrepareModelAssetsResult, error) {
	return models.PrepareModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (assets *resolvedHostLayoutAssets) InspectModelAssets(context.Context, models.InspectModelAssetsRequest) (models.InspectModelAssetsResult, error) {
	return models.InspectModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (assets *resolvedHostLayoutAssets) RemoveModelAssets(context.Context, models.RemoveModelAssetsRequest) (models.RemoveModelAssetsResult, error) {
	return models.RemoveModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (assets *resolvedHostLayoutAssets) ResolveRuntimeCache(context.Context, models.InspectModelAssetsRequest) (scopedassets.RuntimeCacheLayout, error) {
	return assets.layout, nil
}

func (assets *resolvedHostLayoutAssets) InspectRuntimeCache(context.Context, models.InspectModelAssetsRequest) (scopedassets.RuntimeCacheInspection, error) {
	return scopedassets.RuntimeCacheInspection{}, models.ErrUnsupportedOperation
}

var _ scopedassets.Service = (*resolvedHostLayoutAssets)(nil)

func TestResolveJoinedBackendArtifactAcceptsInstalledDirectory(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	root := &Root{resolveBackendArtifact: func(context.Context, modelseffects.ResolvedHostConfiguration, bool) (modelseffects.BackendArtifactSelection, error) {
		return modelseffects.BackendArtifactSelection{InstalledPath: directory}, nil
	}}
	selection, err := root.resolveJoinedBackendArtifact(context.Background(), modelseffects.ResolvedHostConfiguration{Backend: "localai-llamacpp"}, false)
	if err != nil || selection.InstalledPath != directory {
		t.Fatalf("installed backend selection = %#v, error = %v", selection, err)
	}
}

func TestResolveJoinedBackendArtifactRejectsRelativeInstalledDirectory(t *testing.T) {
	t.Parallel()
	root := &Root{resolveBackendArtifact: func(context.Context, modelseffects.ResolvedHostConfiguration, bool) (modelseffects.BackendArtifactSelection, error) {
		return modelseffects.BackendArtifactSelection{InstalledPath: "relative/backend"}, nil
	}}
	_, err := root.resolveJoinedBackendArtifact(context.Background(), modelseffects.ResolvedHostConfiguration{Backend: "localai-whisper"}, false)
	if !errors.Is(err, models.ErrHostMissingAssets) {
		t.Fatalf("relative installed directory error = %v, want ErrHostMissingAssets", err)
	}
}

func TestResolveJoinedBackendArtifactPreservesOfflineFailure(t *testing.T) {
	t.Parallel()
	root := &Root{resolveBackendArtifact: func(_ context.Context, _ modelseffects.ResolvedHostConfiguration, offline bool) (modelseffects.BackendArtifactSelection, error) {
		if !offline {
			t.Fatal("backend resolver did not receive offline policy")
		}
		return modelseffects.BackendArtifactSelection{}, models.ErrAssetOffline
	}}
	_, err := root.resolveJoinedBackendArtifact(context.Background(), modelseffects.ResolvedHostConfiguration{Backend: "localai-whisper"}, true)
	if !errors.Is(err, models.ErrAssetOffline) {
		t.Fatalf("offline backend error = %v, want ErrAssetOffline", err)
	}
}

func TestCustomAudioCPPFileTTSReachesAssetPreparation(t *testing.T) {
	t.Parallel()
	var events []string
	var assetRequests []models.PrepareModelAssetsRequest
	inference := &joinedInferenceService{events: &events, result: joinedCompletedResult(t)}
	root, _, _ := newJoinedInvocationRootWithModel(t, &events, inference, "./unused.gguf", "fixture-backend", &assetRequests)
	source := "file:///models/custom-voice.gguf"
	backend := "localai-audio-cpp"
	loadPolicy := models.LoadPolicyOnDemand
	ref, err := root.runtimeScopes.Open(models.RuntimeBinding{
		OperatorModels: map[string]models.ModelOverlay{
			"custom-voice": {
				Source: &source, Backend: &backend, LoadPolicy: &loadPolicy,
				Operations: []string{models.OperationTTS},
			},
		},
		RuntimeConfig: func() *models.RuntimeConfig { return &models.RuntimeConfig{} },
	})
	if err != nil {
		t.Fatalf("open custom TTS scope: %v", err)
	}
	scope, err := (models.RuntimeScopeRef{}).Parse(string(ref))
	if err != nil {
		t.Fatalf("parse custom TTS scope: %v", err)
	}
	root.backendArtifactPlatform = models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64", CUDAAvailable: true}
	installedBackend := t.TempDir()
	root.resolveBackendArtifact = func(_ context.Context, configuration modelseffects.ResolvedHostConfiguration, _ bool) (modelseffects.BackendArtifactSelection, error) {
		if configuration.Backend != backend {
			t.Fatalf("resolved backend = %q, want %q", configuration.Backend, backend)
		}
		return modelseffects.BackendArtifactSelection{InstalledPath: installedBackend, Accelerator: "cuda"}, nil
	}
	request := models.InvokeModelRequest{
		Scope: scope, Holder: "custom-tts-holder", Model: models.ModelReference{NameOrURI: "custom-voice"},
		Operation: models.OperationTTS,
		Inputs:    []models.InferenceInput{{Name: "text", Modality: models.ModalityText, Content: "hello"}},
	}
	_, invokeErr := root.InvokeModel(context.Background(), request)
	if len(assetRequests) != 1 {
		t.Fatalf("asset preparation requests = %#v, invoke error = %v, want one custom TTS request", assetRequests, invokeErr)
	}
	prepared := assetRequests[0]
	if prepared.Scope != scope || prepared.Name != "custom-voice" || prepared.Backend != backend ||
		prepared.Reference.NameOrURI != "custom-voice" || len(prepared.Artifacts) != 0 {
		t.Fatalf("asset preparation = %#v, want private local source without VibeVoice roles", prepared)
	}
	visible, err := prepareJoinedAssetFixture(request, "custom-voice", models.ResolvedModelReference{
		Definition: models.ModelDefinition{Name: "custom-voice", Source: source, Backend: backend},
	})
	if err != nil {
		t.Fatalf("prepare visible local GGUF source: %v", err)
	}
	if visible.Reference.NameOrURI != source || len(visible.Artifacts) != 1 || visible.Artifacts[0].Name != "custom-voice.gguf" {
		t.Fatalf("visible local source requirements = %#v, want one GGUF without VibeVoice roles", visible)
	}
}

func TestRootPullModelForScopeRoutesDottedOperatorNameBeforeLegacyCatalogPull(t *testing.T) {
	t.Parallel()

	const name = "index-tts2.5"
	source := "file:///models/audio-cpp/index-tts2_5-orig.gguf"
	backend := "localai-audio-cpp"
	loadPolicy := models.LoadPolicyOnDemand
	root, scope, assets, runtime := newPullFallbackRoot(t, name, map[string]models.ModelOverlay{
		name: {Source: &source, Backend: &backend, LoadPolicy: &loadPolicy, Operations: []string{models.OperationTTS}},
	})
	runtime.err = models.ErrAssetSourceMissing
	resolved, err := root.ResolveModelReference(context.Background(), models.ResolveModelReferenceRequest{
		Scope: scope, Reference: models.ModelReference{NameOrURI: name},
	})
	if err != nil || resolved.Resolved.Definition.Name != name || resolved.Resolved.Definition.Backend != backend ||
		resolved.Resolved.Provenance.SourceKind != models.ModelReferenceSourceFileURI {
		t.Fatalf("operator resolution = %#v, error = %v, want named file source", resolved, err)
	}

	result, err := root.PullModelForScope(context.Background(), models.PullModelRequest{Scope: scope, Name: name})
	if err != nil {
		t.Fatalf("PullModelForScope(%q): %v", name, err)
	}
	if runtime.pullCalls != 0 {
		t.Fatalf("legacy catalog pull calls = %d, want zero", runtime.pullCalls)
	}
	if result.ModelName != name || result.ManagedPullOutcome != "INSTALLED_SUCCESSFULLY" {
		t.Fatalf("pull result = %#v, want installed operator model", result)
	}
	if assets.request.Scope != scope || assets.request.Name != name ||
		assets.request.Reference.NameOrURI != name || assets.request.Backend != backend ||
		len(assets.request.Artifacts) != 0 {
		t.Fatalf("asset preparation request = %#v, want private source resolution and configured backend", assets.request)
	}
}

// The shared executor consumes operation data, while collaborators remain fixed.
// This component proof intentionally does not assemble a Root or prefill its graph.
func TestLocalExecutorTwoScopesUseSelectedOperationConfiguration(t *testing.T) {
	t.Parallel()
	a, b := scopedHandleRequest(t, "selected-a"), scopedHandleRequest(t, "selected-b")
	configA := &models.RuntimeConfig{FactoryDirectory: "revision-a", BaseDirectory: "cache-a"}
	configB := &models.RuntimeConfig{FactoryDirectory: "revision-b", BaseDirectory: "cache-b"}
	host, runtime := &operationConfigHost{}, &operationConfigRuntime{}
	resources := mustResourceLimiter(t)
	executor, err := newLocalExecutor(host, runtime, resources,
		modelseffects.LocalRuntimeHooks{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	for _, call := range []struct {
		request models.LocalInvocationRequest
		config  *models.RuntimeConfig
	}{
		{a, configA}, {b, configB}, {a, configA}, {b, configB},
	} {
		assertOperationConfigInvocation(t, executor, ctx, call.request, call.config)
		factory, worker := localExecutionConfiguration(call.request)
		release, err := resources.Acquire(ctx, call.request.Scope, factory, worker)
		if err != nil || release == nil {
			t.Fatalf("capacity after invocation: %v", err)
		}
		release()
	}
	configB = &models.RuntimeConfig{FactoryDirectory: "revision-b2", BaseDirectory: "cache-b2"}
	assertOperationConfigInvocation(t, executor, ctx, b, configB)
	executor.CloseScope(a.Scope)
	if _, err := executor.InvokeLocal(ctx, a, configA, operationConfigAssets{}); !errors.Is(err, models.ErrRuntimeScopeClosed) {
		t.Fatalf("closed A invocation = %v", err)
	}
	assertOperationConfigInvocation(t, executor, ctx, b, configB)
	host.assertEveryLeaseReleasedOnce(t)
	if host.acquires.Load() != 6 || host.releases.Load() != 6 || runtime.loads.Load() != 3 {
		t.Fatalf("acquires/releases/loads = %d/%d/%d", host.acquires.Load(), host.releases.Load(), runtime.loads.Load())
	}
}

func TestLocalExecutorConcurrentScopesPreservePeerWhileResolutionCloses(t *testing.T) {
	t.Parallel()
	a, b := scopedHandleRequest(t, "gated-a"), scopedHandleRequest(t, "gated-b")
	configA := &models.RuntimeConfig{FactoryDirectory: "revision-a", BaseDirectory: "cache-a"}
	configB := &models.RuntimeConfig{FactoryDirectory: "revision-b", BaseDirectory: "cache-b"}
	started, resume := make(chan struct{}), make(chan struct{})
	host, runtime := &operationConfigHost{}, &operationConfigRuntime{}
	executor, err := newLocalExecutor(host, runtime, mustResourceLimiter(t), modelseffects.LocalRuntimeHooks{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := executor.InvokeLocal(ctx, a, configA, operationConfigAssets{started: started, resume: resume})
		done <- err
	}()
	// Context cancellation also unblocks the test-owned effect during failure cleanup.
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("A did not enter cache resolution")
	}
	assertOperationConfigInvocation(t, executor, ctx, b, configB)
	executor.CloseScope(a.Scope)
	close(resume)
	select {
	case err := <-done:
		if !errors.Is(err, models.ErrRuntimeScopeClosed) {
			t.Fatalf("late A invocation = %v", err)
		}
	case <-ctx.Done():
		t.Fatal("closed A did not return")
	}
	assertOperationConfigInvocation(t, executor, ctx, b, configB)
	host.assertEveryLeaseReleasedOnce(t)
	if host.acquires.Load() != 3 || host.releases.Load() != 3 || runtime.loads.Load() != 1 {
		t.Fatalf("acquires/releases/loads = %d/%d/%d", host.acquires.Load(), host.releases.Load(), runtime.loads.Load())
	}
}

func TestLocalExecutorMissingOperationConfigurationFailsBeforeEffects(t *testing.T) {
	t.Parallel()
	host, runtime := &operationConfigHost{}, &operationConfigRuntime{}
	executor, err := newLocalExecutor(host, runtime, mustResourceLimiter(t),
		modelseffects.LocalRuntimeHooks{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	request := scopedHandleRequest(t, "missing-config")
	result, err := executor.InvokeLocal(t.Context(), request, nil, operationConfigAssets{})
	if err == nil || !result.Handled || host.acquires.Load() != 0 || runtime.loads.Load() != 0 {
		t.Fatalf("missing configuration = %#v, %v; acquires/loads = %d/%d", result, err, host.acquires.Load(), runtime.loads.Load())
	}
	request.Worker.Type = "COMMAND"
	result, err = executor.InvokeLocal(t.Context(), request, nil, operationConfigAssets{})
	if err != nil || result.Handled {
		t.Fatalf("unmanaged request = %#v, %v", result, err)
	}
}

func assertOperationConfigInvocation(t *testing.T, executor *localExecutor, ctx context.Context,
	request models.LocalInvocationRequest, config *models.RuntimeConfig) {
	t.Helper()
	result, err := executor.InvokeLocal(ctx, request, config, operationConfigAssets{})
	want := config.BaseDirectory + "|" + config.FactoryDirectory + "|http://" + config.FactoryDirectory
	if err != nil || !result.Handled || result.Content != want {
		t.Fatalf("invoke %s = %#v, %v; want %q", request.Scope, result, err, want)
	}
}

type operationConfigHost struct {
	mu       sync.Mutex
	released map[string]int
	leaseTestHost
	acquires, releases atomic.Int32
}

func (h *operationConfigHost) AcquireLease(_ context.Context, config *models.RuntimeConfig,
	_ string, _ modelhost.LeaseOptions) (modelhost.Lease, error) {
	return modelhost.Lease{ID: fmt.Sprint(h.acquires.Add(1)), Endpoint: "http://" + config.FactoryDirectory}, nil
}
func (h *operationConfigHost) ReleaseLease(_ context.Context, id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.released == nil {
		h.released = make(map[string]int)
	}
	h.released[id]++
	h.releases.Add(1)
	return nil
}

type operationConfigAssets struct {
	leaseTestAssets
	started, resume chan struct{}
}

func (a operationConfigAssets) ResolveModelCache(ctx context.Context, config *models.RuntimeConfig,
	worker *models.RuntimeWorker) (localmodels.CacheLayout, error) {
	if a.started != nil && config.FactoryDirectory == "revision-a" {
		close(a.started)
		select {
		case <-a.resume:
		case <-ctx.Done():
			return localmodels.CacheLayout{}, ctx.Err()
		}
	}
	return localmodels.CacheLayout{ModelName: worker.Model, CachePath: config.BaseDirectory,
		Revision: config.FactoryDirectory}, nil
}

type operationConfigRuntime struct{ loads atomic.Int32 }

func (*operationConfigRuntime) Supports(models.RuntimeResource, *models.RuntimeWorker) bool {
	return true
}
func (r *operationConfigRuntime) Load(_ context.Context, request localmodels.LoadRequest) (localmodels.Handle, error) {
	r.loads.Add(1)
	return operationConfigHandle{content: request.CachePath + "|" + request.Revision + "|" + request.ServingEndpoint}, nil
}

type operationConfigHandle struct{ content string }

func (h operationConfigHandle) Invoke(context.Context, localmodels.InvocationRequest) (localmodels.InvocationResponse, error) {
	return localmodels.InvocationResponse{Content: h.content}, nil
}

func (h *operationConfigHost) assertEveryLeaseReleasedOnce(t *testing.T) {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	for id := int32(1); id <= h.acquires.Load(); id++ {
		if count := h.released[fmt.Sprint(id)]; count != 1 {
			t.Fatalf("lease %d released %d times, want once", id, count)
		}
	}
}

func TestLocalExecutorTwoScopesSelectAssetsPerInvocation(t *testing.T) {
	t.Parallel()
	a, b := scopedHandleRequest(t, "assets-a"), scopedHandleRequest(t, "assets-b")
	config := &models.RuntimeConfig{FactoryDirectory: "shared-config"}
	host, runtime := &operationConfigHost{}, &operationConfigRuntime{}
	executor, err := newLocalExecutor(host, runtime, mustResourceLimiter(t), modelseffects.LocalRuntimeHooks{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	assetsA := &operationSelectedAssets{layout: localmodels.CacheLayout{CachePath: "cache-a", Revision: "revision-a"}}
	assetsB := &operationSelectedAssets{layout: localmodels.CacheLayout{CachePath: "cache-b", Revision: "revision-b"}}
	for _, call := range []struct {
		request models.LocalInvocationRequest
		assets  *operationSelectedAssets
	}{{a, assetsA}, {b, assetsB}, {a, assetsA}, {b, assetsB}} {
		result, err := executor.InvokeLocal(t.Context(), call.request, config, call.assets)
		want := call.assets.layout.CachePath + "|" + call.assets.layout.Revision + "|http://shared-config"
		if err != nil || !result.Handled || result.Content != want {
			t.Fatalf("invoke %s = %#v, %v; want %q", call.request.Scope, result, err, want)
		}
	}
	executor.CloseScope(a.Scope)
	if _, err := executor.InvokeLocal(t.Context(), b, config, assetsB); err != nil {
		t.Fatal(err)
	}
	host.assertEveryLeaseReleasedOnce(t)
	if assetsA.calls.Load() != 2 || assetsB.calls.Load() != 3 || runtime.loads.Load() != 2 {
		t.Fatalf("asset A/B resolutions, loads = %d/%d/%d", assetsA.calls.Load(), assetsB.calls.Load(), runtime.loads.Load())
	}
}

type operationSelectedAssets struct {
	leaseTestAssets
	layout localmodels.CacheLayout
	calls  atomic.Int32
}

func (a *operationSelectedAssets) ResolveModelCache(context.Context, *models.RuntimeConfig, *models.RuntimeWorker) (localmodels.CacheLayout, error) {
	a.calls.Add(1)
	return a.layout, nil
}

func TestLocalExecutorMissingOperationAssetsFailsBeforeEffects(t *testing.T) {
	t.Parallel()
	var typedNil *operationSelectedAssets
	for _, assets := range []localmodels.AssetPuller{nil, typedNil} {
		host, runtime := &operationConfigHost{}, &operationConfigRuntime{}
		executor, err := newLocalExecutor(host, runtime, mustResourceLimiter(t), modelseffects.LocalRuntimeHooks{}, time.Now)
		if err != nil {
			t.Fatal(err)
		}
		request := scopedHandleRequest(t, "missing-assets")
		result, err := executor.InvokeLocal(t.Context(), request, &models.RuntimeConfig{}, assets)
		if !errors.Is(err, ErrInvalidDependencies) || !result.Handled || host.acquires.Load() != 0 || runtime.loads.Load() != 0 {
			t.Fatalf("missing assets = %#v, %v; acquires/loads = %d/%d", result, err, host.acquires.Load(), runtime.loads.Load())
		}
		request.Worker.Type = "COMMAND"
		if result, err := executor.InvokeLocal(t.Context(), request, nil, assets); err != nil || result.Handled {
			t.Fatalf("unmanaged invocation = %#v, %v", result, err)
		}
	}
}

func TestLocalExecutorLateResolutionReleasesLeaseWithoutFurtherEffects(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"lease", "assets"} {
		for _, stop := range []string{"scope", "process", "cancel"} {
			t.Run(stage+"/"+stop, func(t *testing.T) {
				t.Parallel()
				a, b := scopedHandleRequest(t, "late-a"), scopedHandleRequest(t, "late-b")
				configA := &models.RuntimeConfig{FactoryDirectory: "revision-a", BaseDirectory: "cache-a"}
				configB := &models.RuntimeConfig{FactoryDirectory: "revision-b", BaseDirectory: "cache-b"}
				started, resume := make(chan struct{}), make(chan struct{})
				defer close(resume)
				host := &lateResolutionHost{stage: stage, started: started, resume: resume}
				assets := &lateResolutionAssets{stage: stage, started: started, resume: resume}
				runtime, resources := &operationConfigRuntime{}, mustResourceLimiter(t)
				executor, err := newLocalExecutor(host, runtime, resources, modelseffects.LocalRuntimeHooks{}, time.Now)
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
				defer cancel()
				invocationCtx, cancelInvocation := context.WithCancel(ctx)
				defer cancelInvocation()
				done := make(chan error, 1)
				go func() {
					result, err := executor.InvokeLocal(invocationCtx, a, configA, assets)
					if !result.Handled || result.Content != "" {
						err = fmt.Errorf("late result = %#v", result)
					}
					done <- err
				}()
				select {
				case <-started:
				case <-ctx.Done():
					t.Fatal("resolution did not start")
				}
				assertOperationConfigInvocation(t, executor, ctx, b, configB)
				want := stopLateResolution(executor, a.Scope, stop, cancelInvocation)
				resume <- struct{}{}
				assertLateResolutionFinished(t, ctx, done, want)
				if stage == "lease" && assets.calls.Load() != 0 {
					t.Fatal("retired acquisition entered assets")
				}
				if runtime.loads.Load() != 1 || host.canceledRelease.Load() {
					t.Fatalf("loads=%d canceled cleanup=%v", runtime.loads.Load(), host.canceledRelease.Load())
				}
				if stop != "process" {
					assertOperationConfigInvocation(t, executor, ctx, b, configB)
				}
				host.assertEveryLeaseReleasedOnce(t)
				factory, worker := localExecutionConfiguration(a)
				release, err := resources.Acquire(ctx, a.Scope, factory, worker)
				if err != nil || release == nil {
					t.Fatalf("late invocation retained capacity: %v", err)
				}
				release()
			})
		}
	}
}

func stopLateResolution(executor *localExecutor, scope models.RuntimeScopeRef, stop string, cancel context.CancelFunc) error {
	switch stop {
	case "scope":
		executor.CloseScope(scope)
	case "process":
		executor.Close()
	case "cancel":
		cancel()
		return context.Canceled
	}
	return models.ErrRuntimeScopeClosed
}

type lateResolutionHost struct {
	operationConfigHost
	stage           string
	started, resume chan struct{}
	canceledRelease atomic.Bool
}

func (h *lateResolutionHost) AcquireLease(ctx context.Context, config *models.RuntimeConfig,
	name string, options modelhost.LeaseOptions) (modelhost.Lease, error) {
	if h.stage == "lease" && config.FactoryDirectory == "revision-a" {
		close(h.started)
		<-h.resume
	}
	return h.operationConfigHost.AcquireLease(ctx, config, name, options)
}

func (h *lateResolutionHost) ReleaseLease(ctx context.Context, id string) error {
	if ctx.Err() != nil {
		h.canceledRelease.Store(true)
	}
	return h.operationConfigHost.ReleaseLease(ctx, id)
}

type lateResolutionAssets struct {
	operationSelectedAssets
	stage           string
	started, resume chan struct{}
}

func (a *lateResolutionAssets) ResolveModelCache(ctx context.Context, config *models.RuntimeConfig,
	worker *models.RuntimeWorker) (localmodels.CacheLayout, error) {
	if a.stage == "assets" {
		close(a.started)
		<-a.resume
	}
	return a.operationSelectedAssets.ResolveModelCache(ctx, config, worker)
}

// localExecutionFixture adapts the isolated executor to Root's operation port.
// Production resolves scopes through the completed shared execution owner.
type localExecutionFixture struct {
	*Service
	local *localExecutor
}

func (s *localExecutionFixture) PullModelForScope(ctx context.Context, request models.PullModelRequest) (models.PullResult, error) {
	if err := models.ValidatePullModelRequest(request); err != nil {
		return models.PullResult{}, err
	}
	return s.PullModel(ctx, request.Name)
}

func (s *localExecutionFixture) InvokeLocal(ctx context.Context, request models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	if err := models.ValidateLocalInvocationRequest(request); err != nil {
		return models.LocalInvocationResult{}, err
	}
	if s.local == nil || !request.Worker.UsesManagedRuntime() {
		return models.LocalInvocationResult{}, nil
	}
	return s.local.InvokeLocal(ctx, request, s.runtimeConfig(), s.assetPuller)
}
