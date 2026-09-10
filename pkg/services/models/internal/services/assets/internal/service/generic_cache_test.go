package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	platformlocking "github.com/portpowered/infinite-you/pkg/platform/locking"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	assets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

const genericTestRevision = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestGenericModelRequirementsExpandOnlyPinnedGemmaLLMSource(t *testing.T) {
	t.Parallel()

	scopes := newScopes(t, "generic-gemma-requirements")
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("requirement planning must not use HTTP")
		return nil, nil
	}), func(string) string { return "" })
	scope := openScope(t, scopes, t.TempDir(), models.RuntimeConfig{})

	plan, err := service.genericPreparationPlan(context.Background(), models.PrepareModelAssetsRequest{
		Scope: scope,
		Name:  models.BuiltInModelNameLLM,
		// The joined readiness path supplies only the model file today. The
		// exact built-in source must override that incomplete explicit list.
		Artifacts: []models.AssetRequirement{{Name: builtInGemmaLLMModelName}},
	})
	if err != nil {
		t.Fatalf("genericPreparationPlan: %v", err)
	}
	if !isBuiltInGemmaLLMSource(plan.source) || len(plan.modelRequirements) != 2 {
		t.Fatalf("gemma plan source/requirements = %#v/%#v", plan.source, plan.modelRequirements)
	}
	got := make([]models.AssetRequirement, 0, len(plan.modelRequirements))
	for _, artifact := range plan.modelRequirements {
		got = append(got, artifact.requirement)
	}
	want := []models.AssetRequirement{
		{Name: builtInGemmaLLMModelName, Bytes: builtInGemmaLLMModelBytes, SHA256: builtInGemmaLLMModelSHA256},
		{Name: builtInGemmaLLMProjectorName, Bytes: builtInGemmaLLMProjectorBytes, SHA256: builtInGemmaLLMProjectorSHA256},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gemma requirements = %#v, want %#v", got, want)
	}

	for name, mutate := range map[string]func(*genericSource){
		"different model":      func(source *genericSource) { source.modelName = "other" },
		"different owner":      func(source *genericSource) { source.owner = "other" },
		"different repository": func(source *genericSource) { source.repository = "other" },
		"different file":       func(source *genericSource) { source.file = "other.gguf" },
		"different revision":   func(source *genericSource) { source.revision = genericTestRevision },
	} {
		candidate := plan.source
		mutate(&candidate)
		if isBuiltInGemmaLLMSource(candidate) {
			t.Fatalf("isBuiltInGemmaLLMSource accepted %s: %#v", name, candidate)
		}
	}
}

func TestPrepareGenericAssetsRepairsPartialContentAddressedSnapshot(t *testing.T) {
	t.Parallel()

	modelBody := []byte("legacy model body")
	projectorBody := []byte("new projector body")
	source := genericSource{
		kind: genericSourceHF, safe: "hf://owner/repo@" + genericTestRevision,
		owner: "owner", repository: "repo", revision: genericTestRevision,
	}
	modelRequirement := models.AssetRequirement{
		Name: "model.bin", Bytes: int64(len(modelBody)), SHA256: sha256Hex(modelBody),
	}
	projectorRequirement := models.AssetRequirement{
		Name: "projector.bin", Bytes: int64(len(projectorBody)), SHA256: sha256Hex(projectorBody),
	}
	cacheDirectory := t.TempDir()
	legacyIdentity := genericArtifactIdentityHash(assetKindModel, source, []genericArtifact{{requirement: modelRequirement}})
	legacySnapshot := filepath.Join(cacheDirectory, assetContentDirectory, assetKindModel, legacyIdentity)
	if err := os.MkdirAll(legacySnapshot, 0o755); err != nil {
		t.Fatalf("create legacy snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacySnapshot, modelRequirement.Name), modelBody, 0o644); err != nil {
		t.Fatalf("write legacy model: %v", err)
	}
	legacyMetadata := genericCacheMetadata{
		Kind: assetKindModel, Identity: genericCacheKey(assetKindModel, source, []genericArtifact{{requirement: modelRequirement}}),
		Source: source.safe, SourceKey: genericSourceIdentity(source),
		Artifacts: []models.AssetRequirement{modelRequirement},
	}
	metadataBody, err := json.Marshal(legacyMetadata)
	if err != nil {
		t.Fatalf("marshal legacy metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacySnapshot, assetMetadataName), metadataBody, 0o644); err != nil {
		t.Fatalf("write legacy metadata: %v", err)
	}

	var downloads atomic.Int32
	serviceScopes := newScopes(t, "generic-partial-repair")
	scope := openScope(t, serviceScopes, cacheDirectory, models.RuntimeConfig{})
	service := newGenericService(t, serviceScopes, httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || strings.HasPrefix(request.URL.Path, "/models/") {
			t.Fatalf("unexpected partial-repair request: %s %s", request.Method, request.URL)
		}
		downloads.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(projectorBody))}, nil
	}), func(string) string { return "" })
	request := models.PrepareModelAssetsRequest{
		Scope: scope, Name: "partial-model",
		Reference: models.ModelReference{NameOrURI: source.safe},
		Artifacts: []models.AssetRequirement{modelRequirement, projectorRequirement},
	}
	result, err := service.PrepareModelAssets(context.Background(), request)
	if err != nil {
		t.Fatalf("partial snapshot repair: %v", err)
	}
	if downloads.Load() != 1 || len(result.Asset.Artifacts) != 2 ||
		result.Asset.Artifacts[0].Name != modelRequirement.Name ||
		result.Asset.Artifacts[1].Name != projectorRequirement.Name {
		t.Fatalf("partial repair result/downloads = %#v/%d", result.Asset, downloads.Load())
	}
	newIdentity := genericArtifactIdentityHash(assetKindModel, source, []genericArtifact{
		{requirement: modelRequirement}, {requirement: projectorRequirement},
	})
	newSnapshot := filepath.Join(cacheDirectory, assetContentDirectory, assetKindModel, newIdentity)
	assertFileBody(t, filepath.Join(newSnapshot, modelRequirement.Name), modelBody)
	assertFileBody(t, filepath.Join(newSnapshot, projectorRequirement.Name), projectorBody)
	assertFileBody(t, filepath.Join(legacySnapshot, modelRequirement.Name), modelBody)
}

func TestPrepareGenericAssetsOfflinePartialSnapshotReportsOnlyMissingMembers(t *testing.T) {
	t.Parallel()

	body := []byte("cached model body")
	source := genericSource{
		kind: genericSourceHF, safe: "hf://owner/repo@" + genericTestRevision,
		owner: "owner", repository: "repo", revision: genericTestRevision,
	}
	modelRequirement := models.AssetRequirement{
		Name: "model.bin", Bytes: int64(len(body)), SHA256: sha256Hex(body),
	}
	projectorRequirement := models.AssetRequirement{Name: "projector.bin", Bytes: 8, SHA256: sha256Hex([]byte("missing!"))}
	cacheDirectory := t.TempDir()
	legacyIdentity := genericArtifactIdentityHash(assetKindModel, source, []genericArtifact{{requirement: modelRequirement}})
	legacySnapshot := filepath.Join(cacheDirectory, assetContentDirectory, assetKindModel, legacyIdentity)
	if err := os.MkdirAll(legacySnapshot, 0o755); err != nil {
		t.Fatalf("create offline legacy snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacySnapshot, modelRequirement.Name), body, 0o644); err != nil {
		t.Fatalf("write offline legacy model: %v", err)
	}
	metadataBody, err := json.Marshal(genericCacheMetadata{
		Kind: assetKindModel, Identity: genericCacheKey(assetKindModel, source, []genericArtifact{{requirement: modelRequirement}}),
		Source: source.safe, SourceKey: genericSourceIdentity(source),
		Artifacts: []models.AssetRequirement{modelRequirement},
	})
	if err != nil {
		t.Fatalf("marshal offline legacy metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacySnapshot, assetMetadataName), metadataBody, 0o644); err != nil {
		t.Fatalf("write offline legacy metadata: %v", err)
	}

	var requests atomic.Int32
	serviceScopes := newScopes(t, "generic-partial-offline")
	scope := openScope(t, serviceScopes, cacheDirectory, models.RuntimeConfig{})
	service := newGenericService(t, serviceScopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("offline partial snapshot used network")
	}), func(string) string { return "" })
	_, err = service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: scope, Name: "partial-model", Reference: models.ModelReference{NameOrURI: source.safe}, Offline: true,
		Artifacts: []models.AssetRequirement{modelRequirement, projectorRequirement},
	})
	var offline *models.AssetOfflineError
	if !errors.As(err, &offline) || !reflect.DeepEqual(offline.Missing, []string{projectorRequirement.Name}) {
		t.Fatalf("offline partial error = %v, want only %q", err, projectorRequirement.Name)
	}
	if requests.Load() != 0 {
		t.Fatalf("offline partial network requests = %d, want 0", requests.Load())
	}
	assertFileBody(t, filepath.Join(legacySnapshot, modelRequirement.Name), body)
}

func TestPrepareGenericAssetsUsesOrderedHFCachesWithoutNetworkOnHit(t *testing.T) {
	t.Parallel()

	first := t.TempDir()
	secondHome := t.TempDir()
	second := filepath.Join(secondHome, "hub")
	third := t.TempDir()
	you := t.TempDir()
	body := []byte("cached model")
	writeGenericHFFixture(t, first, body)

	var requests atomic.Int32
	client := httpDoerFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("network should not be used for a cache hit")
	})
	environment := func(name string) string {
		switch name {
		case "HUGGINGFACE_HUB_CACHE":
			return first
		case "HF_HOME":
			return secondHome
		default:
			return ""
		}
	}
	scopes := newScopes(t, "generic-cache-order")
	scope := openScope(t, scopes, you, models.RuntimeConfig{})
	service := newGenericService(t, scopes, client, environment)

	result, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope:     scope,
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo/weights.bin@" + genericTestRevision},
		Artifacts: []models.AssetRequirement{{Name: "weights.bin", Bytes: int64(len(body)), SHA256: sha256Hex(body)}},
	})
	if err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	assertGenericHFCachedResult(t, result, body, requests.Load())
	assertGenericRootUntouched(t, second)
	assertGenericRootUntouched(t, filepath.Join(third, "models--owner--repo"))
}

func TestPrepareModelAssetsReportsMissingDirectorySourceAtBoundary(t *testing.T) {
	t.Parallel()

	scopes := newScopes(t, "missing-directory-source")
	cacheDirectory := t.TempDir()
	scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	missingPath := filepath.Join(t.TempDir(), "missing-model")
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("missing local source used the network")
		return nil, nil
	}), func(string) string { return "" })

	_, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      "missing-model",
		Reference: models.ModelReference{NameOrURI: missingPath},
	})
	if !errors.Is(err, models.ErrAssetSourceMissing) {
		t.Fatalf("PrepareModelAssets error = %v, want ErrAssetSourceMissing", err)
	}
	var stageErr *models.PullStageError
	if !errors.As(err, &stageErr) || stageErr.Stage != models.PullStageSourceResolution {
		t.Fatalf("PrepareModelAssets stage error = %#v, want source-resolution stage", stageErr)
	}
	if _, statErr := os.Stat(filepath.Join(cacheDirectory, "MISSING-MODEL")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("missing source created managed cache root: %v", statErr)
	}
}

func TestPrepareGenericAssetsFallsThroughOrderedHFCaches(t *testing.T) {
	t.Parallel()

	first := t.TempDir()
	secondHome := t.TempDir()
	second := filepath.Join(secondHome, "hub")
	body := []byte("verified second cache")
	writeGenericHFFixture(t, first, []byte("stale cache"))
	writeGenericHFFixture(t, second, body)
	var requests atomic.Int32
	client := httpDoerFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("ordered cache hit must not use network")
	})
	environment := func(name string) string {
		switch name {
		case "HUGGINGFACE_HUB_CACHE":
			return first
		case "HF_HOME":
			return secondHome
		default:
			return ""
		}
	}
	scopes := newScopes(t, "generic-cache-fallback")
	scope := openScope(t, scopes, t.TempDir(), models.RuntimeConfig{})
	service := newGenericService(t, scopes, client, environment)

	result, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope:     scope,
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo/weights.bin@" + genericTestRevision},
		Artifacts: []models.AssetRequirement{{Name: "weights.bin", Bytes: int64(len(body)), SHA256: sha256Hex(body)}},
	})
	if err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	if requests.Load() != 0 || len(result.Asset.Artifacts) != 1 ||
		result.Asset.Artifacts[0].SHA256 != sha256Hex(body) {
		t.Fatalf("ordered cache result = %#v, network requests = %d", result, requests.Load())
	}
}

func writeGenericHFFixture(t *testing.T, root string, body []byte) {
	t.Helper()
	path := filepath.Join(root, "models--owner--repo", "snapshots", genericTestRevision, "weights.bin")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create HF cache fixture: %v", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write HF cache fixture: %v", err)
	}
}

func assertGenericHFCachedResult(
	t *testing.T,
	result models.PrepareModelAssetsResult,
	body []byte,
	requests int32,
) {
	t.Helper()
	if requests != 0 {
		t.Fatalf("HTTP requests = %d, want 0 on HF cache hit", requests)
	}
	if result.Outcome != models.AssetPreparationAlreadyAvailable ||
		result.Asset.Readiness != models.AssetReadinessAvailable ||
		result.Asset.Integrity != models.AssetIntegrityVerified {
		t.Fatalf("cache-hit result = %#v", result)
	}
	if len(result.Asset.Artifacts) != 1 || result.Asset.Artifacts[0].Kind != models.AssetArtifactKindModel ||
		result.Asset.Artifacts[0].SHA256 != sha256Hex(body) {
		t.Fatalf("cache-hit artifacts = %#v", result.Asset.Artifacts)
	}
}

func assertGenericRootUntouched(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cache root %q was unexpectedly materialized: %v", path, err)
	}
}

func TestPrepareGenericAssetsKeepsModelAndBackendCachesSeparate(t *testing.T) {
	t.Parallel()

	localPath := filepath.Join(t.TempDir(), "payload.bin")
	body := []byte("shared fixture payload")
	if err := os.WriteFile(localPath, body, 0o644); err != nil {
		t.Fatalf("write local fixture: %v", err)
	}
	var requests atomic.Int32
	client := httpDoerFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("local sources must not use network")
	})
	scopes := newScopes(t, "generic-cache-kinds")
	you := t.TempDir()
	scope := openScope(t, scopes, you, models.RuntimeConfig{})
	service := newGenericService(t, scopes, client, func(string) string { return "" })
	request := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Reference: models.ModelReference{NameOrURI: localPath},
		Artifacts: []models.AssetRequirement{{Name: "payload.bin", Bytes: int64(len(body)), SHA256: sha256Hex(body)}},
		Backend:   "backend-v1",
		BackendArtifacts: []models.AssetRequirement{{
			Name: "payload.bin", Bytes: int64(len(body)), SHA256: sha256Hex(body),
		}},
	}
	result, err := service.PrepareModelAssets(context.Background(), request)
	if err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("HTTP requests = %d, want 0 for local sources", requests.Load())
	}
	if len(result.Asset.Artifacts) != 1 || len(result.Asset.BackendArtifacts) != 1 ||
		result.Asset.Artifacts[0].Kind != models.AssetArtifactKindModel ||
		result.Asset.BackendArtifacts[0].Kind != models.AssetArtifactKindBackend {
		t.Fatalf("separated artifacts = %#v", result.Asset)
	}
	modelPath := filepath.Join(
		you, assetContentDirectory, assetKindModel,
		genericArtifactIdentityHash(assetKindModel, genericSource{
			kind: genericSourceLocal, safe: "local://path", localPath: localPath,
		}, []genericArtifact{{
			requirement: models.AssetRequirement{
				Name: "payload.bin", Bytes: int64(len(body)), SHA256: sha256Hex(body),
			},
		}}),
		"payload.bin",
	)
	backendSource := genericSource{
		kind: genericSourceLocal, safe: "backend://backend-v1/local://path", localPath: localPath,
	}
	backendPath := filepath.Join(
		you, "backend-artifacts", assetContentDirectory, assetKindBackend,
		genericArtifactIdentityHash(assetKindBackend, backendSource, []genericArtifact{{
			requirement: models.AssetRequirement{
				Name: "payload.bin", Bytes: int64(len(body)), SHA256: sha256Hex(body),
			},
		}}), "payload.bin",
	)
	if _, err := os.Stat(modelPath); err != nil {
		t.Fatalf("model cache path %q: %v", modelPath, err)
	}
	if _, err := os.Stat(backendPath); err != nil {
		t.Fatalf("backend cache path %q: %v", backendPath, err)
	}
}

func TestPrepareGenericAssetsAcceptsFileURIWithoutNetwork(t *testing.T) {
	t.Parallel()

	localPath := filepath.Join(t.TempDir(), "weights.bin")
	body := []byte("file URI payload")
	if err := os.WriteFile(localPath, body, 0o644); err != nil {
		t.Fatalf("write local fixture: %v", err)
	}
	fileReference := (&url.URL{Scheme: "file", Path: filepath.ToSlash(localPath)}).String()
	var requests atomic.Int32
	client := httpDoerFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("file URI preparation must not use network")
	})
	scopes := newScopes(t, "generic-file-uri")
	scope := openScope(t, scopes, t.TempDir(), models.RuntimeConfig{})
	service := newGenericService(t, scopes, client, func(string) string { return "" })

	result, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope:     scope,
		Reference: models.ModelReference{NameOrURI: fileReference},
	})
	if err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	if requests.Load() != 0 || len(result.Asset.Artifacts) != 1 ||
		result.Asset.Artifacts[0].SHA256 != sha256Hex(body) {
		t.Fatalf("file URI result = %#v, network requests = %d", result, requests.Load())
	}
}

func TestPrepareGenericAssetsOfflineReportsCompleteMissingSetWithoutNetwork(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	client := httpDoerFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("offline preparation must not use network")
	})
	scopes := newScopes(t, "generic-offline")
	scope := openScope(t, scopes, t.TempDir(), models.RuntimeConfig{})
	service := newGenericService(t, scopes, client, func(string) string { return "" })
	request := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo@" + genericTestRevision},
		Offline:   true,
		Artifacts: []models.AssetRequirement{{Name: "model.bin"}, {Name: "tokenizer.bin"}},
		Backend:   "backend-v1",
		BackendArtifacts: []models.AssetRequirement{{
			Name: "runtime.bin",
		}},
	}
	_, err := service.PrepareModelAssets(context.Background(), request)
	var offline *models.AssetOfflineError
	if !errors.As(err, &offline) {
		t.Fatalf("error = %v, want AssetOfflineError", err)
	}
	if got, want := strings.Join(offline.Missing, ","), "model.bin,runtime.bin,tokenizer.bin"; got != want {
		t.Fatalf("missing artifacts = %q, want %q", got, want)
	}
	if requests.Load() != 0 {
		t.Fatalf("HTTP requests = %d, want 0 while offline", requests.Load())
	}
}

func TestPrepareGenericAssetsOfflineDiscoversPublishedRequirements(t *testing.T) {
	t.Parallel()

	body := []byte("offline snapshot")
	scopes := newScopes(t, "generic-offline-discovery")
	you := t.TempDir()
	scope := openScope(t, scopes, you, models.RuntimeConfig{})
	request := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo@" + genericTestRevision},
		Artifacts: []models.AssetRequirement{{Name: "weights.bin", SHA256: sha256Hex(body)}},
	}
	service := newGenericService(
		t, scopes, genericManifestClient("weights.bin", body, func() []byte { return body }),
		func(string) string { return "" },
	)
	if _, err := service.PrepareModelAssets(context.Background(), request); err != nil {
		t.Fatalf("initial preparation: %v", err)
	}
	var requests atomic.Int32
	service.client = httpDoerFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("offline cache discovery must not use network")
	})

	result, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope:     scope,
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo@" + genericTestRevision},
		Offline:   true,
	})
	if err != nil {
		t.Fatalf("offline preparation: %v", err)
	}
	if requests.Load() != 0 || result.Outcome != models.AssetPreparationAlreadyAvailable ||
		len(result.Asset.Artifacts) != 1 || result.Asset.Artifacts[0].Name != "weights.bin" {
		t.Fatalf("offline discovery result = %#v, network requests = %d", result, requests.Load())
	}
	explicit, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{Scope: scope, Reference: request.Reference, Offline: true, Artifacts: request.Artifacts})
	if err != nil || explicit.Outcome != models.AssetPreparationAlreadyAvailable {
		t.Fatalf("offline explicit discovery result = %#v, error = %v", explicit, err)
	}
}

func TestPrepareGenericAssetsDigestMismatchLeavesNoSnapshotAndCanRetry(t *testing.T) {
	t.Parallel()

	good := []byte("good payload")
	bad := []byte("bad payload")
	digest := sha256Hex(good)
	var downloads atomic.Int32
	client := genericManifestClient("weights.bin", good, func() []byte {
		if downloads.Add(1) == 1 {
			return bad
		}
		return good
	})
	scopes := newScopes(t, "generic-retry")
	you := t.TempDir()
	scope := openScope(t, scopes, you, models.RuntimeConfig{})
	service := newGenericService(t, scopes, client, func(string) string { return "" })
	request := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo/weights.bin@" + genericTestRevision},
		Artifacts: []models.AssetRequirement{{Name: "weights.bin", SHA256: digest, Bytes: int64(len(good))}},
	}
	if _, err := service.PrepareModelAssets(context.Background(), request); !errors.Is(err, models.ErrAssetIntegrityFailed) {
		t.Fatalf("first preparation error = %v, want integrity failure", err)
	}
	contentRoot := filepath.Join(you, assetContentDirectory, assetKindModel)
	entries, err := os.ReadDir(contentRoot)
	if err == nil && len(entries) != 0 {
		t.Fatalf("failed preparation published cache entries: %#v", entries)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read cache root after mismatch: %v", err)
	}
	result, err := service.PrepareModelAssets(context.Background(), request)
	if err != nil {
		t.Fatalf("retry preparation: %v", err)
	}
	if result.Outcome != models.AssetPreparationPrepared || len(result.Asset.Artifacts) != 1 {
		t.Fatalf("retry result = %#v", result)
	}
}

func TestPrepareGenericAssetsPreservesPriorGoodSnapshotAfterFailedReplacement(t *testing.T) {
	t.Parallel()

	body := []byte("prior good payload")
	digest := sha256Hex(body)
	scopes := newScopes(t, "generic-prior-good")
	you := t.TempDir()
	scope := openScope(t, scopes, you, models.RuntimeConfig{})
	service := newGenericService(
		t, scopes, genericManifestWithoutDigestClient("weights.bin", func() []byte { return body }),
		func(string) string { return "" },
	)
	firstRequest := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo/weights.bin@" + genericTestRevision},
		Artifacts: []models.AssetRequirement{{Name: "weights.bin", SHA256: digest}},
	}
	if _, err := service.PrepareModelAssets(context.Background(), firstRequest); err != nil {
		t.Fatalf("initial preparation: %v", err)
	}
	firstPath := filepath.Join(
		you, assetContentDirectory, assetKindModel,
		genericArtifactIdentityHash(assetKindModel, genericSource{
			kind: genericSourceHF, safe: "hf://owner/repo/weights.bin@" + genericTestRevision,
			owner: "owner", repository: "repo", file: "weights.bin", revision: genericTestRevision,
		}, []genericArtifact{{requirement: models.AssetRequirement{
			Name: "weights.bin", Bytes: int64(len(body)), SHA256: digest,
		}}}),
		"weights.bin",
	)
	assertFileBody(t, firstPath, body)

	failedRequest := firstRequest
	failedRequest.Artifacts = []models.AssetRequirement{{
		Name: "weights.bin", Bytes: int64(len(body) + 1), SHA256: digest,
	}}
	if _, err := service.PrepareModelAssets(context.Background(), failedRequest); !errors.Is(err, models.ErrAssetIntegrityFailed) {
		t.Fatalf("replacement error = %v, want integrity failure", err)
	}
	assertFileBody(t, firstPath, body)
	entries, err := os.ReadDir(filepath.Join(you, assetContentDirectory, assetKindModel))
	if err != nil {
		t.Fatalf("read content-addressed cache: %v", err)
	}
	if len(entries) != 1 || strings.HasSuffix(entries[0].Name(), ".partial") {
		t.Fatalf("replacement cache entries = %#v, want only prior good snapshot", entries)
	}
}

func TestPrepareGenericAssetsRejectsSameSizeCorruptedFileBackedHFCache(t *testing.T) {
	t.Parallel()

	good := []byte("good payload")
	corrupt := []byte("evil payload")
	if len(good) != len(corrupt) {
		t.Fatal("corruption fixture must preserve size")
	}
	hfCache := t.TempDir()
	writeGenericHFFixture(t, hfCache, corrupt)
	var requests atomic.Int32
	manifestClient := genericManifestClient("weights.bin", good, func() []byte {
		requests.Add(1)
		return good
	})
	environment := func(name string) string {
		if name == "HUGGINGFACE_HUB_CACHE" {
			return hfCache
		}
		return ""
	}
	scopes := newScopes(t, "generic-hf-corrupt-same-size")
	scope := openScope(t, scopes, t.TempDir(), models.RuntimeConfig{})
	service := newGenericService(t, scopes, manifestClient, environment)

	result, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope:     scope,
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo/weights.bin@" + genericTestRevision},
	})
	if err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	if result.Outcome != models.AssetPreparationPrepared || len(result.Asset.Artifacts) != 1 ||
		result.Asset.Artifacts[0].SHA256 != sha256Hex(good) || requests.Load() == 0 {
		t.Fatalf("same-size corrupted HF result = %#v, network requests = %d", result, requests.Load())
	}
}

func TestPublishGenericCacheReturnsCommittedAbsolutePaths(t *testing.T) {
	t.Parallel()

	body := []byte("committed backend archive")
	localPath := filepath.Join(t.TempDir(), "backend.zip")
	if err := os.WriteFile(localPath, body, 0o644); err != nil {
		t.Fatalf("write backend fixture: %v", err)
	}
	scopes := newScopes(t, "generic-committed-paths")
	cacheDirectory := t.TempDir()
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("local backend preparation used the network")
		return nil, nil
	}), func(string) string { return "" })
	source := genericSource{kind: genericSourceLocal, safe: "local://path", localPath: localPath}
	artifact := genericArtifact{
		requirement: models.AssetRequirement{
			Name: "nested/backend.zip", Bytes: int64(len(body)), SHA256: sha256Hex(body),
		},
		localPath: localPath,
	}

	result, err := service.publishGenericCache(
		context.Background(), assetKindBackend, models.AssetArtifactKindBackend, source,
		[]genericArtifact{artifact}, nil, []genericArtifact{artifact}, []string{cacheDirectory},
	)
	if err != nil {
		t.Fatalf("publishGenericCache: %v", err)
	}
	path := assertPublishedBackendResult(t, result, body)

	hit, err := service.acquireGenericCache(
		context.Background(), assetKindBackend, models.AssetArtifactKindBackend, source,
		[]genericArtifact{artifact}, []string{cacheDirectory}, false,
	)
	if err != nil {
		t.Fatalf("acquireGenericCache cache hit: %v", err)
	}
	assertGenericCacheHit(t, hit, result.snapshotPath, path)
}

func assertPublishedBackendResult(t *testing.T, result genericCacheResult, body []byte) string {
	t.Helper()
	if !result.prepared || len(result.paths) != 1 {
		t.Fatalf("committed result = %#v, want one prepared path", result)
	}
	assertCommittedSnapshotPath(t, result.snapshotPath)
	path := result.paths[0]
	assertCommittedArtifactPath(t, result.snapshotPath, path, body)
	return path
}

func assertCommittedSnapshotPath(t *testing.T, path string) {
	t.Helper()
	if !filepath.IsAbs(path) || strings.Contains(path, ".partial") {
		t.Fatalf("snapshot path = %q, want absolute committed path", path)
	}
}

func assertCommittedArtifactPath(t *testing.T, snapshotPath, path string, wantBody []byte) {
	t.Helper()
	if !filepath.IsAbs(path) || strings.Contains(path, ".partial") {
		t.Fatalf("artifact path = %q, want absolute non-partial path", path)
	}
	relative, err := filepath.Rel(snapshotPath, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("artifact path %q escaped committed snapshot %q", path, snapshotPath)
	}
	info, err := os.Stat(path)
	if err != nil || info == nil || !info.Mode().IsRegular() {
		t.Fatalf("committed artifact stat = (%#v, %v), want existing regular file", info, err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, wantBody) {
		t.Fatalf("committed artifact body = (%q, %v), want %q", got, err, wantBody)
	}
}

func assertGenericCacheHit(t *testing.T, result genericCacheResult, wantSnapshot, wantPath string) {
	t.Helper()
	if result.prepared || len(result.paths) != 1 || result.snapshotPath != wantSnapshot || result.paths[0] != wantPath {
		t.Fatalf("cache-hit result = %#v, want equivalent committed paths", result)
	}
}

func TestPublishGenericCacheValidationFailurePreservesPriorSnapshot(t *testing.T) {
	t.Parallel()

	oldBody := []byte("prior committed backend")
	newBody := []byte("replacement backend")
	localPath := filepath.Join(t.TempDir(), "backend.zip")
	if err := os.WriteFile(localPath, newBody, 0o644); err != nil {
		t.Fatalf("write replacement fixture: %v", err)
	}
	scopes := newScopes(t, "generic-committed-validation")
	cacheDirectory := t.TempDir()
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("local backend preparation used the network")
		return nil, nil
	}), func(string) string { return "" })
	source := genericSource{kind: genericSourceLocal, safe: "local://path", localPath: localPath}
	artifact := genericArtifact{
		requirement: models.AssetRequirement{Name: "backend.zip", Bytes: int64(len(newBody)), SHA256: sha256Hex(newBody)},
		localPath:   localPath,
	}
	finalPath := filepath.Join(
		cacheDirectory, assetContentDirectory, assetKindBackend,
		genericArtifactIdentityHash(assetKindBackend, source, []genericArtifact{artifact}),
	)
	if err := os.MkdirAll(finalPath, 0o755); err != nil {
		t.Fatalf("create prior snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(finalPath, artifact.requirement.Name), oldBody, 0o644); err != nil {
		t.Fatalf("write prior snapshot: %v", err)
	}
	validationErr := errors.New("committed artifact validation failed")
	originalInspect := service.inspectPath
	service.inspectPath = func(path string) (os.FileInfo, error) {
		if path == filepath.Join(finalPath, artifact.requirement.Name) {
			return nil, validationErr
		}
		return originalInspect(path)
	}

	_, err := service.publishGenericCache(
		context.Background(), assetKindBackend, models.AssetArtifactKindBackend, source,
		[]genericArtifact{artifact}, nil, []genericArtifact{artifact}, []string{cacheDirectory},
	)
	if !errors.Is(err, validationErr) || !errors.Is(err, models.ErrAssetPreparationInterrupted) {
		t.Fatalf("validation error = %v, want typed interruption and validation cause", err)
	}
	var stageErr *models.PullStageError
	if !errors.As(err, &stageErr) || stageErr.Stage != models.PullStageCacheInstallation {
		t.Fatalf("validation stage error = %#v, want cache-installation stage", stageErr)
	}
	if _, statErr := os.Stat(finalPath + ".partial"); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("staging path after validation failure = %v, want absent", statErr)
	}
	assertFileBody(t, filepath.Join(finalPath+".previous", artifact.requirement.Name), oldBody)
}

func TestPublishGenericCacheRestoresPriorSnapshotWhenCommitRenameFails(t *testing.T) {
	t.Parallel()

	oldBody := []byte("prior snapshot")
	newBody := []byte("replacement")
	localPath := filepath.Join(t.TempDir(), "weights.bin")
	if err := os.WriteFile(localPath, newBody, 0o644); err != nil {
		t.Fatalf("write replacement fixture: %v", err)
	}
	scopes := newScopes(t, "generic-rename-rollback")
	you := t.TempDir()
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("local replacement used the network")
		return nil, nil
	}), func(string) string { return "" })
	source := genericSource{kind: genericSourceLocal, safe: "local://path", localPath: localPath}
	artifact := genericArtifact{requirement: models.AssetRequirement{Name: "weights.bin"}, localPath: localPath}
	observed := genericArtifact{requirement: models.AssetRequirement{
		Name: "weights.bin", Bytes: int64(len(newBody)), SHA256: sha256Hex(newBody),
	}}
	finalPath := filepath.Join(
		you, assetContentDirectory, assetKindModel,
		genericArtifactIdentityHash(assetKindModel, source, []genericArtifact{observed}),
	)
	if err := os.MkdirAll(finalPath, 0o755); err != nil {
		t.Fatalf("create prior snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(finalPath, "weights.bin"), oldBody, 0o644); err != nil {
		t.Fatalf("write prior snapshot: %v", err)
	}
	originalRename := service.renamePath
	service.renamePath = func(oldPath, newPath string) error {
		if strings.HasSuffix(oldPath, ".partial") && newPath == finalPath {
			return errors.New("injected snapshot commit rename failure")
		}
		return originalRename(oldPath, newPath)
	}

	_, err := service.publishGenericCache(
		context.Background(), assetKindModel, models.AssetArtifactKindModel, source,
		[]genericArtifact{artifact}, nil, []genericArtifact{artifact}, []string{you},
	)
	if !errors.Is(err, models.ErrAssetPreparationInterrupted) {
		t.Fatalf("publish error = %v, want interrupted rename", err)
	}
	assertFileBody(t, filepath.Join(finalPath, "weights.bin"), oldBody)
	if _, statErr := os.Stat(finalPath + ".previous"); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("rollback backup = %v, want removed after restoration", statErr)
	}
	if _, statErr := os.Stat(finalPath + ".partial"); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("partial snapshot = %v, want removed after rollback", statErr)
	}
}

func TestPrepareGenericAssetsDiskFailureLeavesNoPartialSnapshot(t *testing.T) {
	t.Parallel()

	body := []byte("disk failure payload")
	client := genericManifestClient("weights.bin", body, func() []byte { return body })
	scopes := newScopes(t, "generic-disk-failure")
	you := t.TempDir()
	scope := openScope(t, scopes, you, models.RuntimeConfig{})
	service := newGenericService(t, scopes, client, func(string) string { return "" })
	service.createFile = func(string) (io.WriteCloser, error) {
		return nil, errors.New("disk full")
	}
	_, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope:     scope,
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo/weights.bin@" + genericTestRevision},
		Artifacts: []models.AssetRequirement{{Name: "weights.bin", SHA256: sha256Hex(body), Bytes: int64(len(body))}},
	})
	if !errors.Is(err, models.ErrAssetPreparationInterrupted) {
		t.Fatalf("preparation error = %v, want interruption", err)
	}
	contentRoot := filepath.Join(you, assetContentDirectory, assetKindModel)
	entries, readErr := os.ReadDir(contentRoot)
	if readErr == nil && len(entries) != 0 {
		t.Fatalf("disk failure published cache entries: %#v", entries)
	}
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		t.Fatalf("read cache root after disk failure: %v", readErr)
	}
}

func TestPrepareGenericAssetsCancellationRedactsCause(t *testing.T) {
	t.Parallel()

	body := []byte("cancelled payload")
	secretCause := errors.New("HF_TOKEN=secret cache=C:\\private\\weights")
	ctx, cancel := context.WithCancelCause(context.Background())
	scopes := newScopes(t, "generic-cancel-redaction")
	scope := openScope(t, scopes, t.TempDir(), models.RuntimeConfig{})
	service := newGenericService(t, scopes, genericCancellationClient("weights.bin", cancel, secretCause), func(string) string { return "" })
	_, err := service.PrepareModelAssets(ctx, models.PrepareModelAssetsRequest{
		Scope:     scope,
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo/weights.bin@" + genericTestRevision},
		Artifacts: []models.AssetRequirement{{Name: "weights.bin", SHA256: sha256Hex(body)}},
	})
	if !errors.Is(err, models.ErrAssetCancelled) || !errors.Is(err, secretCause) {
		t.Fatalf("cancellation error = %v, want typed cancellation and cause identity", err)
	}
	if strings.Contains(err.Error(), secretCause.Error()) || strings.Contains(err.Error(), "C:\\private") {
		t.Fatalf("cancellation error leaked sensitive cause: %v", err)
	}
}

func newGenericService(
	t *testing.T,
	scopes runtimescopes.Service,
	client modelseffects.AssetHTTPDoer,
	environment modelseffects.AssetResolveEnvironment,
) *service {
	t.Helper()
	coordination, err := platformlocking.New(platformlocking.LocalFileSystem{})
	if err != nil {
		t.Fatalf("construct asset coordination: %v", err)
	}
	value := New(
		scopes,
		models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		client,
		models.RuntimeAssetEndpoints{BaseURL: "https://assets.example.test", APIBaseURL: "https://api.example.test"},
		os.MkdirAll,
		os.Stat,
		os.UserHomeDir,
		os.WriteFile,
		os.Rename,
		os.Remove,
		os.ReadFile,
		os.ReadDir,
		func(path string) (io.WriteCloser, error) { return os.Create(path) },
		func(path string) (io.ReadCloser, error) { return os.Open(path) },
		assets.ConstructionOptions{
			ResolveEnvironment: environment,
			Coordination:       coordination,
		},
	)
	service, ok := value.(*service)
	if !ok {
		t.Fatalf("New returned %T, want *service", value)
	}
	return service
}

func genericManifestClient(
	name string,
	body []byte,
	nextBody func() []byte,
) modelseffects.AssetHTTPDoer {
	manifest, _ := json.Marshal(map[string]any{
		"sha": genericTestRevision,
		"siblings": []map[string]any{{
			"rfilename": name,
			"size":      len(body),
			"lfs": map[string]any{
				"oid": sha256Hex(body), "size": len(body),
			},
		}},
	})
	return httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		if strings.HasPrefix(request.URL.Path, "/models/") {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(manifest))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(nextBody()))}, nil
	})
}

func genericManifestWithoutDigestClient(name string, nextBody func() []byte) modelseffects.AssetHTTPDoer {
	manifest, _ := json.Marshal(map[string]any{
		"sha":      genericTestRevision,
		"siblings": []map[string]any{{"rfilename": name}},
	})
	return httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		if strings.HasPrefix(request.URL.Path, "/models/") {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(manifest))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(nextBody()))}, nil
	})
}

func genericCancellationClient(
	name string,
	cancel context.CancelCauseFunc,
	cause error,
) modelseffects.AssetHTTPDoer {
	manifest, _ := json.Marshal(map[string]any{
		"sha":      genericTestRevision,
		"siblings": []map[string]any{{"rfilename": name}},
	})
	return httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		if strings.HasPrefix(request.URL.Path, "/models/") {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(manifest))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: &cancellingReadCloser{cancel: cancel, cause: cause}}, nil
	})
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func TestPrepareGenericAssetsCommitsObservedMetadataAndReusesItOffline(t *testing.T) {
	t.Parallel()

	body := []byte("observed generic cache payload")
	digest := sha256Hex(body)
	var downloads atomic.Int32
	scopes := newScopes(t, "generic-observed-metadata")
	cacheDirectory := t.TempDir()
	scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newGenericService(
		t, scopes,
		genericManifestWithoutDigestClient("weights.bin", func() []byte {
			downloads.Add(1)
			return body
		}),
		func(string) string { return "" },
	)
	request := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo/weights.bin@" + genericTestRevision},
		Artifacts: []models.AssetRequirement{{Name: "weights.bin"}},
	}

	first, err := service.PrepareModelAssets(context.Background(), request)
	requireObservedMetadataPreparation(t, first, err, body, digest)
	if downloads.Load() != 1 {
		t.Fatalf("asset downloads = %d, want one initial transfer", downloads.Load())
	}

	source := observedMetadataSource(request.Reference.NameOrURI)
	observed := []genericArtifact{{requirement: models.AssetRequirement{
		Name: "weights.bin", Bytes: int64(len(body)), SHA256: digest,
	}}}
	assertObservedMetadataCommitted(t, cacheDirectory, source, request, observed)

	service.client = httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline reuse must not contact the source")
	})
	second, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: scope, Reference: request.Reference, Offline: true,
	})
	requireObservedMetadataReuse(t, second, err, body, digest)
	if downloads.Load() != 1 {
		t.Fatalf("offline asset downloads = %d, want unchanged", downloads.Load())
	}
}

func observedMetadataSource(reference string) genericSource {
	return genericSource{
		kind: genericSourceHF, safe: reference,
		owner: "owner", repository: "repo", file: "weights.bin", revision: genericTestRevision,
	}
}

func requireObservedMetadataPreparation(
	t *testing.T,
	result models.PrepareModelAssetsResult,
	err error,
	body []byte,
	digest string,
) {
	t.Helper()
	if err != nil {
		t.Fatalf("initial preparation: %v", err)
	}
	if result.Outcome != models.AssetPreparationPrepared || result.Asset.TotalBytes != int64(len(body)) ||
		len(result.Asset.Artifacts) != 1 || result.Asset.Artifacts[0].Bytes != int64(len(body)) ||
		result.Asset.Artifacts[0].SHA256 != digest {
		t.Fatalf("initial result = %#v, want observed bytes and digest", result)
	}
}

func assertObservedMetadataCommitted(
	t *testing.T,
	cacheDirectory string,
	source genericSource,
	request models.PrepareModelAssetsRequest,
	observed []genericArtifact,
) {
	t.Helper()
	observedIdentity := genericArtifactIdentityHash(assetKindModel, source, observed)
	inputIdentity := genericArtifactIdentityHash(assetKindModel, source, requestArtifacts(request))
	if observedIdentity == inputIdentity {
		t.Fatalf("observed identity = input identity %q; fixture must exercise unresolved metadata", observedIdentity)
	}
	metadataPath := filepath.Join(cacheDirectory, assetContentDirectory, assetKindModel, observedIdentity, assetMetadataName)
	metadataBody, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read committed metadata: %v", err)
	}
	var metadata genericCacheMetadata
	if err := json.Unmarshal(metadataBody, &metadata); err != nil {
		t.Fatalf("decode committed metadata: %v", err)
	}
	wantIdentity := genericCacheKey(assetKindModel, source, observed)
	if metadata.Identity != wantIdentity || len(metadata.Artifacts) != 1 || metadata.Artifacts[0] != observed[0].requirement {
		t.Fatalf("committed metadata = %#v, want observed identity/requirements", metadata)
	}
	if _, err := os.Stat(filepath.Join(cacheDirectory, assetContentDirectory, assetKindModel, inputIdentity)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unresolved identity snapshot = %v, want absent", err)
	}
}

func requireObservedMetadataReuse(
	t *testing.T,
	result models.PrepareModelAssetsResult,
	err error,
	body []byte,
	digest string,
) {
	t.Helper()
	if err != nil {
		t.Fatalf("offline preparation: %v", err)
	}
	if result.Outcome != models.AssetPreparationAlreadyAvailable || result.Asset.Integrity != models.AssetIntegrityVerified ||
		result.Asset.TotalBytes != int64(len(body)) || len(result.Asset.Artifacts) != 1 ||
		result.Asset.Artifacts[0].Bytes != int64(len(body)) || result.Asset.Artifacts[0].SHA256 != digest {
		t.Fatalf("offline result = %#v, want the committed observed identity", result)
	}
}

func requestArtifacts(request models.PrepareModelAssetsRequest) []genericArtifact {
	artifacts := make([]genericArtifact, 0, len(request.Artifacts))
	for _, requirement := range request.Artifacts {
		artifacts = append(artifacts, genericArtifact{requirement: requirement})
	}
	return artifacts
}
