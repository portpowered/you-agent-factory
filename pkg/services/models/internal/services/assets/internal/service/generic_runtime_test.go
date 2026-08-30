package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	assets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
)

func TestPrepareGenericAssetsPublishesDurableRuntimeCacheAcrossServiceReconstruction(t *testing.T) {
	t.Parallel()

	cacheDirectory, scope, service, localPath, body := newNamedGenericRuntimeFixture(t, "durable-runtime")
	request := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      "joined-model",
		Reference: models.ModelReference{NameOrURI: localPath},
		Artifacts: []models.AssetRequirement{{
			Name: filepath.Base(localPath), Bytes: int64(len(body)), SHA256: sha256Hex(body),
		}},
	}
	if _, err := service.PrepareModelAssets(context.Background(), request); err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}

	first := inspectNamedGenericRuntime(t, service, scope, "joined-model")
	assertNamedGenericRuntimeReady(t, first)
	assertGenericRuntimeMetadataExists(t, cacheDirectory, "joined-model")

	secondScopes := newScopes(t, "durable-runtime-reconstructed")
	secondScope := openScope(t, secondScopes, cacheDirectory, models.RuntimeConfig{})
	second := newGenericService(t, secondScopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("reconstructed runtime inspection must not use network")
	}), func(string) string { return "" })
	secondInspection := inspectNamedGenericRuntime(t, second, secondScope, "joined-model")
	assertNamedGenericRuntimeReady(t, secondInspection)
	if secondInspection.CachePath != first.CachePath || secondInspection.Revision != first.Revision {
		t.Fatalf("reconstructed inspection = %#v, first inspection = %#v", secondInspection, first)
	}

	layout, err := second.ResolveRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: secondScope,
		Name:  "joined-model",
	})
	if err != nil {
		t.Fatalf("ResolveRuntimeCache after reconstruction: %v", err)
	}
	if layout.CachePath != secondInspection.CachePath || len(layout.Files) != 1 {
		t.Fatalf("reconstructed runtime layout = %#v, inspection = %#v", layout, secondInspection)
	}
}

func TestPrepareGenericAssetsReusesVerifiedManagedRuntimeBeforeRemotePreflight(t *testing.T) {
	t.Parallel()

	scope, service, modelName, revision, fileName, body := newVerifiedManagedRuntimeFixture(t)
	request := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      modelName,
		Reference: models.ModelReference{NameOrURI: "hf://owner/embed@" + revision},
	}
	preflight, err := service.PreflightModelAssets(context.Background(), request)
	if err != nil {
		t.Fatalf("PreflightModelAssets: %v", err)
	}
	assertVerifiedManagedRuntimePreflight(t, preflight)
	result, err := service.PrepareModelAssets(context.Background(), request)
	if err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	assertVerifiedManagedRuntimeResult(t, result, fileName, body)

	_, err = service.PreflightModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      modelName,
		Reference: models.ModelReference{NameOrURI: "hf://owner/embed@" + strings.Repeat("b", 40)},
		Offline:   true,
	})
	if !errors.Is(err, models.ErrAssetOffline) {
		t.Fatalf("different revision preflight error = %v, want offline cache miss", err)
	}
}

func newVerifiedManagedRuntimeFixture(t *testing.T) (
	models.RuntimeScopeRef, *service, string, string, string, []byte,
) {
	t.Helper()
	cacheDirectory := t.TempDir()
	modelName := "embed"
	revision := strings.Repeat("a", 40)
	fileName := "model.safetensors"
	body := []byte("verified managed embedding model")
	modelRoot := filepath.Join(cacheDirectory, canonicalModelName(modelName))
	revisionPath := filepath.Join(modelRoot, revision)
	if err := os.MkdirAll(revisionPath, 0o755); err != nil {
		t.Fatalf("create managed runtime: %v", err)
	}
	if err := os.WriteFile(filepath.Join(revisionPath, fileName), body, 0o644); err != nil {
		t.Fatalf("write managed runtime artifact: %v", err)
	}
	metadata, err := json.Marshal(cacheMetadata{
		ModelName: modelName,
		Revision:  revision,
		Files: []metadataFile{{
			Path: fileName, Bytes: int64(len(body)), SHA256: sha256Hex(body),
		}},
	})
	if err != nil {
		t.Fatalf("marshal managed runtime metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelRoot, metadataFileName), metadata, 0o644); err != nil {
		t.Fatalf("write managed runtime metadata: %v", err)
	}
	scopes := newScopes(t, "generic-managed-cache-reuse")
	scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("verified managed runtime preflight used the network")
		return nil, nil
	}), func(string) string { return "" })
	return scope, service, modelName, revision, fileName, body
}

func assertVerifiedManagedRuntimePreflight(
	t *testing.T,
	preflight models.PreflightModelAssetsResult,
) {
	t.Helper()
	if preflight.ModelDownloadRequired || preflight.BackendDownloadRequired || preflight.TotalBytes != 0 {
		t.Fatalf("preflight = %#v, want no download for verified managed runtime", preflight)
	}
}

func assertVerifiedManagedRuntimeResult(
	t *testing.T,
	result models.PrepareModelAssetsResult,
	fileName string,
	body []byte,
) {
	t.Helper()
	if result.Outcome != models.AssetPreparationAlreadyAvailable || len(result.Asset.Artifacts) != 1 ||
		result.Asset.Artifacts[0].Name != fileName || result.Asset.Artifacts[0].Bytes != int64(len(body)) ||
		result.Asset.Artifacts[0].SHA256 != sha256Hex(body) {
		t.Fatalf("prepared asset = %#v, want verified managed artifact", result.Asset)
	}
}

func TestPrepareGenericAssetsReplacesNamedRuntimeCacheAtomically(t *testing.T) {
	t.Parallel()

	cacheDirectory, scope, service, localPath, firstBody := newNamedGenericRuntimeFixture(t, "atomic-runtime-replacement")
	request := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      "replace-model",
		Reference: models.ModelReference{NameOrURI: localPath},
		Artifacts: []models.AssetRequirement{{
			Name: filepath.Base(localPath), Bytes: int64(len(firstBody)), SHA256: sha256Hex(firstBody),
		}},
	}
	if _, err := service.PrepareModelAssets(context.Background(), request); err != nil {
		t.Fatalf("initial PrepareModelAssets: %v", err)
	}

	secondBody := []byte("replacement runtime weights")
	if err := os.WriteFile(localPath, secondBody, 0o644); err != nil {
		t.Fatalf("write replacement model artifact: %v", err)
	}
	request.Artifacts[0].Bytes = int64(len(secondBody))
	request.Artifacts[0].SHA256 = sha256Hex(secondBody)
	if _, err := service.PrepareModelAssets(context.Background(), request); err != nil {
		t.Fatalf("replacement PrepareModelAssets: %v", err)
	}

	inspection := inspectNamedGenericRuntime(t, service, scope, request.Name)
	assertNamedGenericRuntimeReady(t, inspection)
	assertFileBody(t, filepath.Join(inspection.CachePath, filepath.Base(localPath)), secondBody)
	if _, err := os.Stat(filepath.Join(
		cacheDirectory, canonicalModelName(request.Name), metadataFileName+".previous",
	)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime metadata backup = %v, want removed after replacement", err)
	}
}

func TestInspectRuntimeCacheReportsInvalidGenericManifest(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	modelDirectory := filepath.Join(cacheDirectory, "GENERIC-MODEL")
	if err := os.MkdirAll(modelDirectory, 0o755); err != nil {
		t.Fatalf("create generic model cache directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(modelDirectory, metadataFileName),
		[]byte(`{"revision":`),
		0o644,
	); err != nil {
		t.Fatalf("write malformed generic cache metadata: %v", err)
	}

	scopes := newScopes(t, "generic-invalid-manifest")
	scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid generic manifest inspection used the network")
		return nil, nil
	}), func(string) string { return "" })
	inspection, err := service.InspectRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  "generic-model",
	})
	if err != nil {
		t.Fatalf("InspectRuntimeCache: %v", err)
	}
	if !inspection.Supported || !inspection.ManifestPresent || inspection.ManifestValid ||
		inspection.Installed || inspection.FailureReason != "managed cache manifest is invalid" {
		t.Fatalf("invalid generic manifest inspection = %#v", inspection)
	}
}

func TestInspectRuntimeCacheRejectsIncompleteGenericManifest(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	modelDirectory := filepath.Join(cacheDirectory, "GENERIC-MODEL")
	if err := os.MkdirAll(modelDirectory, 0o755); err != nil {
		t.Fatalf("create generic model cache directory: %v", err)
	}
	metadata, err := json.Marshal(cacheMetadata{
		ModelName: "generic-model",
		Revision:  "revision-1",
		Files: []metadataFile{
			{Path: "weights.bin"},
			{Path: "weights.bin"},
		},
	})
	if err != nil {
		t.Fatalf("marshal incomplete generic metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelDirectory, metadataFileName), metadata, 0o644); err != nil {
		t.Fatalf("write incomplete generic metadata: %v", err)
	}

	scopes := newScopes(t, "generic-incomplete-manifest")
	scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("incomplete generic manifest inspection used the network")
		return nil, nil
	}), func(string) string { return "" })
	inspection, err := service.InspectRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  "generic-model",
	})
	if err != nil {
		t.Fatalf("InspectRuntimeCache: %v", err)
	}
	if !inspection.Supported || !inspection.ManifestPresent || inspection.ManifestValid ||
		inspection.Installed || inspection.FailureReason != "managed cache manifest is invalid" {
		t.Fatalf("incomplete generic manifest inspection = %#v", inspection)
	}
}

func TestInspectRuntimeCacheRejectsPinnedGenericCacheForDifferentSource(t *testing.T) {
	t.Parallel()

	scope, service := newStalePinnedGenericRuntimeFixture(t)
	inspection, err := service.InspectRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  "embed",
	})
	if err != nil {
		t.Fatalf("InspectRuntimeCache: %v", err)
	}
	assertStalePinnedGenericRuntimeInspection(t, inspection)
	assertStalePinnedGenericRuntimeUnavailable(t, service, scope)
}

func newStalePinnedGenericRuntimeFixture(t *testing.T) (models.RuntimeScopeRef, *service) {
	t.Helper()
	cacheDirectory := t.TempDir()
	modelDirectory := filepath.Join(cacheDirectory, canonicalModelName("embed"))
	staleRevision := strings.Repeat("a", 40)
	revisionDirectory := filepath.Join(modelDirectory, staleRevision)
	staleBody := []byte("stale sentence-transformer model")
	if err := os.MkdirAll(revisionDirectory, 0o755); err != nil {
		t.Fatalf("create stale generic model cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(revisionDirectory, "model.safetensors"), staleBody, 0o644); err != nil {
		t.Fatalf("write stale generic model artifact: %v", err)
	}
	metadata := stalePinnedGenericRuntimeMetadata(t, staleRevision, staleBody)
	if err := os.WriteFile(filepath.Join(modelDirectory, metadataFileName), metadata, 0o644); err != nil {
		t.Fatalf("write stale generic metadata: %v", err)
	}

	scopes := newScopes(t, "generic-source-mismatch")
	scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("source-mismatched generic inspection used the network")
		return nil, nil
	}), func(string) string { return "" })
	return scope, service
}

func stalePinnedGenericRuntimeMetadata(t *testing.T, revision string, body []byte) []byte {
	t.Helper()
	metadata, err := json.Marshal(cacheMetadata{
		ModelName: "embed",
		Revision:  revision,
		Files: []metadataFile{{
			Path: "model.safetensors", Bytes: int64(len(body)), SHA256: sha256Hex(body),
		}},
	})
	if err != nil {
		t.Fatalf("marshal stale generic metadata: %v", err)
	}
	return metadata
}

func assertStalePinnedGenericRuntimeInspection(t *testing.T, inspection assets.RuntimeCacheInspection) {
	t.Helper()
	if !inspection.Supported || !inspection.ManifestPresent || !inspection.ManifestValid ||
		inspection.Installed || inspection.IntegrityVerified ||
		inspection.FailureReason != "managed cache does not match configured source" {
		t.Fatalf("source-mismatched inspection = %#v, want unavailable cache", inspection)
	}
	if len(inspection.ExpectedArtifacts) != 1 ||
		inspection.ExpectedArtifacts[0].Name != "Qwen3-Embedding-0.6B-f16.gguf" ||
		len(inspection.MissingAssets) != 1 ||
		inspection.MissingAssets[0] != "Qwen3-Embedding-0.6B-f16.gguf" {
		t.Fatalf("source-mismatched artifact facts = %#v, want pinned GGUF missing", inspection)
	}
}

func assertStalePinnedGenericRuntimeUnavailable(
	t *testing.T,
	service *service,
	scope models.RuntimeScopeRef,
) {
	t.Helper()
	if _, err := service.ResolveRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  "embed",
	}); !errors.Is(err, models.ErrNotAvailable) {
		t.Fatalf("ResolveRuntimeCache error = %v, want stale cache rejected", err)
	}
}

func TestInspectRuntimeCacheReportsMissingGenericArtifact(t *testing.T) {
	t.Parallel()

	cacheDirectory, scope, service, localPath, body := newNamedGenericRuntimeFixture(t, "generic-missing-artifact")
	request := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      "missing-artifact-model",
		Reference: models.ModelReference{NameOrURI: localPath},
		Artifacts: []models.AssetRequirement{{
			Name: "weights.gguf", Bytes: int64(len(body)), SHA256: sha256Hex(body),
		}},
	}
	if _, err := service.PrepareModelAssets(context.Background(), request); err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	ready := inspectNamedGenericRuntime(t, service, scope, request.Name)
	if ready.CachePath == "" {
		t.Fatalf("ready runtime inspection = %#v, want cache path", ready)
	}
	if err := os.Remove(filepath.Join(ready.CachePath, "weights.gguf")); err != nil {
		t.Fatalf("remove prepared generic artifact: %v", err)
	}

	inspection := inspectNamedGenericRuntime(t, service, scope, request.Name)
	if !inspection.Supported || !inspection.ManifestPresent || !inspection.ManifestValid ||
		inspection.Installed || len(inspection.MissingAssets) != 1 ||
		inspection.MissingAssets[0] != "weights.gguf" {
		t.Fatalf("missing generic artifact inspection = %#v", inspection)
	}
	if _, err := os.Stat(filepath.Join(cacheDirectory, canonicalModelName(request.Name), metadataFileName)); err != nil {
		t.Fatalf("generic runtime metadata after missing artifact: %v", err)
	}
}

func TestResolveRuntimeCacheReportsUnavailableForUnknownGenericModel(t *testing.T) {
	t.Parallel()

	scopes := newScopes(t, "generic-runtime-missing")
	scope := openScope(t, scopes, t.TempDir(), models.RuntimeConfig{})
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("unknown generic runtime resolution used the network")
		return nil, nil
	}), func(string) string { return "" })

	_, err := service.ResolveRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  "unknown-model",
	})
	if !errors.Is(err, models.ErrNotAvailable) {
		t.Fatalf("ResolveRuntimeCache error = %v, want ErrNotAvailable", err)
	}
}

func TestPrepareGenericAssetsDoesNotPublishManagedRuntimeAfterRuntimeCommitFailure(t *testing.T) {
	t.Parallel()

	cacheDirectory, scope, service, localPath, body := newNamedGenericRuntimeFixture(t, "atomic-runtime")
	originalRename := service.renamePath
	service.renamePath = func(oldPath, newPath string) error {
		if strings.HasSuffix(newPath, metadataFileName) {
			return errors.New("injected managed runtime metadata commit failure")
		}
		return originalRename(oldPath, newPath)
	}
	_, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      "atomic-model",
		Reference: models.ModelReference{NameOrURI: localPath},
		Artifacts: []models.AssetRequirement{{
			Name: filepath.Base(localPath), Bytes: int64(len(body)), SHA256: sha256Hex(body),
		}},
	})
	if !errors.Is(err, models.ErrAssetPreparationInterrupted) {
		t.Fatalf("PrepareModelAssets error = %v, want interrupted preparation", err)
	}

	assertNoGenericRuntimePublication(t, cacheDirectory, "atomic-model")
	inspection := inspectNamedGenericRuntime(t, service, scope, "atomic-model")
	if inspection.Installed || inspection.ManifestPresent {
		t.Fatalf("failed runtime publication inspection = %#v, want no installed cache", inspection)
	}
}

func TestPrepareGenericAssetsReplacesExistingManagedRuntimeMetadataAtomically(t *testing.T) {
	t.Parallel()

	cacheDirectory, scope, service, localPath, body := newNamedGenericRuntimeFixture(t, "replace-runtime")
	request := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      "replace-model",
		Reference: models.ModelReference{NameOrURI: localPath},
		Artifacts: []models.AssetRequirement{{
			Name: filepath.Base(localPath), Bytes: int64(len(body)), SHA256: sha256Hex(body),
		}},
	}
	if _, err := service.PrepareModelAssets(context.Background(), request); err != nil {
		t.Fatalf("initial PrepareModelAssets: %v", err)
	}

	updatedBody := []byte("replacement runtime weights")
	if err := os.WriteFile(localPath, updatedBody, 0o644); err != nil {
		t.Fatalf("write replacement source: %v", err)
	}
	request.Artifacts[0].Bytes = int64(len(updatedBody))
	request.Artifacts[0].SHA256 = sha256Hex(updatedBody)
	if _, err := service.PrepareModelAssets(context.Background(), request); err != nil {
		t.Fatalf("replacement PrepareModelAssets: %v", err)
	}

	inspection := inspectNamedGenericRuntime(t, service, scope, "replace-model")
	if !inspection.Installed || !inspection.ManifestPresent || !inspection.ManifestValid ||
		!inspection.IntegrityVerified || inspection.CacheBytes != int64(len(updatedBody)) {
		t.Fatalf("replacement runtime inspection = %#v, want verified replacement", inspection)
	}
	root := filepath.Join(cacheDirectory, canonicalModelName("replace-model"))
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read replacement runtime root: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".partial") || strings.HasSuffix(entry.Name(), ".previous") {
			t.Fatalf("replacement publication left transient entry %q", entry.Name())
		}
	}
}

func TestMoveExistingGenericMetadataClassifiesReplacementFailures(t *testing.T) {
	t.Parallel()

	t.Run("stat failure", func(t *testing.T) {
		_, _, service, _, _ := newNamedGenericRuntimeFixture(t, "metadata-stat-failure")
		statErr := errors.New("metadata stat failed")
		service.inspectPath = func(string) (os.FileInfo, error) { return nil, statErr }
		if _, _, err := service.moveExistingGenericMetadata(filepath.Join(t.TempDir(), metadataFileName)); !errors.Is(err, statErr) {
			t.Fatalf("moveExistingGenericMetadata error = %v, want %v", err, statErr)
		}
	})

	t.Run("directory destination", func(t *testing.T) {
		_, _, service, _, _ := newNamedGenericRuntimeFixture(t, "metadata-directory")
		path := filepath.Join(t.TempDir(), metadataFileName)
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatalf("create metadata directory: %v", err)
		}
		if _, _, err := service.moveExistingGenericMetadata(path); err == nil {
			t.Fatal("moveExistingGenericMetadata succeeded for directory metadata")
		}
	})

	t.Run("backup removal failure", func(t *testing.T) {
		_, _, service, _, _ := newNamedGenericRuntimeFixture(t, "metadata-backup-failure")
		path := filepath.Join(t.TempDir(), metadataFileName)
		if err := os.WriteFile(path, []byte("metadata"), 0o644); err != nil {
			t.Fatalf("write metadata: %v", err)
		}
		removeErr := errors.New("metadata backup removal failed")
		service.removePath = func(string) error { return removeErr }
		if _, _, err := service.moveExistingGenericMetadata(path); !errors.Is(err, removeErr) {
			t.Fatalf("moveExistingGenericMetadata error = %v, want %v", err, removeErr)
		}
	})

	t.Run("backup rename failure", func(t *testing.T) {
		_, _, service, _, _ := newNamedGenericRuntimeFixture(t, "metadata-rename-failure")
		path := filepath.Join(t.TempDir(), metadataFileName)
		if err := os.WriteFile(path, []byte("metadata"), 0o644); err != nil {
			t.Fatalf("write metadata: %v", err)
		}
		renameErr := errors.New("metadata backup rename failed")
		service.renamePath = func(string, string) error { return renameErr }
		if _, _, err := service.moveExistingGenericMetadata(path); !errors.Is(err, renameErr) {
			t.Fatalf("moveExistingGenericMetadata error = %v, want %v", err, renameErr)
		}
	})
}

func TestGenericRuntimeMetadataValidationRejectsInvalidAndDuplicateArtifacts(t *testing.T) {
	t.Parallel()

	if _, err := genericRuntimeMetadataFiles([]models.AssetArtifact{{Bytes: 1}}); !errors.Is(err, models.ErrAssetPreparationInterrupted) {
		t.Fatalf("invalid generic metadata error = %v, want interrupted preparation", err)
	}
	if _, err := genericRuntimeMetadataFiles([]models.AssetArtifact{
		{Name: "weights.bin", Bytes: 1},
		{Name: "weights.bin", Bytes: 1},
	}); !errors.Is(err, models.ErrAssetPreparationInterrupted) {
		t.Fatalf("duplicate generic metadata error = %v, want interrupted preparation", err)
	}
	if got := genericRuntimeRevision(genericSource{revision: "pinned-revision"}, nil); got != "pinned-revision" {
		t.Fatalf("genericRuntimeRevision() = %q, want pinned-revision", got)
	}
}

func newNamedGenericRuntimeFixture(
	t *testing.T,
	issuer string,
) (string, models.RuntimeScopeRef, *service, string, []byte) {
	t.Helper()
	cacheDirectory := t.TempDir()
	localPath := filepath.Join(t.TempDir(), "weights.gguf")
	body := []byte("durable runtime weights")
	if err := os.WriteFile(localPath, body, 0o644); err != nil {
		t.Fatalf("write local fixture: %v", err)
	}
	scopes := newScopes(t, issuer)
	scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("local generic preparation must not use network")
	}), func(string) string { return "" })
	return cacheDirectory, scope, service, localPath, body
}

func inspectNamedGenericRuntime(
	t *testing.T,
	service *service,
	scope models.RuntimeScopeRef,
	modelName string,
) assets.RuntimeCacheInspection {
	t.Helper()
	inspection, err := service.InspectRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  modelName,
	})
	if err != nil {
		t.Fatalf("InspectRuntimeCache: %v", err)
	}
	return inspection
}

func assertNamedGenericRuntimeReady(t *testing.T, inspection assets.RuntimeCacheInspection) {
	t.Helper()
	if !inspection.Supported || !inspection.Installed || !inspection.ManifestPresent ||
		!inspection.ManifestValid || inspection.CachePath == "" || inspection.InstalledFileCount != 1 {
		t.Fatalf("runtime inspection = %#v, want durable ready cache", inspection)
	}
	if _, err := os.Stat(filepath.Join(inspection.CachePath, "weights.gguf")); err != nil {
		t.Fatalf("runtime artifact in %q: %v", inspection.CachePath, err)
	}
}

func assertGenericRuntimeMetadataExists(t *testing.T, cacheDirectory, modelName string) {
	t.Helper()
	path := filepath.Join(cacheDirectory, canonicalModelName(modelName), metadataFileName)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("managed runtime metadata %q: %v", path, err)
	}
}

func assertNoGenericRuntimePublication(t *testing.T, cacheDirectory, modelName string) {
	t.Helper()
	root := filepath.Join(cacheDirectory, canonicalModelName(modelName))
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatalf("read managed runtime root %q: %v", root, err)
	}
	for _, entry := range entries {
		if entry.Name() == metadataFileName || strings.HasSuffix(entry.Name(), ".partial") ||
			strings.HasSuffix(entry.Name(), ".previous") {
			t.Fatalf("failed publication left %q in %q", entry.Name(), root)
		}
	}
}
