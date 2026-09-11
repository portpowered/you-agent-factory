package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

import models "github.com/portpowered/infinite-you/pkg/services/models"
import modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
import scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"

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

func TestRuntimeServiceContractOnlyOperationsFailExplicitly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := &runtimeService{}
	_, err := svc.OpenRuntimeScope(ctx, models.OpenRuntimeScopeRequest{})
	assertContractOnlyUnsupported(t, "OpenRuntimeScope", err)
	_, err = svc.CloseRuntimeScope(ctx, models.CloseRuntimeScopeRequest{})
	assertContractOnlyUnsupported(t, "CloseRuntimeScope", err)
	_, err = svc.PrepareModelAssets(ctx, models.PrepareModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "PrepareModelAssets", err)
	_, err = svc.InspectModelAssets(ctx, models.InspectModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "InspectModelAssets", err)
	_, err = svc.RemoveModelAssets(ctx, models.RemoveModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "RemoveModelAssets", err)
	_, err = svc.ResolveModelReference(ctx, models.ResolveModelReferenceRequest{})
	assertContractOnlyUnsupported(t, "ResolveModelReference", err)
	_, err = svc.InvokeModel(ctx, models.InvokeModelRequest{})
	assertContractOnlyUnsupported(t, "InvokeModel", err)
	result, err := svc.InvokeLocal(ctx, models.LocalInvocationRequest{})
	if err != nil || result.Handled {
		t.Fatalf("InvokeLocal result = %#v, error = %v, want declined no-op", result, err)
	}
	_, err = svc.InvokeLocal(ctx, models.LocalInvocationRequest{
		Worker: models.LocalWorker{
			Type:          models.RuntimeWorkerTypeInference,
			ModelLocality: models.RuntimeModelLocalityLocal,
		},
	})
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("managed InvokeLocal error = %v, want ErrNotFound", err)
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
		resolveBackendArtifact: func(_ context.Context, request modelseffects.ResolvedHostConfiguration) (modelseffects.BackendArtifactSelection, error) {
			observed = request.Clone()
			request.ModelFiles[0] = "mutated-by-selector"
			request.Source.NameOrURI = "mutated-by-selector"
			return modelseffects.BackendArtifactSelection{
				Name: "backend.tar.gz", Location: "https://example.invalid/backend.tar.gz",
				Bytes: 1, SHA256: "10a84e67d02d078f711608accf13cb80b6724a4c03dc4acae5ba936831801172",
			}, nil
		},
	}
	selection, err := root.resolveJoinedBackendArtifact(context.Background(), configuration)
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
	root.process = modelseffects.ProcessDependencies{BackendArtifactPlatform: platform}
	root.resolveBackendArtifact = func(_ context.Context, configuration modelseffects.ResolvedHostConfiguration) (modelseffects.BackendArtifactSelection, error) {
		observed = configuration.Clone()
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
			root := &Root{resolveBackendArtifact: func(context.Context, modelseffects.ResolvedHostConfiguration) (modelseffects.BackendArtifactSelection, error) {
				return test.selection, nil
			}}
			_, err := root.resolveJoinedBackendArtifact(context.Background(), base)
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
	request, err := joinedAssetPreparationRequest(
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
