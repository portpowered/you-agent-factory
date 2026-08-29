package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	platformlocking "github.com/portpowered/infinite-you/pkg/platform/locking"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	assets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
)

func TestInspectModelAssetsReturnsDetachedConfiguredSourceAndCacheFacts(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		configured   string
		wantProvider string
	}{
		{name: "default upstream", wantProvider: "UPSTREAM_REPOSITORY"},
		{name: "explicit upstream", configured: "HUGGINGFACE", wantProvider: "UPSTREAM_REPOSITORY"},
		{name: "managed mirror", configured: "MODELSCOPE", wantProvider: "MANAGED_MIRROR"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cacheDirectory := t.TempDir()
			writeCacheFixture(t, cacheDirectory, true)
			scopes := newScopes(t, test.name)
			ref := openScope(t, scopes, cacheDirectory, runtimeConfig(test.configured))
			service := newTestService(scopes, nil)

			first, err := service.InspectModelAssets(
				context.Background(),
				models.InspectModelAssetsRequest{Scope: ref, Name: "omnivoice_q4_k_m"},
			)
			if err != nil {
				t.Fatalf("InspectModelAssets: %v", err)
			}
			assertAvailableSnapshot(t, first.Asset, test.wantProvider)

			first.Asset.Artifacts[0].Name = "peer mutation"
			second, err := service.InspectModelAssets(
				context.Background(),
				models.InspectModelAssetsRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"},
			)
			if err != nil {
				t.Fatalf("InspectModelAssets repeated: %v", err)
			}
			if second.Asset.Artifacts[0].Name != "omnivoice-base-Q4_K_M.gguf" {
				t.Fatalf("repeated inspection retained peer mutation: %#v", second.Asset.Artifacts)
			}
		})
	}
}

func TestInspectModelAssetsDiscoversCompleteLocalRevisionWithoutMetadata(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, false)
	scopes := newScopes(t, "discovery")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)

	result, err := service.InspectModelAssets(
		context.Background(),
		models.InspectModelAssetsRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"},
	)
	if err != nil {
		t.Fatalf("InspectModelAssets: %v", err)
	}
	assertAvailableSnapshot(t, result.Asset, "UPSTREAM_REPOSITORY")
	for _, artifact := range result.Asset.Artifacts {
		if artifact.SHA256 != "" {
			t.Fatalf("discovered artifact unexpectedly has checksum metadata: %#v", artifact)
		}
	}
}

func TestRuntimeCacheCompatibilityFactsComeFromScopedAssetsService(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "runtime-cache-compatibility")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)
	request := models.InspectModelAssetsRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	}

	layout, err := service.ResolveRuntimeCache(context.Background(), request)
	if err != nil {
		t.Fatalf("ResolveRuntimeCache: %v", err)
	}
	if layout.ModelName != "OMNIVOICE_Q4_K_M" ||
		layout.Revision != "rev-test" ||
		len(layout.Files) != 2 {
		t.Fatalf("ResolveRuntimeCache = %#v", layout)
	}
	for _, path := range layout.Files {
		if filepath.Dir(path) != layout.CachePath {
			t.Fatalf("runtime cache file %q is outside cache path %q", path, layout.CachePath)
		}
	}

	inspection, err := service.InspectRuntimeCache(context.Background(), request)
	if err != nil {
		t.Fatalf("InspectRuntimeCache: %v", err)
	}
	if !inspection.Supported ||
		!inspection.Installed ||
		inspection.Revision != "rev-test" ||
		inspection.CachePath != filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M", "rev-test") ||
		inspection.InstalledFileCount != 2 ||
		inspection.CacheBytes != 4 ||
		len(inspection.MissingAssets) != 0 {
		t.Fatalf("InspectRuntimeCache = %#v", inspection)
	}
}

func TestInspectRuntimeCacheReportsInvalidConfiguredManifest(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	modelDirectory := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	if err := os.MkdirAll(modelDirectory, 0o755); err != nil {
		t.Fatalf("create model cache directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(modelDirectory, metadataFileName),
		[]byte(`{"revision":`),
		0o644,
	); err != nil {
		t.Fatalf("write malformed cache metadata: %v", err)
	}

	scopes := newScopes(t, "runtime-cache-invalid-manifest")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)
	inspection, err := service.InspectRuntimeCache(
		context.Background(),
		models.InspectModelAssetsRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"},
	)
	if err != nil {
		t.Fatalf("InspectRuntimeCache: %v", err)
	}
	if !inspection.Supported || !inspection.ManifestPresent || inspection.ManifestValid ||
		inspection.Installed || inspection.FailureReason != "managed cache manifest is invalid" {
		t.Fatalf("invalid manifest inspection = %#v", inspection)
	}
	if len(inspection.ExpectedArtifacts) != 2 ||
		inspection.ExpectedArtifacts[0].Name != "omnivoice-base-Q4_K_M.gguf" ||
		inspection.ExpectedArtifacts[1].Name != "omnivoice-tokenizer-Q4_K_M.gguf" {
		t.Fatalf("invalid manifest expected artifacts = %#v", inspection.ExpectedArtifacts)
	}
}

func TestInspectRuntimeCacheVerifiesConfiguredArtifactIntegrity(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeVerifiedCacheFixture(t, cacheDirectory)
	scopes := newScopes(t, "runtime-cache-integrity")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)
	inspection, err := service.InspectRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("InspectRuntimeCache: %v", err)
	}
	if !inspection.Supported || !inspection.Installed || !inspection.ManifestValid ||
		!inspection.IntegrityVerified || inspection.InstalledFileCount != 2 ||
		len(inspection.MissingAssets) != 0 || len(inspection.ObservedArtifacts) != 2 {
		t.Fatalf("verified runtime inspection = %#v", inspection)
	}
}

func TestPrepareModelAssetsCopiesDirectoryBackedGenericArtifacts(t *testing.T) {
	t.Parallel()

	localRoot := t.TempDir()
	localPath := filepath.Join(localRoot, "nested", "weights.gguf")
	body := []byte("directory-backed model weights")
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("create local model directory: %v", err)
	}
	if err := os.WriteFile(localPath, body, 0o644); err != nil {
		t.Fatalf("write local model artifact: %v", err)
	}

	scopes := newScopes(t, "prepare-directory-generic")
	cacheDirectory := t.TempDir()
	scope := openScope(t, scopes, cacheDirectory, models.RuntimeConfig{})
	service := newGenericService(t, scopes, httpDoerFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("directory-backed preparation used the network")
		return nil, nil
	}), func(string) string { return "" })
	request := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      "directory-model",
		Reference: models.ModelReference{NameOrURI: localRoot},
		Artifacts: []models.AssetRequirement{{
			Name: "nested/weights.gguf", Bytes: int64(len(body)), SHA256: sha256Hex(body),
		}},
	}

	result, err := service.PrepareModelAssets(context.Background(), request)
	if err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	assertPreparedDirectoryAsset(t, result, body)

	inspection, err := service.InspectRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  "directory-model",
	})
	if err != nil {
		t.Fatalf("InspectRuntimeCache after directory preparation: %v", err)
	}
	assertDirectoryRuntimeInspection(t, inspection, body)
}

func assertPreparedDirectoryAsset(t *testing.T, result models.PrepareModelAssetsResult, body []byte) {
	t.Helper()
	if result.Outcome != models.AssetPreparationPrepared ||
		result.Asset.Readiness != models.AssetReadinessAvailable ||
		result.Asset.Integrity != models.AssetIntegrityVerified ||
		result.Asset.Source.Provider != "LOCAL" || len(result.Asset.Artifacts) != 1 ||
		result.Asset.Artifacts[0].Name != "nested/weights.gguf" ||
		result.Asset.Artifacts[0].Bytes != int64(len(body)) ||
		result.Asset.Artifacts[0].SHA256 != sha256Hex(body) {
		t.Fatalf("directory-backed preparation result = %#v", result)
	}
}

func assertDirectoryRuntimeInspection(t *testing.T, inspection assets.RuntimeCacheInspection, body []byte) {
	t.Helper()
	if !inspection.Installed || !inspection.ManifestValid || inspection.InstalledFileCount != 1 ||
		len(inspection.ObservedArtifacts) != 1 ||
		inspection.ObservedArtifacts[0].Name != "nested/weights.gguf" {
		t.Fatalf("directory-backed runtime inspection = %#v", inspection)
	}
	assertFileBody(t, filepath.Join(inspection.CachePath, filepath.FromSlash("nested/weights.gguf")), body)
}

func TestInspectRuntimeCacheCountsNestedRegularFilesAndSkipsSymlinks(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	revisionDirectory := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M", "rev-test")
	nestedDirectory := filepath.Join(revisionDirectory, "nested", "deeper")
	if err := os.MkdirAll(nestedDirectory, 0o755); err != nil {
		t.Fatalf("create nested cache directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(revisionDirectory, "extra.bin"), []byte("extra"), 0o644); err != nil {
		t.Fatalf("write extra cache file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nestedDirectory, "nested.bin"), []byte("nested"), 0o644); err != nil {
		t.Fatalf("write nested cache file: %v", err)
	}

	externalFile := filepath.Join(t.TempDir(), "outside.bin")
	if err := os.WriteFile(externalFile, []byte("outside-file"), 0o644); err != nil {
		t.Fatalf("write external cache file: %v", err)
	}
	if err := os.Symlink(externalFile, filepath.Join(revisionDirectory, "external-link")); err != nil {
		if runtime.GOOS != "windows" && !errors.Is(err, os.ErrPermission) {
			t.Fatalf("create cache symlink: %v", err)
		}
	}

	scopes := newScopes(t, "runtime-cache-accounting")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)
	inspection, err := service.InspectRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("InspectRuntimeCache: %v", err)
	}
	if inspection.CacheBytes != 15 {
		t.Fatalf("CacheBytes = %d, want 15 bytes from required plus nested regular files", inspection.CacheBytes)
	}
}

func TestInspectRuntimeCacheRejectsRevisionEscapingManagedCacheRoot(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	modelDirectory := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	if err := os.MkdirAll(modelDirectory, 0o755); err != nil {
		t.Fatalf("create model cache directory: %v", err)
	}
	body, err := json.Marshal(cacheMetadata{
		ModelName: "OMNIVOICE_Q4_K_M",
		Revision:  "../outside",
	})
	if err != nil {
		t.Fatalf("marshal malicious cache metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelDirectory, metadataFileName), body, 0o644); err != nil {
		t.Fatalf("write malicious cache metadata: %v", err)
	}

	scopes := newScopes(t, "runtime-cache-path-validation")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)
	if _, err := service.InspectRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	}); err == nil {
		t.Fatal("InspectRuntimeCache succeeded for a revision outside the managed cache root")
	}
}

func TestInspectRuntimeCacheReportsInvalidManagedManifest(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	modelRoot := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	if err := os.MkdirAll(modelRoot, 0o755); err != nil {
		t.Fatalf("create model cache root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelRoot, metadataFileName), []byte("{"), 0o644); err != nil {
		t.Fatalf("write invalid manifest: %v", err)
	}
	scopes := newScopes(t, "invalid-manifest")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)
	inspection, err := service.InspectRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: ref, Name: "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("InspectRuntimeCache: %v", err)
	}
	if !inspection.Supported || !inspection.ManifestPresent || inspection.FailureReason != "managed cache manifest is invalid" {
		t.Fatalf("invalid manifest inspection = %#v, want supported invalid-manifest diagnostics", inspection)
	}
}

func TestInspectRuntimeCacheMarksVerifiedManagedManifest(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeVerifiedCacheFixture(t, cacheDirectory)
	scopes := newScopes(t, "verified-runtime-cache")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)
	inspection, err := service.InspectRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: ref, Name: "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("InspectRuntimeCache: %v", err)
	}
	if !inspection.Supported || !inspection.Installed || !inspection.IntegrityVerified || inspection.Revision != "verified-revision" || inspection.CacheBytes == 0 {
		t.Fatalf("verified runtime cache inspection = %#v, want installed verified facts", inspection)
	}
}

func TestInspectRuntimeCacheReportsCorruptManagedArtifact(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeVerifiedCacheFixture(t, cacheDirectory)
	corruptPath := filepath.Join(
		cacheDirectory,
		"OMNIVOICE_Q4_K_M",
		"verified-revision",
		"omnivoice-base-Q4_K_M.gguf",
	)
	if err := os.WriteFile(corruptPath, []byte("corrupt"), 0o644); err != nil {
		t.Fatalf("corrupt cached artifact: %v", err)
	}
	scopes := newScopes(t, "corrupt-runtime-cache")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)

	inspection, err := service.InspectRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: ref, Name: "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("InspectRuntimeCache: %v", err)
	}
	if inspection.Installed || inspection.IntegrityVerified || !inspection.PartialArtifacts ||
		inspection.FailureReason != "asset integrity verification failed" {
		t.Fatalf("corrupt runtime cache inspection = %#v, want failed integrity diagnostics", inspection)
	}
}

func TestInspectRuntimeCacheReportsMissingManagedArtifact(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeVerifiedCacheFixture(t, cacheDirectory)
	if err := os.Remove(filepath.Join(
		cacheDirectory,
		"OMNIVOICE_Q4_K_M",
		"verified-revision",
		"omnivoice-tokenizer-Q4_K_M.gguf",
	)); err != nil {
		t.Fatalf("remove cached artifact: %v", err)
	}
	scopes := newScopes(t, "missing-runtime-cache")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)

	inspection, err := service.InspectRuntimeCache(context.Background(), models.InspectModelAssetsRequest{
		Scope: ref, Name: "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("InspectRuntimeCache: %v", err)
	}
	if inspection.Installed || inspection.IntegrityVerified || len(inspection.MissingAssets) != 1 ||
		inspection.MissingAssets[0] != "omnivoice-tokenizer-Q4_K_M.gguf" {
		t.Fatalf("missing runtime cache inspection = %#v, want one missing artifact", inspection)
	}
}

func TestInspectRuntimeCachePreservesCancellationIdentityAndRedactsCause(t *testing.T) {
	t.Parallel()

	scopes := newScopes(t, "runtime-cache-cancelled")
	ref := openScope(t, scopes, t.TempDir(), runtimeConfig(""))
	secretCause := errors.New("HF_TOKEN=secret cache=C:\\private\\weights")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(secretCause)
	service := newTestService(scopes, nil)

	_, err := service.InspectRuntimeCache(ctx, models.InspectModelAssetsRequest{
		Scope: ref, Name: "OMNIVOICE_Q4_K_M",
	})
	if !errors.Is(err, models.ErrAssetCancelled) || !errors.Is(err, context.Canceled) || !errors.Is(err, secretCause) {
		t.Fatalf("cancelled inspection error = %v, want typed cancellation and cause identity", err)
	}
	if err.Error() != models.ErrAssetCancelled.Error() || strings.Contains(err.Error(), "HF_TOKEN") {
		t.Fatalf("cancelled inspection error leaked cause: %q", err.Error())
	}
	if errors.Unwrap(err) == nil {
		t.Fatal("cancelled inspection error has no wrapped cause")
	}
}

func TestInspectModelAssetsVerifiesMetadataBackedCache(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeVerifiedCacheFixture(t, cacheDirectory)
	scopes := newScopes(t, "verified-inspection")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)

	result, err := service.InspectModelAssets(
		context.Background(),
		models.InspectModelAssetsRequest{
			Scope: ref, Name: "OMNIVOICE_Q4_K_M", VerifyIntegrity: true,
		},
	)
	if err != nil {
		t.Fatalf("InspectModelAssets: %v", err)
	}
	if result.Asset.Readiness != models.AssetReadinessAvailable ||
		result.Asset.Integrity != models.AssetIntegrityVerified ||
		result.Asset.Revision != "verified-revision" ||
		len(result.Asset.Artifacts) != 2 ||
		result.Asset.Artifacts[0].SHA256 == "" {
		t.Fatalf("verified inspection = %#v", result.Asset)
	}
}

func TestInspectModelAssetsReportsCorruptCacheWithDetachedDiagnostics(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeVerifiedCacheFixture(t, cacheDirectory)
	corruptPath := filepath.Join(
		cacheDirectory,
		"OMNIVOICE_Q4_K_M",
		"verified-revision",
		"omnivoice-base-Q4_K_M.gguf",
	)
	corruptBody := []byte("cached basf")
	if err := os.WriteFile(corruptPath, corruptBody, 0o644); err != nil {
		t.Fatalf("corrupt cached asset: %v", err)
	}
	scopes := newScopes(t, "corrupt-inspection")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig("MODELSCOPE"))
	service := newTestService(scopes, nil)

	result, err := service.InspectModelAssets(
		context.Background(),
		models.InspectModelAssetsRequest{
			Scope: ref, Name: "OMNIVOICE_Q4_K_M", VerifyIntegrity: true,
		},
	)
	if !errors.Is(err, models.ErrAssetIntegrityFailed) {
		t.Fatalf("InspectModelAssets error = %v, want ErrAssetIntegrityFailed", err)
	}
	if result.Asset.ModelName != "OMNIVOICE_Q4_K_M" ||
		result.Asset.Readiness != models.AssetReadinessFailed ||
		result.Asset.Integrity != models.AssetIntegrityFailed ||
		result.Asset.Source.Provider != "MANAGED_MIRROR" ||
		result.Asset.Revision != "verified-revision" ||
		len(result.Asset.Artifacts) != 1 ||
		result.Asset.Artifacts[0].Bytes != int64(len(corruptBody)) ||
		result.Asset.Artifacts[0].SHA256 == "" {
		t.Fatalf("corrupt inspection diagnostics = %#v", result.Asset)
	}
	body, readErr := os.ReadFile(corruptPath)
	if readErr != nil || string(body) != string(corruptBody) {
		t.Fatalf("integrity inspection mutated corrupt cache: body=%q error=%v", body, readErr)
	}
}

func TestInspectModelAssetsIntegrityCheckReportsMissingArtifact(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeVerifiedCacheFixture(t, cacheDirectory)
	if err := os.Remove(filepath.Join(
		cacheDirectory,
		"OMNIVOICE_Q4_K_M",
		"verified-revision",
		"omnivoice-tokenizer-Q4_K_M.gguf",
	)); err != nil {
		t.Fatalf("remove cached artifact: %v", err)
	}
	scopes := newScopes(t, "missing-inspection")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)

	result, err := service.InspectModelAssets(
		context.Background(),
		models.InspectModelAssetsRequest{
			Scope: ref, Name: "OMNIVOICE_Q4_K_M", VerifyIntegrity: true,
		},
	)
	if !errors.Is(err, models.ErrAssetUnavailable) {
		t.Fatalf("InspectModelAssets error = %v, want ErrAssetUnavailable", err)
	}
	if result.Asset.Readiness != models.AssetReadinessMissing ||
		result.Asset.Integrity != models.AssetIntegrityUnknown ||
		result.Asset.Revision != "verified-revision" ||
		len(result.Asset.Artifacts) != 1 {
		t.Fatalf("missing inspection = %#v", result.Asset)
	}
}

func TestInspectModelAssetsRejectsScopeBeforeCacheEffects(t *testing.T) {
	t.Parallel()

	left := newScopes(t, "left")
	right := newScopes(t, "right")
	config := runtimeConfig("")
	live := openScope(t, left, "cache", config)
	closed := openScope(t, left, "cache", config)
	if err := left.Close(runtimescopes.Reference(closed.String())); err != nil {
		t.Fatalf("close scope: %v", err)
	}
	foreign := openScope(t, right, "cache", config)
	stale, err := (models.RuntimeScopeRef{}).Parse("not-issued")
	if err != nil {
		t.Fatalf("parse stale scope: %v", err)
	}

	var cacheReads int
	service := newTestService(left, &cacheReads)
	tests := []struct {
		name  string
		scope models.RuntimeScopeRef
		want  error
	}{
		{name: "invalid", want: models.ErrRuntimeScopeInvalid},
		{name: "stale", scope: stale, want: models.ErrRuntimeScopeStale},
		{name: "closed", scope: closed, want: models.ErrRuntimeScopeClosed},
		{name: "foreign", scope: foreign, want: models.ErrRuntimeScopeForeign},
	}
	for _, test := range tests {
		_, err := service.InspectModelAssets(context.Background(), models.InspectModelAssetsRequest{
			Scope: test.scope,
			Name:  "OMNIVOICE_Q4_K_M",
		})
		if !errors.Is(err, test.want) {
			t.Fatalf("%s error = %v, want %v", test.name, err, test.want)
		}
	}
	if cacheReads != 0 {
		t.Fatalf("invalid scope operations performed %d cache effects", cacheReads)
	}

	if _, err := service.InspectModelAssets(context.Background(), models.InspectModelAssetsRequest{
		Scope: live,
		Name:  "OMNIVOICE_Q4_K_M",
	}); !errors.Is(err, models.ErrAssetUnavailable) {
		t.Fatalf("live scope error = %v, want ErrAssetUnavailable", err)
	}
	if cacheReads == 0 {
		t.Fatal("live scope did not reach cache inspection")
	}
}

func TestInspectModelAssetsClassifiesMissingAndUnsupportedSourcesBeforeCacheEffects(t *testing.T) {
	t.Parallel()

	scopes := newScopes(t, "sources")
	missing := openScope(t, scopes, "cache", models.RuntimeConfig{})
	unsupported := openScope(t, scopes, "cache", runtimeConfig("custom-provider"))
	unsupportedModel := openScope(t, scopes, "cache", models.RuntimeConfig{
		Resources: []models.RuntimeResource{{
			Type: models.RuntimeResourceTypeModel, Model: "OTHER_MODEL",
		}},
	})
	var cacheReads int
	service := newTestService(scopes, &cacheReads)

	tests := []struct {
		name  string
		scope models.RuntimeScopeRef
		model string
		want  error
	}{
		{name: "missing", scope: missing, model: "OMNIVOICE_Q4_K_M", want: models.ErrAssetSourceMissing},
		{name: "provider", scope: unsupported, model: "OMNIVOICE_Q4_K_M", want: models.ErrAssetSourceUnsupported},
		{name: "model", scope: unsupportedModel, model: "OTHER_MODEL", want: models.ErrAssetSourceUnsupported},
	}
	for _, test := range tests {
		_, err := service.InspectModelAssets(context.Background(), models.InspectModelAssetsRequest{
			Scope: test.scope,
			Name:  test.model,
		})
		if !errors.Is(err, test.want) {
			t.Fatalf("%s error = %v, want %v", test.name, err, test.want)
		}
	}
	if cacheReads != 0 {
		t.Fatalf("source failures performed %d cache effects", cacheReads)
	}
}

func newScopes(t *testing.T, issuer string) runtimescopes.Service {
	t.Helper()
	scopes, err := runtimescopeswire.NewService(func() string { return issuer })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return scopes
}

func openScope(
	t *testing.T,
	scopes runtimescopes.Service,
	cacheDirectory string,
	config models.RuntimeConfig,
) models.RuntimeScopeRef {
	t.Helper()
	ref, err := scopes.Open(models.RuntimeBinding{
		CacheDirectory: cacheDirectory,
		RuntimeConfig:  func() *models.RuntimeConfig { return &config },
	})
	if err != nil {
		t.Fatalf("open scope: %v", err)
	}
	parsed, err := (models.RuntimeScopeRef{}).Parse(string(ref))
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	return parsed
}

func runtimeConfig(provider string) models.RuntimeConfig {
	return models.RuntimeConfig{
		Resources: []models.RuntimeResource{{
			Name:     "omnivoice-cache",
			Type:     models.RuntimeResourceTypeModel,
			Model:    "OMNIVOICE_Q4_K_M",
			Provider: provider,
		}},
	}
}

func newTestService(scopes runtimescopes.Service, cacheReads *int) *service {
	record := func() {
		if cacheReads != nil {
			*cacheReads = *cacheReads + 1
		}
	}
	return New(
		scopes,
		models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		http.DefaultClient,
		models.RuntimeAssetEndpoints{
			BaseURL: "https://assets.example.test", APIBaseURL: "https://api.example.test",
		},
		os.MkdirAll,
		func(path string) (os.FileInfo, error) {
			record()
			return os.Stat(path)
		},
		func() (string, error) {
			record()
			return os.UserHomeDir()
		},
		os.WriteFile,
		os.Rename,
		os.Remove,
		func(path string) ([]byte, error) {
			record()
			return os.ReadFile(path)
		},
		func(path string) ([]os.DirEntry, error) {
			record()
			return os.ReadDir(path)
		},
		func(path string) (io.WriteCloser, error) { return os.Create(path) },
		func(path string) (io.ReadCloser, error) { return os.Open(path) },
		assets.ConstructionOptions{Coordination: mustServiceTestLockingService()},
	).(*service)
}

func mustServiceTestLockingService() platformlocking.Service {
	service, err := platformlocking.New(platformlocking.LocalFileSystem{})
	if err != nil {
		panic(err)
	}
	return service
}

func writeCacheFixture(t *testing.T, cacheDirectory string, includeMetadata bool) {
	t.Helper()
	root := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	revisionDirectory := filepath.Join(root, "rev-test")
	if err := os.MkdirAll(revisionDirectory, 0o755); err != nil {
		t.Fatalf("create cache fixture: %v", err)
	}
	files := []metadataFile{
		{Path: "omnivoice-base-Q4_K_M.gguf", SHA256: "aaa"},
		{Path: "omnivoice-tokenizer-Q4_K_M.gguf", SHA256: "bbb"},
	}
	for index, file := range files {
		content := []byte{byte(index + 1), byte(index + 2)}
		if err := os.WriteFile(filepath.Join(revisionDirectory, file.Path), content, 0o644); err != nil {
			t.Fatalf("write cache artifact: %v", err)
		}
	}
	if !includeMetadata {
		return
	}
	body, err := json.Marshal(cacheMetadata{
		ModelName: "OMNIVOICE_Q4_K_M",
		Revision:  "rev-test",
		Files:     files,
	})
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, metadataFileName), body, 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
}

func assertAvailableSnapshot(t *testing.T, snapshot models.AssetSnapshot, provider string) {
	t.Helper()
	if snapshot.ModelName != "OMNIVOICE_Q4_K_M" ||
		snapshot.Readiness != models.AssetReadinessAvailable ||
		snapshot.Integrity != models.AssetIntegrityUnknown ||
		snapshot.Source.Provider != provider ||
		snapshot.Source.Reference != "Serveurperso/OmniVoice-GGUF" ||
		snapshot.Source.Revision != "rev-test" ||
		snapshot.Revision != "rev-test" {
		t.Fatalf("snapshot = %#v, want available scoped cache facts", snapshot)
	}
	if len(snapshot.Artifacts) != 2 ||
		snapshot.Artifacts[0].Name != "omnivoice-base-Q4_K_M.gguf" ||
		snapshot.TotalBytes != 4 {
		t.Fatalf("artifact facts = %#v total=%d", snapshot.Artifacts, snapshot.TotalBytes)
	}
}
