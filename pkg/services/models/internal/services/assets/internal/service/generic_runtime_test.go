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

func TestPrepareGenericAssetsPersistsBackendRuntimeFactsAcrossServiceReconstruction(t *testing.T) {
	t.Parallel()

	cacheDirectory, scope, service, localPath, body := newNamedGenericRuntimeFixture(t, "durable-runtime-backend")
	request := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      "joined-model",
		Reference: models.ModelReference{NameOrURI: localPath},
		Artifacts: []models.AssetRequirement{{
			Name: filepath.Base(localPath), Bytes: int64(len(body)), SHA256: sha256Hex(body),
		}},
		Backend: "localai-vibevoice",
		BackendArtifacts: []models.AssetRequirement{{
			Name: "backend.zip", Bytes: int64(len(body)), SHA256: sha256Hex(body),
		}},
	}
	if _, err := service.PrepareModelAssets(context.Background(), request); err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}

	first := inspectNamedGenericRuntime(t, service, scope, request.Name)
	if !first.BackendRequired || len(first.BackendFiles) != 1 || first.BackendCachePath == "" {
		t.Fatalf("first runtime inspection = %#v, want one backend file", first)
	}
	assertCommittedBackendPath(t, first, body)

	secondScopes := newScopes(t, "durable-runtime-backend-reconstructed")
	secondScope := openScope(t, secondScopes, cacheDirectory, models.RuntimeConfig{})
	second := newGenericService(t, secondScopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("reconstructed backend runtime inspection must not use network")
	}), func(string) string { return "" })
	secondInspection := inspectNamedGenericRuntime(t, second, secondScope, request.Name)
	if !secondInspection.BackendRequired || len(secondInspection.BackendFiles) != 1 ||
		secondInspection.BackendCachePath != first.BackendCachePath {
		t.Fatalf("reconstructed backend inspection = %#v, first inspection = %#v", secondInspection, first)
	}
	assertCommittedBackendPath(t, secondInspection, body)

	layout, err := second.ResolveRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: secondScope,
		Name:  request.Name,
	})
	if err != nil {
		t.Fatalf("ResolveRuntimeCache after backend reconstruction: %v", err)
	}
	if layout.BackendCachePath != secondInspection.BackendCachePath || len(layout.BackendFiles) != 1 {
		t.Fatalf("reconstructed backend runtime layout = %#v, inspection = %#v", layout, secondInspection)
	}
}

func TestPrepareGenericAssetsAddsBackendToExistingManagedModelAcrossReconstruction(t *testing.T) {
	t.Parallel()

	cacheDirectory, scope, service, localPath, modelBody := newNamedGenericRuntimeFixture(t, "durable-runtime-backend-upgrade")
	modelRequest := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      "joined-model",
		Reference: models.ModelReference{NameOrURI: localPath},
		Artifacts: []models.AssetRequirement{{
			Name: filepath.Base(localPath), Bytes: int64(len(modelBody)), SHA256: sha256Hex(modelBody),
		}},
	}
	if _, err := service.PrepareModelAssets(context.Background(), modelRequest); err != nil {
		t.Fatalf("initial model PrepareModelAssets: %v", err)
	}
	initial := inspectNamedGenericRuntime(t, service, scope, modelRequest.Name)
	if initial.BackendRequired {
		t.Fatalf("initial runtime inspection = %#v, want model-only durable manifest", initial)
	}

	backendBody := []byte("new durable backend archive")
	backendPath := filepath.Join(t.TempDir(), "backend.zip")
	if err := os.WriteFile(backendPath, backendBody, 0o644); err != nil {
		t.Fatalf("write backend fixture: %v", err)
	}
	backendRequest := modelRequest
	backendRequest.Backend = "localai-vibevoice"
	backendRequest.BackendReference = models.ModelReference{NameOrURI: backendPath}
	backendRequest.BackendArtifacts = []models.AssetRequirement{{
		Name: filepath.Base(backendPath), Bytes: int64(len(backendBody)), SHA256: sha256Hex(backendBody),
	}}
	if _, err := service.PrepareModelAssets(context.Background(), backendRequest); err != nil {
		t.Fatalf("backend upgrade PrepareModelAssets: %v", err)
	}
	upgraded := inspectNamedGenericRuntime(t, service, scope, backendRequest.Name)
	if !upgraded.BackendRequired || len(upgraded.BackendFiles) != 1 || upgraded.BackendCachePath == "" {
		t.Fatalf("upgraded runtime inspection = %#v, want one durable backend file", upgraded)
	}
	assertCommittedBackendPath(t, upgraded, backendBody)

	reconstructedScopes := newScopes(t, "durable-runtime-backend-upgrade-reconstructed")
	reconstructedScope := openScope(t, reconstructedScopes, cacheDirectory, models.RuntimeConfig{})
	reconstructed := newGenericService(t, reconstructedScopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("reconstructed backend upgrade inspection must not use network")
	}), func(string) string { return "" })
	reconstructedInspection := inspectNamedGenericRuntime(t, reconstructed, reconstructedScope, backendRequest.Name)
	if !reconstructedInspection.BackendRequired ||
		reconstructedInspection.BackendCachePath != upgraded.BackendCachePath ||
		len(reconstructedInspection.BackendFiles) != 1 {
		t.Fatalf("reconstructed backend inspection = %#v, upgraded inspection = %#v", reconstructedInspection, upgraded)
	}
	assertCommittedBackendPath(t, reconstructedInspection, backendBody)
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
	metadata, err := json.Marshal(cacheMetadata{
		ModelName: "embed",
		Revision:  staleRevision,
		Files: []metadataFile{{
			Path: "model.safetensors", Bytes: int64(len(staleBody)), SHA256: sha256Hex(staleBody),
		}},
	})
	if err != nil {
		t.Fatalf("marshal stale generic metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelDirectory, metadataFileName), metadata, 0o644); err != nil {
		t.Fatalf("write stale generic metadata: %v", err)
	}

	scopes := newScopes(t, "generic-source-mismatch")
	scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("source-mismatched generic inspection used the network")
		return nil, nil
	}), func(string) string { return "" })
	inspection, err := service.InspectRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  "embed",
	})
	if err != nil {
		t.Fatalf("InspectRuntimeCache: %v", err)
	}
	if !inspection.Supported || !inspection.ManifestPresent || !inspection.ManifestValid ||
		inspection.Installed || inspection.IntegrityVerified ||
		inspection.FailureReason != "managed cache does not match configured source" {
		t.Fatalf("source-mismatched inspection = %#v, want unavailable cache", inspection)
	}
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

func TestPrepareGenericAssetsBindsPublishedSnapshotForRuntimeHost(t *testing.T) {
	t.Parallel()

	localPath := filepath.Join(t.TempDir(), "weights.gguf")
	body := []byte("joined runtime weights")
	if err := os.WriteFile(localPath, body, 0o644); err != nil {
		t.Fatalf("write local fixture: %v", err)
	}
	scopes := newScopes(t, "generic-runtime-binding")
	scope := openScope(t, scopes, t.TempDir(), models.RuntimeConfig{})
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("local joined preparation used the network")
		return nil, nil
	}), func(string) string { return "" })

	if _, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      "joined-model",
		Reference: models.ModelReference{NameOrURI: localPath},
		Artifacts: []models.AssetRequirement{{Name: filepath.Base(localPath), Bytes: int64(len(body)), SHA256: sha256Hex(body)}},
	}); err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	inspection, err := service.InspectRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  "joined-model",
	})
	if err != nil {
		t.Fatalf("InspectRuntimeCache: %v", err)
	}
	if !inspection.Supported || !inspection.Installed || inspection.CachePath == "" ||
		inspection.InstalledFileCount != 1 {
		t.Fatalf("runtime inspection = %#v, want prepared generic snapshot", inspection)
	}
	if info, err := os.Stat(inspection.CachePath); err != nil || !info.IsDir() {
		t.Fatalf("runtime cache path = %q, stat = (%v, %#v), want snapshot directory",
			inspection.CachePath, err, info)
	}
}

func TestPrepareGenericAssetsPassesCommittedBackendPathsToRuntimeInspection(t *testing.T) {
	t.Parallel()

	body := []byte("runtime backend archive")
	localPath := filepath.Join(t.TempDir(), "backend.zip")
	if err := os.WriteFile(localPath, body, 0o644); err != nil {
		t.Fatalf("write backend fixture: %v", err)
	}
	scopes := newScopes(t, "generic-runtime-backend-paths")
	cacheDirectory := t.TempDir()
	scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("local backend preparation used the network")
		return nil, nil
	}), func(string) string { return "" })
	request := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      "joined-model",
		Reference: models.ModelReference{NameOrURI: localPath},
		Artifacts: []models.AssetRequirement{{
			Name: "model.bin", Bytes: int64(len(body)), SHA256: sha256Hex(body),
		}},
		Backend: "localai-vibevoice",
		BackendArtifacts: []models.AssetRequirement{{
			Name: "backend.zip", Bytes: int64(len(body)), SHA256: sha256Hex(body),
		}},
	}
	if _, err := service.PrepareModelAssets(context.Background(), request); err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	inspection := requireBackendRuntimeInspection(t, service, scope, request.Name)
	assertCommittedBackendPath(t, inspection, body)
}

func requireBackendRuntimeInspection(
	t *testing.T,
	service *service,
	scope models.RuntimeScopeRef,
	name string,
) assets.RuntimeCacheInspection {
	t.Helper()
	inspection, err := service.InspectRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  name,
	})
	if err != nil {
		t.Fatalf("InspectRuntimeCache: %v", err)
	}
	if !inspection.BackendRequired || len(inspection.BackendFiles) != 1 || inspection.BackendCachePath == "" {
		t.Fatalf("runtime backend inspection = %#v, want one backend file", inspection)
	}
	return inspection
}

func assertCommittedBackendPath(t *testing.T, inspection assets.RuntimeCacheInspection, wantBody []byte) {
	t.Helper()
	path := inspection.BackendFiles[0]
	if !filepath.IsAbs(path) || strings.Contains(path, ".partial") {
		t.Fatalf("runtime backend path = %q, want absolute non-partial path", path)
	}
	relative, err := filepath.Rel(inspection.BackendCachePath, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("runtime backend path %q escaped cache snapshot %q", path, inspection.BackendCachePath)
	}
	info, err := os.Stat(path)
	if err != nil || info == nil || !info.Mode().IsRegular() {
		t.Fatalf("runtime backend path stat = (%#v, %v), want existing regular file", info, err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, wantBody) {
		t.Fatalf("runtime backend body = (%q, %v), want %q", got, err, wantBody)
	}
}

func TestPrepareGenericAssetsDownloadsPinnedBackendIntoSeparateRuntimeCache(t *testing.T) {
	t.Parallel()

	modelPath := filepath.Join(t.TempDir(), "weights.gguf")
	modelBody := []byte("joined runtime weights")
	backendBody := []byte("pinned backend archive")
	if err := os.WriteFile(modelPath, modelBody, 0o644); err != nil {
		t.Fatalf("write model fixture: %v", err)
	}
	backendURL := "https://github.com/owner/backend/releases/download/v1/backend.bin"
	scopes := newScopes(t, "generic-pinned-backend")
	scope := openScope(t, scopes, t.TempDir(), models.RuntimeConfig{})
	service := newGenericService(t, scopes, httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != backendURL {
			t.Fatalf("backend download URL = %q, want %q", request.URL.String(), backendURL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(backendBody)),
		}, nil
	}), func(string) string { return "" })

	result, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      "joined-model",
		Reference: models.ModelReference{NameOrURI: modelPath},
		Artifacts: []models.AssetRequirement{{
			Name: filepath.Base(modelPath), Bytes: int64(len(modelBody)), SHA256: sha256Hex(modelBody),
		}},
		Backend:          "localai-vibevoice",
		BackendReference: models.ModelReference{NameOrURI: backendURL},
		BackendArtifacts: []models.AssetRequirement{{
			Name: "backend.bin", Bytes: int64(len(backendBody)), SHA256: sha256Hex(backendBody),
		}},
	})
	if err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	if len(result.Asset.Artifacts) != 1 || len(result.Asset.BackendArtifacts) != 1 ||
		result.Asset.BackendArtifacts[0].SHA256 != sha256Hex(backendBody) {
		t.Fatalf("prepared asset snapshot = %#v, want separate verified backend artifact", result.Asset)
	}
	inspection, err := service.InspectRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  "joined-model",
	})
	if err != nil {
		t.Fatalf("InspectRuntimeCache: %v", err)
	}
	if !inspection.BackendRequired || inspection.BackendCachePath == "" ||
		inspection.BackendInstalledFiles != 1 {
		t.Fatalf("runtime inspection = %#v, want installed backend cache facts", inspection)
	}
	backendPath := filepath.Join(inspection.BackendCachePath, "backend.bin")
	if body, err := os.ReadFile(backendPath); err != nil || !bytes.Equal(body, backendBody) {
		t.Fatalf("backend cache file = (%q, %v), want verified archive", body, err)
	}
}

func TestPrepareGenericAssetsRecoversStaleGenericRuntimeRevisionStage(t *testing.T) {
	t.Parallel()
	assertGenericRuntimeStageRecovery(t, true, false)
}

func TestPrepareGenericAssetsRecoversStaleGenericRuntimeMetadataStage(t *testing.T) {
	t.Parallel()
	assertGenericRuntimeStageRecovery(t, false, true)
}

func TestPrepareGenericAssetsRecoversBothStaleGenericRuntimeStages(t *testing.T) {
	t.Parallel()
	assertGenericRuntimeStageRecovery(t, true, true)
}

func assertGenericRuntimeStageRecovery(t *testing.T, revisionStage, metadataStage bool) {
	t.Helper()
	fixture := newGenericRuntimeRecoveryFixture(t, "stale-managed-runtime")
	root := removeGenericRuntimeCommit(t, fixture)
	revisionStagePath := filepath.Join(root, fixture.inspection.Revision+".partial")
	metadataStagePath := filepath.Join(root, metadataFileName+".partial")
	if revisionStage {
		writeGenericRuntimeRevisionStage(t, revisionStagePath, []byte("abandoned revision stage"))
	}
	if metadataStage {
		if err := os.WriteFile(metadataStagePath, []byte("abandoned metadata stage"), 0o644); err != nil {
			t.Fatalf("write abandoned metadata stage: %v", err)
		}
	}
	if err := os.RemoveAll(fixture.sourceRoot); err != nil {
		t.Fatalf("remove original fixture sources: %v", err)
	}
	request := fixture.request
	request.Offline = true
	prepared, err := fixture.service.PrepareModelAssets(context.Background(), request)
	if err != nil {
		t.Fatalf("PrepareModelAssets after stale staging: %v", err)
	}
	if prepared.Outcome != models.AssetPreparationAlreadyAvailable {
		t.Fatalf("PrepareModelAssets outcome = %v, want verified cache reuse", prepared.Outcome)
	}
	assertGenericRuntimeRecoveryContents(
		t, inspectNamedGenericRuntime(t, fixture.service, fixture.scope, request.Name),
		fixture.modelBodies, fixture.backendBody,
	)
	assertPathAbsent(t, revisionStagePath)
	assertPathAbsent(t, metadataStagePath)
}

func TestPrepareGenericAssetsPreservesCommittedRuntimeAndUnrelatedStages(t *testing.T) {
	t.Parallel()
	fixture := newGenericRuntimeRecoveryFixture(t, "preserve-managed-runtime")
	root := filepath.Join(fixture.cacheDirectory, canonicalModelName(fixture.request.Name))
	metadataPath := filepath.Join(root, metadataFileName)
	metadataBefore, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read committed metadata: %v", err)
	}
	revisionStagePath := filepath.Join(root, fixture.inspection.Revision+".partial")
	metadataStagePath := metadataPath + ".partial"
	writeGenericRuntimeRevisionStage(t, revisionStagePath, []byte("abandoned revision stage"))
	if err := os.WriteFile(metadataStagePath, []byte("abandoned metadata stage"), 0o644); err != nil {
		t.Fatalf("write abandoned metadata stage: %v", err)
	}
	otherRevision := strings.Repeat("b", len(fixture.inspection.Revision))
	otherRevisionPath := filepath.Join(root, otherRevision+".partial")
	writeGenericRuntimeRevisionStage(t, otherRevisionPath, []byte("unrelated revision stage"))
	otherModelPath := filepath.Join(fixture.cacheDirectory, canonicalModelName("another-model"), fixture.inspection.Revision+".partial")
	writeGenericRuntimeRevisionStage(t, otherModelPath, []byte("other model stage"))

	request := fixture.request
	request.Offline = true
	prepared, err := fixture.service.PrepareModelAssets(context.Background(), request)
	if err != nil {
		t.Fatalf("PrepareModelAssets with committed cache: %v", err)
	}
	if prepared.Outcome != models.AssetPreparationAlreadyAvailable {
		t.Fatalf("PrepareModelAssets outcome = %v, want already available", prepared.Outcome)
	}
	inspection := inspectNamedGenericRuntime(t, fixture.service, fixture.scope, request.Name)
	assertGenericRuntimeRecoveryContents(t, inspection, fixture.modelBodies, fixture.backendBody)
	if inspection.Revision != fixture.inspection.Revision || filepath.Clean(inspection.CachePath) != filepath.Clean(fixture.inspection.CachePath) {
		t.Fatalf("committed runtime identity changed: before=%#v after=%#v", fixture.inspection, inspection)
	}
	metadataAfter, err := os.ReadFile(metadataPath)
	if err != nil || !bytes.Equal(metadataBefore, metadataAfter) {
		t.Fatalf("committed metadata changed: before=%q after=%q err=%v", metadataBefore, metadataAfter, err)
	}
	assertPathAbsent(t, revisionStagePath)
	assertPathAbsent(t, metadataStagePath)
	assertFileBody(t, filepath.Join(otherRevisionPath, "abandoned.bin"), []byte("unrelated revision stage"))
	assertFileBody(t, filepath.Join(otherModelPath, "abandoned.bin"), []byte("other model stage"))
}

func TestPrepareGenericAssetsReturnsTypedErrorWhenStaleStageCleanupFails(t *testing.T) {
	t.Parallel()
	fixture := newGenericRuntimeRecoveryFixture(t, "stale-stage-cleanup-failure")
	root := removeGenericRuntimeCommit(t, fixture)
	revisionStagePath := filepath.Join(root, fixture.inspection.Revision+".partial")
	metadataStagePath := filepath.Join(root, metadataFileName+".partial")
	writeGenericRuntimeRevisionStage(t, revisionStagePath, []byte("stale stage"))
	if err := os.WriteFile(metadataStagePath, []byte("stale metadata"), 0o644); err != nil {
		t.Fatalf("write stale metadata: %v", err)
	}

	cleanupErr := errors.New("injected stale stage inspection failure")
	originalInspect := fixture.service.inspectPath
	fixture.service.inspectPath = func(path string) (os.FileInfo, error) {
		if filepath.Clean(path) == filepath.Clean(revisionStagePath) {
			return nil, cleanupErr
		}
		return originalInspect(path)
	}
	request := fixture.request
	request.Offline = true
	_, err := fixture.service.PrepareModelAssets(context.Background(), request)
	if !errors.Is(err, models.ErrAssetPreparationInterrupted) || !errors.Is(err, cleanupErr) {
		t.Fatalf("PrepareModelAssets error = %v, want typed cleanup failure", err)
	}
	assertPathAbsent(t, filepath.Join(root, fixture.inspection.Revision))
	assertPathAbsent(t, filepath.Join(root, metadataFileName))
	assertFileBody(t, filepath.Join(revisionStagePath, "abandoned.bin"), []byte("stale stage"))
	assertFileBody(t, metadataStagePath, []byte("stale metadata"))
}

func TestPublishGenericRuntimeLeavesStaleStagesWhenModelLockIsUnavailable(t *testing.T) {
	t.Parallel()
	fixture := newGenericRuntimeRecoveryFixture(t, "stale-stage-lock-failure")
	root := filepath.Join(fixture.cacheDirectory, canonicalModelName(fixture.request.Name))
	revisionStagePath := filepath.Join(root, fixture.inspection.Revision+".partial")
	metadataStagePath := filepath.Join(root, metadataFileName+".partial")
	writeGenericRuntimeRevisionStage(t, revisionStagePath, []byte("stale stage"))
	if err := os.WriteFile(metadataStagePath, []byte("stale metadata"), 0o644); err != nil {
		t.Fatalf("write stale metadata: %v", err)
	}

	source, err := parseGenericSource(fixture.request.Reference.NameOrURI)
	if err != nil {
		t.Fatalf("parse model source: %v", err)
	}
	paths := make([]string, 0, len(fixture.inspection.ObservedArtifacts))
	for _, artifact := range fixture.inspection.ObservedArtifacts {
		paths = append(paths, filepath.Join(fixture.inspection.CachePath, filepath.FromSlash(artifact.Name)))
	}
	lockErr := errors.New("injected model lock failure")
	fixture.service.coordination = rejectingStagingCoordination{err: lockErr}
	_, err = fixture.service.publishGenericRuntimeCache(
		context.Background(), fixture.cacheDirectory, fixture.request.Name, source,
		genericCacheResult{
			snapshotPath: fixture.inspection.CachePath,
			artifacts:    fixture.inspection.ObservedArtifacts,
			paths:        paths,
		}, genericCacheResult{}, "",
	)
	if !errors.Is(err, models.ErrAssetPreparationInterrupted) || !errors.Is(err, lockErr) {
		t.Fatalf("publishGenericRuntimeCache error = %v, want typed lock failure", err)
	}
	assertFileBody(t, filepath.Join(revisionStagePath, "abandoned.bin"), []byte("stale stage"))
	assertFileBody(t, metadataStagePath, []byte("stale metadata"))
}

type genericRuntimeRecoveryFixture struct {
	cacheDirectory string
	sourceRoot     string
	scope          models.RuntimeScopeRef
	service        *service
	request        models.PrepareModelAssetsRequest
	modelBodies    map[string][]byte
	backendBody    []byte
	inspection     assets.RuntimeCacheInspection
}

func newGenericRuntimeRecoveryFixture(t *testing.T, issuer string) genericRuntimeRecoveryFixture {
	t.Helper()
	cacheDirectory := t.TempDir()
	sourceRoot := t.TempDir()
	modelDirectory := filepath.Join(sourceRoot, "model")
	if err := os.MkdirAll(modelDirectory, 0o755); err != nil {
		t.Fatalf("create controlled model directory: %v", err)
	}
	modelBodies := map[string][]byte{
		"weights.gguf":  []byte("controlled model weights"),
		"projector.bin": []byte("controlled projector bytes"),
	}
	modelArtifacts := []models.AssetRequirement{
		{Name: "weights.gguf", Bytes: int64(len(modelBodies["weights.gguf"])), SHA256: sha256Hex(modelBodies["weights.gguf"])},
		{Name: "projector.bin", Bytes: int64(len(modelBodies["projector.bin"])), SHA256: sha256Hex(modelBodies["projector.bin"])},
	}
	for _, artifact := range modelArtifacts {
		if err := os.WriteFile(filepath.Join(modelDirectory, artifact.Name), modelBodies[artifact.Name], 0o644); err != nil {
			t.Fatalf("write controlled model artifact %q: %v", artifact.Name, err)
		}
	}
	backendBody := []byte("controlled backend archive")
	backendPath := filepath.Join(sourceRoot, "backend.zip")
	if err := os.WriteFile(backendPath, backendBody, 0o644); err != nil {
		t.Fatalf("write controlled backend artifact: %v", err)
	}
	scopes := newScopes(t, issuer)
	scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("controlled stale-stage recovery used the network")
		return nil, nil
	}), func(string) string { return "" })
	request := models.PrepareModelAssetsRequest{
		Scope:            scope,
		Name:             "recovery-model",
		Reference:        models.ModelReference{NameOrURI: modelDirectory},
		Artifacts:        modelArtifacts,
		Backend:          "localai-vibevoice",
		BackendReference: models.ModelReference{NameOrURI: backendPath},
		BackendArtifacts: []models.AssetRequirement{{
			Name: "backend.zip", Bytes: int64(len(backendBody)), SHA256: sha256Hex(backendBody),
		}},
	}
	prepared, err := service.PrepareModelAssets(context.Background(), request)
	if err != nil || prepared.Outcome != models.AssetPreparationPrepared {
		t.Fatalf("initial PrepareModelAssets = (%#v, %v), want prepared controlled runtime", prepared, err)
	}
	inspection := inspectNamedGenericRuntime(t, service, scope, request.Name)
	assertGenericRuntimeRecoveryContents(t, inspection, modelBodies, backendBody)
	return genericRuntimeRecoveryFixture{
		cacheDirectory: cacheDirectory,
		sourceRoot:     sourceRoot,
		scope:          scope,
		service:        service,
		request:        request,
		modelBodies:    modelBodies,
		backendBody:    backendBody,
		inspection:     inspection,
	}
}

func assertGenericRuntimeRecoveryContents(
	t *testing.T,
	inspection assets.RuntimeCacheInspection,
	modelBodies map[string][]byte,
	backendBody []byte,
) {
	t.Helper()
	if !inspection.Supported || !inspection.Installed || !inspection.ManifestPresent ||
		!inspection.ManifestValid || !inspection.IntegrityVerified ||
		inspection.InstalledFileCount != len(modelBodies) ||
		len(inspection.ObservedArtifacts) != len(modelBodies) || !inspection.BackendRequired ||
		inspection.BackendInstalledFiles != 1 || len(inspection.BackendFiles) != 1 {
		t.Fatalf("runtime inspection = %#v, want verified multi-artifact model and backend", inspection)
	}
	observed := make(map[string]models.AssetArtifact, len(inspection.ObservedArtifacts))
	var cacheBytes int64
	for _, artifact := range inspection.ObservedArtifacts {
		observed[artifact.Name] = artifact
	}
	for name, body := range modelBodies {
		artifact, ok := observed[name]
		if !ok || artifact.Bytes != int64(len(body)) || artifact.SHA256 != sha256Hex(body) {
			t.Fatalf("managed artifact %q = %#v, want %d bytes with digest %s", name, artifact, len(body), sha256Hex(body))
		}
		assertFileBody(t, filepath.Join(inspection.CachePath, filepath.FromSlash(name)), body)
		cacheBytes += int64(len(body))
	}
	if inspection.CacheBytes != cacheBytes {
		t.Fatalf("managed cache bytes = %d, want %d", inspection.CacheBytes, cacheBytes)
	}
	assertFileBody(t, inspection.BackendFiles[0], backendBody)
}

func removeGenericRuntimeCommit(t *testing.T, fixture genericRuntimeRecoveryFixture) string {
	t.Helper()
	root := filepath.Join(fixture.cacheDirectory, canonicalModelName(fixture.request.Name))
	if err := os.RemoveAll(fixture.inspection.CachePath); err != nil {
		t.Fatalf("remove committed managed runtime: %v", err)
	}
	if err := os.Remove(filepath.Join(root, metadataFileName)); err != nil {
		t.Fatalf("remove committed managed metadata: %v", err)
	}
	return root
}

func writeGenericRuntimeRevisionStage(t *testing.T, stagePath string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(stagePath, 0o755); err != nil {
		t.Fatalf("create managed runtime revision stage: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagePath, "abandoned.bin"), body, 0o644); err != nil {
		t.Fatalf("write managed runtime revision stage: %v", err)
	}
}

func assertPathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("path %q stat error = %v, want not-exist", path, err)
	}
}
