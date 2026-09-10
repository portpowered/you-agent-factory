package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

func TestPrepareModelAssetsRejectsPreCancelledRequestBeforeEffects(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "prepare-pre-cancelled")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	var sourceRequests atomic.Int32
	var mutations atomic.Int32
	service := newPreparationTestService(
		scopes,
		httpDoerFunc(func(*http.Request) (*http.Response, error) {
			sourceRequests.Add(1)
			return nil, errors.New("unexpected source request")
		}),
		models.RuntimeAssetEndpoints{
			BaseURL: "https://assets.example.test", APIBaseURL: "https://api.example.test",
		},
		&mutations,
	)
	cancelCause := errors.New("operator stopped asset preparation")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cancelCause)

	_, err := service.PrepareModelAssets(ctx, models.PrepareModelAssetsRequest{
		Scope: ref, Name: "OMNIVOICE_Q4_K_M",
	})
	if !errors.Is(err, models.ErrAssetCancelled) ||
		!errors.Is(err, context.Canceled) ||
		!errors.Is(err, cancelCause) {
		t.Fatalf("pre-cancelled error = %v, want typed cancellation and underlying causes", err)
	}
	if sourceRequests.Load() != 0 || mutations.Load() != 0 {
		t.Fatalf(
			"pre-cancelled preparation performed source=%d mutation=%d effects",
			sourceRequests.Load(),
			mutations.Load(),
		)
	}
}

func TestPrepareModelAssetsCancelsInFlightCleansUpAndRetries(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "prepare-in-flight-cancelled")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	baseBody := []byte("complete base")
	tokenizerBody := []byte("complete tokenizer")
	cancelCause := errors.New("runtime scope stopped")
	ctx, cancel := context.WithCancelCause(context.Background())
	service := newPreparationTestService(
		scopes,
		newAssetTestClient("cancelled-revision", baseBody, tokenizerBody, func() io.ReadCloser {
			return &cancellingReadCloser{cancel: cancel, cause: cancelCause}
		}),
		models.RuntimeAssetEndpoints{
			BaseURL: "https://assets.example.test", APIBaseURL: "https://api.example.test",
		},
		nil,
	)
	request := models.PrepareModelAssetsRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"}

	result, err := service.PrepareModelAssets(ctx, request)
	if !errors.Is(err, models.ErrAssetCancelled) ||
		!errors.Is(err, context.Canceled) ||
		!errors.Is(err, cancelCause) {
		t.Fatalf("in-flight cancellation error = %v, want typed cancellation and causes", err)
	}
	if result.Asset.Readiness != models.AssetReadinessFailed {
		t.Fatalf("cancelled result = %#v, want failed readiness", result)
	}
	assertAttemptAbsent(t, cacheDirectory, "cancelled-revision")

	service.client = newAssetTestClient("cancelled-revision", baseBody, tokenizerBody, nil)
	retried, err := service.PrepareModelAssets(context.Background(), request)
	if err != nil {
		t.Fatalf("retry after cancellation: %v", err)
	}
	if retried.Outcome != models.AssetPreparationPrepared ||
		retried.Asset.Integrity != models.AssetIntegrityVerified {
		t.Fatalf("retry result = %#v, want verified preparation", retried)
	}
}

func TestPrepareModelAssetsInterruptedReadCleansUpAndRetries(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "prepare-interrupted-read")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	baseBody := []byte("complete base")
	tokenizerBody := []byte("complete tokenizer")
	service := newPreparationTestService(
		scopes,
		newAssetTestClient("interrupted-revision", baseBody, tokenizerBody, func() io.ReadCloser {
			return &failingReadCloser{cause: io.ErrUnexpectedEOF}
		}),
		models.RuntimeAssetEndpoints{
			BaseURL: "https://assets.example.test", APIBaseURL: "https://api.example.test",
		},
		nil,
	)
	request := models.PrepareModelAssetsRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"}

	_, err := service.PrepareModelAssets(context.Background(), request)
	if !errors.Is(err, models.ErrAssetPreparationInterrupted) ||
		!errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("interrupted read error = %v, want interruption and read cause", err)
	}
	assertAttemptAbsent(t, cacheDirectory, "interrupted-revision")

	service.client = newAssetTestClient("interrupted-revision", baseBody, tokenizerBody, nil)
	if _, err := service.PrepareModelAssets(context.Background(), request); err != nil {
		t.Fatalf("retry after interrupted read: %v", err)
	}
}

func TestPrepareModelAssetsSurfacesCleanupFailureWithPrimaryFailure(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "prepare-cleanup-failure")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	primaryFailure := errors.New("response stream failed")
	cleanupFailure := errors.New("staged file removal failed")
	service := newPreparationTestService(
		scopes,
		newAssetTestClient("cleanup-failure", []byte("base"), []byte("tokenizer"), func() io.ReadCloser {
			return &failingReadCloser{cause: primaryFailure}
		}),
		models.RuntimeAssetEndpoints{
			BaseURL: "https://assets.example.test", APIBaseURL: "https://api.example.test",
		},
		nil,
	)
	removePath := service.removePath
	service.removePath = func(path string) error {
		if filepath.Base(path) == "omnivoice-base-Q4_K_M.gguf" {
			return cleanupFailure
		}
		return removePath(path)
	}

	_, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: ref, Name: "OMNIVOICE_Q4_K_M",
	})
	if !errors.Is(err, models.ErrAssetPreparationInterrupted) ||
		!errors.Is(err, primaryFailure) ||
		!errors.Is(err, cleanupFailure) {
		t.Fatalf("cleanup error = %v, want primary classification, primary cause, and cleanup cause", err)
	}
}

func TestPrepareModelAssetsRenameFailurePreservesExistingRevisionAndRetries(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	existingPath := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M", "existing-revision", "marker")
	if err := os.MkdirAll(filepath.Dir(existingPath), 0o755); err != nil {
		t.Fatalf("create existing revision: %v", err)
	}
	if err := os.WriteFile(existingPath, []byte("preserve me"), 0o644); err != nil {
		t.Fatalf("write existing revision: %v", err)
	}
	scopes := newScopes(t, "prepare-rename-failure")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	baseBody := []byte("base")
	tokenizerBody := []byte("tokenizer")
	service := newPreparationTestService(
		scopes,
		newAssetTestClient("new-revision", baseBody, tokenizerBody, nil),
		models.RuntimeAssetEndpoints{
			BaseURL: "https://assets.example.test", APIBaseURL: "https://api.example.test",
		},
		nil,
	)
	renameFailure := errors.New("cache rename interrupted")
	renamePath := service.renamePath
	service.renamePath = func(_, _ string) error { return renameFailure }
	request := models.PrepareModelAssetsRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"}

	_, err := service.PrepareModelAssets(context.Background(), request)
	if !errors.Is(err, models.ErrAssetPreparationInterrupted) ||
		!errors.Is(err, renameFailure) {
		t.Fatalf("rename error = %v, want typed interruption and rename cause", err)
	}
	var stageErr *models.PullStageError
	if !errors.As(err, &stageErr) || stageErr.Stage != models.PullStageCacheInstallation {
		t.Fatalf("rename stage error = %#v, want cache-installation stage", stageErr)
	}
	assertFileBody(t, existingPath, []byte("preserve me"))
	assertAttemptAbsent(t, cacheDirectory, "new-revision")

	service.renamePath = renamePath
	if _, err := service.PrepareModelAssets(context.Background(), request); err != nil {
		t.Fatalf("retry after rename interruption: %v", err)
	}
	assertFileBody(t, existingPath, []byte("preserve me"))
}

func newAssetTestClient(
	revision string,
	baseBody []byte,
	tokenizerBody []byte,
	baseOverride func() io.ReadCloser,
) modelseffects.AssetHTTPDoer {
	manifest, _ := json.Marshal(map[string]any{
		"sha": revision,
		"siblings": []map[string]any{
			{
				"rfilename": "omnivoice-base-Q4_K_M.gguf",
				"lfs": map[string]any{
					"oid": sha256String(baseBody), "size": len(baseBody),
				},
			},
			{
				"rfilename": "omnivoice-tokenizer-Q4_K_M.gguf",
				"lfs": map[string]any{
					"oid": sha256String(tokenizerBody), "size": len(tokenizerBody),
				},
			},
		},
	})
	return httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		var body io.ReadCloser
		switch {
		case strings.HasPrefix(request.URL.Path, "/models/"):
			body = io.NopCloser(bytes.NewReader(manifest))
		case strings.Contains(request.URL.Path, "omnivoice-base"):
			if baseOverride != nil {
				body = baseOverride()
			} else {
				body = io.NopCloser(bytes.NewReader(baseBody))
			}
		default:
			body = io.NopCloser(bytes.NewReader(tokenizerBody))
		}
		return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
	})
}

func assertAttemptAbsent(t *testing.T, cacheDirectory, revision string) {
	t.Helper()
	root := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	for _, path := range []string{
		filepath.Join(root, revision),
		filepath.Join(root, revision+".partial"),
		filepath.Join(root, metadataFileName),
		filepath.Join(root, metadataFileName+".partial"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("failed attempt left %q: %v", path, err)
		}
	}
}

type cancellingReadCloser struct {
	cancel context.CancelCauseFunc
	cause  error
}

func (reader *cancellingReadCloser) Read([]byte) (int, error) {
	reader.cancel(reader.cause)
	return 0, reader.cause
}

func (*cancellingReadCloser) Close() error { return nil }

type failingReadCloser struct {
	cause error
}

func (reader *failingReadCloser) Read([]byte) (int, error) { return 0, reader.cause }
func (*failingReadCloser) Close() error                    { return nil }

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
