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
	"time"

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
		}, []genericArtifact{{requirement: firstRequest.Artifacts[0]}}),
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

func TestPrepareGenericAssetsSharesConcurrentFirstDownload(t *testing.T) {
	t.Parallel()

	body := []byte("concurrent payload")
	digest := sha256Hex(body)
	entered := make(chan struct{})
	release := make(chan struct{})
	var downloads atomic.Int32
	client := genericManifestClient("weights.bin", body, func() []byte {
		downloads.Add(1)
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		return body
	})
	scopes := newScopes(t, "generic-singleflight")
	scope := openScope(t, scopes, t.TempDir(), models.RuntimeConfig{})
	service := newGenericService(t, scopes, client, func(string) string { return "" })
	request := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo/weights.bin@" + genericTestRevision},
		Artifacts: []models.AssetRequirement{{Name: "weights.bin", SHA256: digest, Bytes: int64(len(body))}},
	}
	results := make(chan error, 8)
	for index := 0; index < 8; index++ {
		go func() {
			_, err := service.PrepareModelAssets(context.Background(), request)
			results <- err
		}()
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent download did not start")
	}
	close(release)
	for index := 0; index < 8; index++ {
		if err := <-results; err != nil {
			t.Fatalf("concurrent preparation %d: %v", index, err)
		}
	}
	if got := downloads.Load(); got != 1 {
		t.Fatalf("download count = %d, want 1", got)
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
		assets.ConstructionOptions{ResolveEnvironment: environment},
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
