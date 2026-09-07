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
	if err != nil {
		t.Fatalf("initial preparation: %v", err)
	}
	if first.Outcome != models.AssetPreparationPrepared || first.Asset.TotalBytes != int64(len(body)) ||
		len(first.Asset.Artifacts) != 1 || first.Asset.Artifacts[0].Bytes != int64(len(body)) ||
		first.Asset.Artifacts[0].SHA256 != digest {
		t.Fatalf("initial result = %#v, want observed bytes and digest", first)
	}
	if downloads.Load() != 1 {
		t.Fatalf("asset downloads = %d, want one initial transfer", downloads.Load())
	}

	source := genericSource{
		kind: genericSourceHF, safe: "hf://owner/repo/weights.bin@" + genericTestRevision,
		owner: "owner", repository: "repo", file: "weights.bin", revision: genericTestRevision,
	}
	observed := []genericArtifact{{requirement: models.AssetRequirement{
		Name: "weights.bin", Bytes: int64(len(body)), SHA256: digest,
	}}}
	observedIdentity := genericArtifactIdentityHash(assetKindModel, source, observed)
	inputIdentity := genericArtifactIdentityHash(assetKindModel, source, requestArtifacts(request))
	if observedIdentity == inputIdentity {
		t.Fatalf("observed identity = input identity %q; fixture must exercise unresolved metadata", observedIdentity)
	}
	metadataPath := filepath.Join(
		cacheDirectory, assetContentDirectory, assetKindModel, observedIdentity, assetMetadataName,
	)
	metadataBody, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read committed metadata: %v", err)
	}
	var metadata genericCacheMetadata
	if err := json.Unmarshal(metadataBody, &metadata); err != nil {
		t.Fatalf("decode committed metadata: %v", err)
	}
	wantIdentity := genericCacheKey(assetKindModel, source, observed)
	if metadata.Identity != wantIdentity || len(metadata.Artifacts) != 1 ||
		metadata.Artifacts[0] != observed[0].requirement {
		t.Fatalf("committed metadata = %#v, want observed identity/requirements", metadata)
	}
	if _, err := os.Stat(filepath.Join(
		cacheDirectory, assetContentDirectory, assetKindModel, inputIdentity,
	)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unresolved identity snapshot = %v, want absent", err)
	}

	service.client = httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline reuse must not contact the source")
	})
	second, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: scope, Reference: request.Reference, Offline: true,
	})
	if err != nil {
		t.Fatalf("offline preparation: %v", err)
	}
	if second.Outcome != models.AssetPreparationAlreadyAvailable ||
		second.Asset.Integrity != models.AssetIntegrityVerified || second.Asset.TotalBytes != int64(len(body)) ||
		len(second.Asset.Artifacts) != 1 || second.Asset.Artifacts[0].Bytes != int64(len(body)) ||
		second.Asset.Artifacts[0].SHA256 != digest {
		t.Fatalf("offline result = %#v, want the committed observed identity", second)
	}
	if downloads.Load() != 1 {
		t.Fatalf("offline asset downloads = %d, want unchanged", downloads.Load())
	}
}

func TestPrepareGenericAssetsRepairsLegacyMetadataWithoutTransferOrDeletion(t *testing.T) {
	t.Parallel()

	body := []byte("legacy generic cache payload")
	source := genericSource{
		kind: genericSourceHF, safe: "hf://owner/repo/weights.bin@" + genericTestRevision,
		owner: "owner", repository: "repo", file: "weights.bin", revision: genericTestRevision,
	}
	cacheDirectory := t.TempDir()
	legacyPath, correctedPath := writeLegacyGenericCacheFixture(t, cacheDirectory, source, body)
	scopes := newScopes(t, "generic-legacy-repair")
	scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	var sourceRequests atomic.Int32
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		sourceRequests.Add(1)
		return nil, errors.New("legacy repair must not contact the source")
	}), func(string) string { return "" })
	request := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Reference: models.ModelReference{NameOrURI: source.safe},
		Artifacts: []models.AssetRequirement{{
			Name: "weights.bin", Bytes: int64(len(body)), SHA256: sha256Hex(body),
		}},
	}

	result, err := service.PrepareModelAssets(context.Background(), request)
	if err != nil {
		t.Fatalf("legacy repair: %v", err)
	}
	if result.Outcome != models.AssetPreparationAlreadyAvailable ||
		result.Asset.Integrity != models.AssetIntegrityVerified || len(result.Asset.Artifacts) != 1 ||
		result.Asset.Artifacts[0].Bytes != int64(len(body)) || result.Asset.Artifacts[0].SHA256 != sha256Hex(body) {
		t.Fatalf("legacy repair result = %#v, want verified cache hit", result)
	}
	if sourceRequests.Load() != 0 {
		t.Fatalf("legacy repair source requests = %d, want zero", sourceRequests.Load())
	}
	assertFileBody(t, filepath.Join(correctedPath, "weights.bin"), body)
	if _, err := os.Stat(legacyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy snapshot after repair = %v, want moved to corrected identity", err)
	}
	metadataBody, err := os.ReadFile(filepath.Join(correctedPath, assetMetadataName))
	if err != nil {
		t.Fatalf("read repaired metadata: %v", err)
	}
	var metadata genericCacheMetadata
	if err := json.Unmarshal(metadataBody, &metadata); err != nil {
		t.Fatalf("decode repaired metadata: %v", err)
	}
	if metadata.Identity != genericCacheKey(assetKindModel, source, []genericArtifact{{requirement: request.Artifacts[0]}}) ||
		len(metadata.Artifacts) != 1 || metadata.Artifacts[0] != request.Artifacts[0] {
		t.Fatalf("repaired metadata = %#v, want observed requirement and identity", metadata)
	}
	for _, suffix := range []string{".partial", ".previous"} {
		if _, err := os.Stat(correctedPath + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("repair transient %q = %v, want absent", suffix, err)
		}
	}

	service.client = httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline repaired reuse must not contact the source")
	})
	reused, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: scope, Reference: request.Reference, Offline: true,
	})
	if err != nil || reused.Outcome != models.AssetPreparationAlreadyAvailable ||
		reused.Asset.Integrity != models.AssetIntegrityVerified || reused.Asset.TotalBytes != int64(len(body)) {
		t.Fatalf("offline repaired reuse = %#v, %v", reused, err)
	}
}

func TestPrepareGenericAssetsRepairsLegacyMetadataAfterManifestResolution(t *testing.T) {
	t.Parallel()

	body := []byte("manifest-backed legacy payload")
	source := genericSource{
		kind: genericSourceHF, safe: "hf://owner/repo/weights.bin@" + genericTestRevision,
		owner: "owner", repository: "repo", file: "weights.bin", revision: genericTestRevision,
	}
	cacheDirectory := t.TempDir()
	legacyPath, correctedPath := writeLegacyGenericCacheFixture(t, cacheDirectory, source, body)
	manifest, err := json.Marshal(map[string]any{
		"sha": genericTestRevision,
		"siblings": []map[string]any{{
			"rfilename": "weights.bin",
			"size":      len(body),
			"lfs":       map[string]any{"oid": sha256Hex(body), "size": len(body)},
		}},
	})
	if err != nil {
		t.Fatalf("encode legacy repair manifest: %v", err)
	}
	var manifestRequests, artifactRequests atomic.Int32
	scopes := newScopes(t, "generic-legacy-repair-manifest")
	scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newGenericService(t, scopes, httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		if strings.HasPrefix(request.URL.Path, "/models/") {
			manifestRequests.Add(1)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(manifest))}, nil
		}
		artifactRequests.Add(1)
		return nil, errors.New("manifest-resolved legacy repair must not transfer the artifact")
	}), func(string) string { return "" })

	result, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: scope, Reference: models.ModelReference{NameOrURI: source.safe},
	})
	if err != nil {
		t.Fatalf("manifest-backed legacy repair: %v", err)
	}
	if result.Asset.Integrity != models.AssetIntegrityVerified || manifestRequests.Load() == 0 ||
		artifactRequests.Load() != 0 {
		t.Fatalf("manifest-backed repair result = %#v, manifest requests = %d, artifact requests = %d",
			result, manifestRequests.Load(), artifactRequests.Load())
	}
	assertFileBody(t, filepath.Join(correctedPath, "weights.bin"), body)
	if _, err := os.Stat(legacyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("manifest-backed legacy snapshot = %v, want moved to corrected identity", err)
	}
}

func TestPrepareGenericAssetsLeavesCorruptOrAmbiguousLegacyMetadataUnresolved(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*genericCacheMetadata)
		body   []byte
	}{
		{
			name: "corrupt bytes",
			body: []byte("corrupt legacy payload"),
		},
		{
			name: "missing source identity",
			body: []byte("valid legacy payload"),
			mutate: func(metadata *genericCacheMetadata) {
				metadata.SourceKey = ""
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			source := genericSource{
				kind: genericSourceHF, safe: "hf://owner/repo/weights.bin@" + genericTestRevision,
				owner: "owner", repository: "repo", file: "weights.bin", revision: genericTestRevision,
			}
			cacheDirectory := t.TempDir()
			legacyPath, correctedPath := writeLegacyGenericCacheFixture(t, cacheDirectory, source, []byte("valid legacy payload"))
			metadataPath := filepath.Join(legacyPath, assetMetadataName)
			if testCase.mutate != nil {
				body, err := os.ReadFile(metadataPath)
				if err != nil {
					t.Fatalf("read legacy metadata: %v", err)
				}
				var metadata genericCacheMetadata
				if err := json.Unmarshal(body, &metadata); err != nil {
					t.Fatalf("decode legacy metadata: %v", err)
				}
				testCase.mutate(&metadata)
				body, err = json.Marshal(metadata)
				if err != nil {
					t.Fatalf("encode legacy metadata: %v", err)
				}
				if err := os.WriteFile(metadataPath, body, 0o644); err != nil {
					t.Fatalf("rewrite legacy metadata: %v", err)
				}
			}
			if testCase.name == "corrupt bytes" {
				if err := os.WriteFile(filepath.Join(legacyPath, "weights.bin"), testCase.body, 0o644); err != nil {
					t.Fatalf("write corrupt legacy bytes: %v", err)
				}
			}
			scopes := newScopes(t, "generic-legacy-unresolved-"+testCase.name)
			scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
			var sourceRequests atomic.Int32
			service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
				sourceRequests.Add(1)
				return nil, errors.New("unresolved legacy cache must stay offline")
			}), func(string) string { return "" })
			_, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
				Scope: scope, Reference: models.ModelReference{NameOrURI: source.safe}, Offline: true,
				Artifacts: []models.AssetRequirement{{
					Name: "weights.bin", Bytes: int64(len("valid legacy payload")), SHA256: sha256Hex([]byte("valid legacy payload")),
				}},
			})
			if !errors.Is(err, models.ErrAssetOffline) {
				t.Fatalf("unresolved legacy error = %v, want typed offline error", err)
			}
			if sourceRequests.Load() != 0 {
				t.Fatalf("unresolved legacy source requests = %d, want zero", sourceRequests.Load())
			}
			if _, err := os.Stat(legacyPath); err != nil {
				t.Fatalf("unresolved legacy snapshot: %v", err)
			}
			if _, err := os.Stat(correctedPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unresolved corrected snapshot = %v, want absent", err)
			}
		})
	}
}

func TestPrepareGenericAssetsLeavesConflictingLegacyFactsUnresolved(t *testing.T) {
	t.Parallel()

	body := []byte("conflicting legacy payload")
	source := genericSource{
		kind: genericSourceHF, safe: "hf://owner/repo/weights.bin@" + genericTestRevision,
		owner: "owner", repository: "repo", file: "weights.bin", revision: genericTestRevision,
	}
	cacheDirectory := t.TempDir()
	legacyPath, correctedPath := writeLegacyGenericCacheFixture(t, cacheDirectory, source, body)
	metadataPath := filepath.Join(legacyPath, assetMetadataName)
	metadataBody, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read conflicting legacy metadata: %v", err)
	}
	var metadata genericCacheMetadata
	if err := json.Unmarshal(metadataBody, &metadata); err != nil {
		t.Fatalf("decode conflicting legacy metadata: %v", err)
	}
	wrongDigest := sha256Hex([]byte("different payload"))
	metadata.Artifacts[0].SHA256 = wrongDigest
	metadata.Identity = genericCacheKey(assetKindModel, source, []genericArtifact{{
		requirement: metadata.Artifacts[0],
	}})
	conflictingPath := filepath.Join(
		cacheDirectory, assetContentDirectory, assetKindModel,
		genericArtifactIdentityHash(assetKindModel, source, []genericArtifact{{
			requirement: metadata.Artifacts[0],
		}}),
	)
	if err := os.Rename(legacyPath, conflictingPath); err != nil {
		t.Fatalf("move conflicting legacy snapshot: %v", err)
	}
	metadataBody, err = json.Marshal(metadata)
	if err != nil {
		t.Fatalf("encode conflicting legacy metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(conflictingPath, assetMetadataName), metadataBody, 0o644); err != nil {
		t.Fatalf("write conflicting legacy metadata: %v", err)
	}

	scopes := newScopes(t, "generic-legacy-conflicting-facts")
	scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("conflicting legacy facts used the source")
		return nil, nil
	}), func(string) string { return "" })
	_, err = service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: scope, Reference: models.ModelReference{NameOrURI: source.safe}, Offline: true,
		Artifacts: []models.AssetRequirement{{
			Name: "weights.bin", Bytes: int64(len(body)), SHA256: sha256Hex(body),
		}},
	})
	if !errors.Is(err, models.ErrAssetIntegrityFailed) {
		t.Fatalf("conflicting legacy facts error = %v, want typed integrity error", err)
	}
	if _, err := os.Stat(conflictingPath); err != nil {
		t.Fatalf("conflicting legacy snapshot: %v", err)
	}
	if _, err := os.Stat(correctedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("conflicting corrected snapshot = %v, want absent", err)
	}
}

func TestPrepareGenericAssetsLegacyRepairFailuresPreserveRecovery(t *testing.T) {
	t.Parallel()

	for _, failure := range []string{"metadata write", "snapshot rename"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()

			body := []byte("recoverable legacy payload")
			source := genericSource{
				kind: genericSourceHF, safe: "hf://owner/repo/weights.bin@" + genericTestRevision,
				owner: "owner", repository: "repo", file: "weights.bin", revision: genericTestRevision,
			}
			cacheDirectory := t.TempDir()
			legacyPath, correctedPath := writeLegacyGenericCacheFixture(t, cacheDirectory, source, body)
			scopes := newScopes(t, "generic-legacy-repair-"+failure)
			scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
			service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("legacy repair failure must not contact the source")
			}), func(string) string { return "" })
			request := models.PrepareModelAssetsRequest{
				Scope: scope, Reference: models.ModelReference{NameOrURI: source.safe}, Offline: true,
				Artifacts: []models.AssetRequirement{{
					Name: "weights.bin", Bytes: int64(len(body)), SHA256: sha256Hex(body),
				}},
			}
			failureCause := errors.New(failure)
			if failure == "metadata write" {
				originalWrite := service.writeFile
				service.writeFile = func(path string, data []byte, mode os.FileMode) error {
					if strings.HasSuffix(path, assetMetadataName+".partial") {
						return failureCause
					}
					return originalWrite(path, data, mode)
				}
			} else {
				originalRename := service.renamePath
				service.renamePath = func(oldPath, newPath string) error {
					if oldPath == legacyPath && newPath == correctedPath {
						return failureCause
					}
					return originalRename(oldPath, newPath)
				}
			}

			_, err := service.PrepareModelAssets(context.Background(), request)
			if !errors.Is(err, models.ErrAssetPreparationInterrupted) || !errors.Is(err, failureCause) {
				t.Fatalf("legacy %s error = %v, want typed interruption and cause", failure, err)
			}
			assertFileBody(t, filepath.Join(legacyPath, "weights.bin"), body)
			if _, err := os.Stat(correctedPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed corrected snapshot = %v, want absent", err)
			}
			for _, path := range []string{
				filepath.Join(legacyPath, assetMetadataName+".partial"),
				filepath.Join(legacyPath, assetMetadataName+".previous"),
			} {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("legacy %s transient %q = %v, want absent", failure, path, err)
				}
			}

			// Construct a fresh service so the retry proves recovery through the
			// public preparation boundary rather than a test-only mutation.
			retryService := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("legacy repair retry must not contact the source")
			}), func(string) string { return "" })
			retried, err := retryService.PrepareModelAssets(context.Background(), request)
			if err != nil || retried.Outcome != models.AssetPreparationAlreadyAvailable ||
				retried.Asset.Integrity != models.AssetIntegrityVerified {
				t.Fatalf("legacy %s retry = %#v, %v", failure, retried, err)
			}
		})
	}
}

func TestPrepareGenericAssetsLegacyRepairCancellationRollsBack(t *testing.T) {
	t.Parallel()

	body := []byte("cancellable legacy payload")
	source := genericSource{
		kind: genericSourceHF, safe: "hf://owner/repo/weights.bin@" + genericTestRevision,
		owner: "owner", repository: "repo", file: "weights.bin", revision: genericTestRevision,
	}
	cacheDirectory := t.TempDir()
	legacyPath, correctedPath := writeLegacyGenericCacheFixture(t, cacheDirectory, source, body)
	scopes := newScopes(t, "generic-legacy-repair-cancellation")
	scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("cancelled legacy repair must not contact the source")
	}), func(string) string { return "" })
	cancelCause := errors.New("operator stopped legacy repair")
	ctx, cancel := context.WithCancelCause(context.Background())
	originalRename := service.renamePath
	service.renamePath = func(oldPath, newPath string) error {
		if oldPath == legacyPath && newPath == correctedPath {
			cancel(cancelCause)
		}
		return originalRename(oldPath, newPath)
	}
	request := models.PrepareModelAssetsRequest{
		Scope: scope, Reference: models.ModelReference{NameOrURI: source.safe}, Offline: true,
		Artifacts: []models.AssetRequirement{{
			Name: "weights.bin", Bytes: int64(len(body)), SHA256: sha256Hex(body),
		}},
	}

	_, err := service.PrepareModelAssets(ctx, request)
	if !errors.Is(err, models.ErrAssetCancelled) || !errors.Is(err, context.Canceled) ||
		!errors.Is(err, cancelCause) {
		t.Fatalf("legacy repair cancellation = %v, want typed cancellation and cause", err)
	}
	assertFileBody(t, filepath.Join(legacyPath, "weights.bin"), body)
	if _, err := os.Stat(correctedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled corrected snapshot = %v, want absent", err)
	}
	for _, path := range []string{
		filepath.Join(legacyPath, assetMetadataName+".partial"),
		filepath.Join(legacyPath, assetMetadataName+".previous"),
	} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("cancelled repair transient %q = %v, want absent", path, err)
		}
	}

	retryService := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("legacy repair retry must not contact the source")
	}), func(string) string { return "" })
	retried, err := retryService.PrepareModelAssets(context.Background(), request)
	if err != nil || retried.Outcome != models.AssetPreparationAlreadyAvailable ||
		retried.Asset.Integrity != models.AssetIntegrityVerified {
		t.Fatalf("legacy repair cancellation retry = %#v, %v", retried, err)
	}
}

func writeLegacyGenericCacheFixture(
	t *testing.T,
	cacheDirectory string,
	source genericSource,
	body []byte,
) (string, string) {
	t.Helper()
	stale := []genericArtifact{{requirement: models.AssetRequirement{Name: "weights.bin"}}}
	observed := []genericArtifact{{requirement: models.AssetRequirement{
		Name: "weights.bin", Bytes: int64(len(body)), SHA256: sha256Hex(body),
	}}}
	base := filepath.Join(cacheDirectory, assetContentDirectory, assetKindModel)
	legacyPath := filepath.Join(base, genericArtifactIdentityHash(assetKindModel, source, stale))
	correctedPath := filepath.Join(base, genericArtifactIdentityHash(assetKindModel, source, observed))
	if err := os.MkdirAll(legacyPath, 0o755); err != nil {
		t.Fatalf("create legacy generic snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyPath, "weights.bin"), body, 0o644); err != nil {
		t.Fatalf("write legacy generic artifact: %v", err)
	}
	metadata := genericCacheMetadata{
		Kind: assetKindModel, Identity: genericCacheKey(assetKindModel, source, stale),
		Source: source.safe, SourceKey: genericSourceIdentity(source), Artifacts: []models.AssetRequirement{{Name: "weights.bin"}},
	}
	metadataBody, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("encode legacy generic metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyPath, assetMetadataName), metadataBody, 0o644); err != nil {
		t.Fatalf("write legacy generic metadata: %v", err)
	}
	return legacyPath, correctedPath
}

func requestArtifacts(request models.PrepareModelAssetsRequest) []genericArtifact {
	artifacts := make([]genericArtifact, 0, len(request.Artifacts))
	for _, requirement := range request.Artifacts {
		artifacts = append(artifacts, genericArtifact{requirement: requirement})
	}
	return artifacts
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

func TestGenericPrivatePlanningAndCachePreservation(t *testing.T) {
	t.Parallel()

	scopes := newScopes(t, "generic-private-planning")
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("private planning test must not use HTTP")
	}), func(string) string { return "" })

	assertGenericLocalRequirements(t, service)
	assertGenericOverlayPlanning(t)
	assertGenericCachePreservation(t, service)
}

func assertGenericLocalRequirements(t *testing.T, service *service) {
	t.Helper()

	localRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(localRoot, "nested"), 0o755); err != nil {
		t.Fatalf("create local fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localRoot, "root.bin"), []byte("root"), 0o644); err != nil {
		t.Fatalf("write root fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localRoot, "nested", "child.bin"), []byte("child"), 0o644); err != nil {
		t.Fatalf("write nested fixture: %v", err)
	}
	requirements, err := service.localRequirements(localRoot)
	if err != nil || len(requirements) != 2 || requirements[0].Name != "nested/child.bin" || requirements[1].Name != "root.bin" {
		t.Fatalf("localRequirements = %#v, %v, want sorted recursive files", requirements, err)
	}
	if _, err := service.localRequirements(filepath.Join(localRoot, "missing")); !errors.Is(err, models.ErrAssetSourceMissing) {
		t.Fatalf("missing localRequirements error = %v, want ErrAssetSourceMissing", err)
	}
}

func assertGenericOverlayPlanning(t *testing.T) {
	t.Helper()

	sourceText := "hf://owner/repo@" + genericTestRevision
	overlays := map[string]models.ModelOverlay{
		"Alias":  {Source: &sourceText},
		"alias ": {Source: func() *string { value := "hf://owner/other@" + genericTestRevision; return &value }()},
	}
	name, overlay, ok := genericOverlay(overlays, "alias")
	if !ok || name != "Alias" || overlay.Source == nil {
		t.Fatalf("genericOverlay = %q, %#v, %v", name, overlay, ok)
	}
	*overlay.Source = "mutated"
	if *overlays["Alias"].Source != sourceText {
		t.Fatal("genericOverlay returned a mutable source pointer")
	}
	safe := genericHFSafeReference(genericSource{owner: "owner", repository: "repo", file: "weights.bin", revision: genericTestRevision})
	if safe != "hf://owner/repo/weights.bin@"+genericTestRevision {
		t.Fatalf("genericHFSafeReference = %q", safe)
	}
	var revisionFailure *models.InvocationFailure
	if !errors.As(genericRevisionFailure(), &revisionFailure) || revisionFailure.Class != models.InvocationFailureClassRevisionResolution {
		t.Fatalf("genericRevisionFailure = %#v", genericRevisionFailure())
	}
}

func assertGenericCachePreservation(t *testing.T, service *service) {
	t.Helper()

	cachedPath := filepath.Join(t.TempDir(), "cached.bin")
	if err := os.WriteFile(cachedPath, []byte("cached"), 0o644); err != nil {
		t.Fatalf("write cached fixture: %v", err)
	}
	stagePath := t.TempDir()
	artifact := genericCachePath{
		artifact: models.AssetArtifact{Name: "nested/cached.bin", Bytes: int64(len("cached"))},
		path:     cachedPath,
	}
	if err := service.preserveGenericArtifact(context.Background(), artifact, artifact.artifact.Name, stagePath); err != nil {
		t.Fatalf("preserveGenericArtifact: %v", err)
	}
	preserved, err := os.ReadFile(filepath.Join(stagePath, "nested", "cached.bin"))
	if err != nil || string(preserved) != "cached" {
		t.Fatalf("preserved cached artifact = %q, %v", preserved, err)
	}
}

func TestGenericSnapshotDiscoveryAndSourcePlanning(t *testing.T) {
	t.Parallel()

	scopes := newScopes(t, "generic-snapshot-planning")
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("source planning test must not use HTTP")
	}), func(string) string { return "" })
	assertGenericSnapshotDiscovery(t, service)
	assertGenericCacheRoots(t, service)
	assertGenericSourceResolution(t, service)
	assertGenericPreparationSelection(t)
}

func assertGenericSnapshotDiscovery(t *testing.T, service *service) {
	t.Helper()
	snapshotRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(snapshotRoot, "nested"), 0o755); err != nil {
		t.Fatalf("create snapshot fixture: %v", err)
	}
	for name, body := range map[string]string{
		"root.bin":            "root",
		"nested/child.bin":    "child",
		".hidden/ignored.bin": "ignored",
	} {
		path := filepath.Join(snapshotRoot, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create snapshot parent: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write snapshot file: %v", err)
		}
	}
	requirements := service.discoverSnapshotRequirements(snapshotRoot)
	if len(requirements) != 2 || requirements[0].Name != "nested/child.bin" || requirements[1].Name != "root.bin" {
		t.Fatalf("discoverSnapshotRequirements = %#v, want visible sorted files", requirements)
	}
	if got := service.discoverSnapshotRequirements(filepath.Join(snapshotRoot, "missing")); got != nil {
		t.Fatalf("missing snapshot requirements = %#v, want nil", got)
	}
}

func assertGenericCacheRoots(t *testing.T, service *service) {
	t.Helper()
	scopeConfig := models.RuntimeScopeConfig{CacheDirectory: t.TempDir()}
	modelRoots, err := service.genericCacheRoots(scopeConfig, models.AssetArtifactKindModel)
	if err != nil || len(modelRoots) == 0 || modelRoots[len(modelRoots)-1] != scopeConfig.CacheDirectory {
		t.Fatalf("model genericCacheRoots = %#v, %v", modelRoots, err)
	}
	backendRoots, err := service.genericCacheRoots(scopeConfig, models.AssetArtifactKindBackend)
	if err != nil || len(backendRoots) != 1 || !strings.HasSuffix(filepath.ToSlash(backendRoots[0]), "/backend-artifacts") {
		t.Fatalf("backend genericCacheRoots = %#v, %v", backendRoots, err)
	}
}

func assertGenericSourceResolution(t *testing.T, service *service) {
	t.Helper()
	scopeConfig := models.RuntimeScopeConfig{CacheDirectory: t.TempDir()}
	service.resolveRevision = func(context.Context, string) (string, error) { return genericTestRevision, nil }
	resolved, err := service.resolveGenericSource(context.Background(), scopeConfig, "hf://owner/repo")
	if err != nil || resolved.revision != genericTestRevision || resolved.safe != "hf://owner/repo@"+genericTestRevision {
		t.Fatalf("resolved generic source = %#v, %v", resolved, err)
	}
	service.resolveRevision = func(context.Context, string) (string, error) { return "not-a-commit", nil }
	if _, err := service.resolveGenericSource(context.Background(), scopeConfig, "hf://owner/repo"); !errors.Is(err, models.ErrModelRevisionUnresolved) {
		t.Fatalf("unresolved revision error = %v, want ErrModelRevisionUnresolved", err)
	}
	if _, err := parseGenericReleaseSource("https://example.com/releases/download/v1/backend.tar"); !errors.Is(err, models.ErrModelReferenceInvalid) {
		t.Fatalf("invalid release source error = %v, want ErrModelReferenceInvalid", err)
	}
	release, err := parseGenericReleaseSource("https://github.com/owner/repo/releases/download/v1/backend.tar")
	if err != nil || release.kind != genericSourceRelease || release.artifactURL == "" {
		t.Fatalf("valid release source = %#v, %v", release, err)
	}
}

func assertGenericPreparationSelection(t *testing.T) {
	t.Helper()
	for _, request := range []models.PrepareModelAssetsRequest{
		{Reference: models.ModelReference{NameOrURI: "hf://owner/repo@" + genericTestRevision}},
		{Offline: true},
		{Artifacts: []models.AssetRequirement{}},
		{Backend: "backend-v1"},
		{Name: "llm"},
		{Name: "./local/model"},
	} {
		if !shouldPrepareGenericAssets(request) {
			t.Fatalf("shouldPrepareGenericAssets(%#v) = false, want true", request)
		}
	}
	if shouldPrepareGenericAssets(models.PrepareModelAssetsRequest{Name: "unknown-symbol"}) {
		t.Fatal("unknown symbolic name should not select generic preparation")
	}
}
