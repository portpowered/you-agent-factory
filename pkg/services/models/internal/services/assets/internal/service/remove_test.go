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
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestRemoveModelAssetsRemovesSelectedRevisionAndPreservesSiblings(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	sibling := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M", "rev-sibling")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatalf("create sibling revision: %v", err)
	}
	siblingFile := filepath.Join(sibling, "sibling.bin")
	if err := os.WriteFile(siblingFile, []byte("sibling"), 0o644); err != nil {
		t.Fatalf("write sibling revision: %v", err)
	}
	nestedFile := filepath.Join(
		cacheDirectory, "OMNIVOICE_Q4_K_M", "rev-test", "nested", "empty-marker",
	)
	if err := os.MkdirAll(filepath.Dir(nestedFile), 0o755); err != nil {
		t.Fatalf("create nested revision directory: %v", err)
	}
	if err := os.WriteFile(nestedFile, nil, 0o644); err != nil {
		t.Fatalf("write nested revision marker: %v", err)
	}

	scopes := newScopes(t, "remove-success")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)
	result, err := service.RemoveModelAssets(context.Background(), models.RemoveModelAssetsRequest{
		Scope: ref,
		Name:  "omnivoice_q4_k_m",
	})
	if err != nil {
		t.Fatalf("RemoveModelAssets: %v", err)
	}
	if result.ModelName != "OMNIVOICE_Q4_K_M" ||
		result.Revision != "rev-test" ||
		result.CachePath != filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M", "rev-test") ||
		result.BytesRemoved != 4 ||
		result.Readiness != models.AssetReadinessMissing ||
		result.Outcome != models.AssetRemovalRemoved {
		t.Fatalf("RemoveModelAssets result = %#v", result)
	}
	if _, err := os.Stat(result.CachePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed revision stat error = %v, want not-exist", err)
	}
	if body, err := os.ReadFile(siblingFile); err != nil || string(body) != "sibling" {
		t.Fatalf("sibling revision changed: body=%q error=%v", body, err)
	}
}

func TestRemoveModelAssetsReportsMissingCacheWithoutFilesystemMutation(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "remove-missing")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)

	_, err := service.RemoveModelAssets(context.Background(), models.RemoveModelAssetsRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if !errors.Is(err, models.ErrModelCacheNotFound) {
		t.Fatalf("RemoveModelAssets error = %v, want ErrModelCacheNotFound", err)
	}
	entries, readErr := os.ReadDir(cacheDirectory)
	if readErr != nil {
		t.Fatalf("read cache root after missing removal: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("missing removal created or removed cache entries: %#v", entries)
	}
}

func TestRemoveModelAssetsRejectsModelDirectorySymlink(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	externalDirectory := t.TempDir()
	writeCacheFixture(t, externalDirectory, true)
	modelPath := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	if err := os.Symlink(filepath.Join(externalDirectory, "OMNIVOICE_Q4_K_M"), modelPath); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation is unavailable: %v", err)
		}
		t.Fatalf("create model cache symlink: %v", err)
	}

	scopes := newScopes(t, "remove-model-link")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)
	_, err := service.RemoveModelAssets(context.Background(), models.RemoveModelAssetsRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if !errors.Is(err, models.ErrModelCacheUnsafe) {
		t.Fatalf("RemoveModelAssets error = %v, want ErrModelCacheUnsafe", err)
	}
	if _, err := os.Stat(filepath.Join(externalDirectory, "OMNIVOICE_Q4_K_M", "rev-test")); err != nil {
		t.Fatalf("external revision changed after rejected removal: %v", err)
	}
}

func TestRemoveModelAssetsUnlinksNestedSymlinksWithoutFollowingThem(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	externalFile := filepath.Join(t.TempDir(), "outside.bin")
	if err := os.WriteFile(externalFile, []byte("outside"), 0o644); err != nil {
		t.Fatalf("write external file: %v", err)
	}
	linkPath := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M", "rev-test", "outside-link")
	if err := os.Symlink(externalFile, linkPath); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation is unavailable: %v", err)
		}
		t.Fatalf("create nested cache symlink: %v", err)
	}

	scopes := newScopes(t, "remove-nested-link")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)
	if _, err := service.RemoveModelAssets(context.Background(), models.RemoveModelAssetsRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	}); err != nil {
		t.Fatalf("RemoveModelAssets: %v", err)
	}
	if body, err := os.ReadFile(externalFile); err != nil || string(body) != "outside" {
		t.Fatalf("external symlink target changed: body=%q error=%v", body, err)
	}
}

func TestRemoveModelAssetsHonorsCancellationAndRemovalFailure(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "remove-cancel")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newTestService(scopes, nil)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.RemoveModelAssets(cancelled, models.RemoveModelAssetsRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	}); !errors.Is(err, models.ErrAssetCancelled) {
		t.Fatalf("cancelled RemoveModelAssets error = %v, want ErrAssetCancelled", err)
	}

	failedService := newTestService(scopes, nil)
	failedService.removePath = func(path string) error {
		if strings.HasSuffix(path, "omnivoice-tokenizer-Q4_K_M.gguf") {
			return errors.New("injected remove failure")
		}
		return os.Remove(path)
	}
	if _, err := failedService.RemoveModelAssets(context.Background(), models.RemoveModelAssetsRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	}); !errors.Is(err, models.ErrModelCacheRemovalFailed) {
		t.Fatalf("failed RemoveModelAssets error = %v, want ErrModelCacheRemovalFailed", err)
	}
}

func TestPrepareGenericAssetsRepairsLegacyMetadataWithoutTransferOrDeletion(t *testing.T) {
	t.Parallel()

	body := []byte("legacy generic cache payload")
	digest := sha256Hex(body)
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
		Artifacts: []models.AssetRequirement{{Name: "weights.bin", Bytes: int64(len(body)), SHA256: digest}},
	}

	result, err := service.PrepareModelAssets(context.Background(), request)
	assertLegacyRepairResult(t, result, err, body, digest)
	if sourceRequests.Load() != 0 {
		t.Fatalf("legacy repair source requests = %d, want zero", sourceRequests.Load())
	}
	assertLegacyRepairSnapshot(t, legacyPath, correctedPath, source, request, body)

	service.client = httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline repaired reuse must not contact the source")
	})
	reused, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: scope, Reference: request.Reference, Offline: true,
	})
	assertLegacyRepairReuse(t, reused, err, body)
}

func assertLegacyRepairResult(
	t *testing.T,
	result models.PrepareModelAssetsResult,
	err error,
	body []byte,
	digest string,
) {
	t.Helper()
	if err != nil {
		t.Fatalf("legacy repair: %v", err)
	}
	if result.Outcome != models.AssetPreparationAlreadyAvailable || result.Asset.Integrity != models.AssetIntegrityVerified ||
		len(result.Asset.Artifacts) != 1 || result.Asset.Artifacts[0].Bytes != int64(len(body)) ||
		result.Asset.Artifacts[0].SHA256 != digest {
		t.Fatalf("legacy repair result = %#v, want verified cache hit", result)
	}
}

func assertLegacyRepairSnapshot(
	t *testing.T,
	legacyPath string,
	correctedPath string,
	source genericSource,
	request models.PrepareModelAssetsRequest,
	body []byte,
) {
	t.Helper()
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
	wantIdentity := genericCacheKey(assetKindModel, source, []genericArtifact{{requirement: request.Artifacts[0]}})
	if metadata.Identity != wantIdentity || len(metadata.Artifacts) != 1 || metadata.Artifacts[0] != request.Artifacts[0] {
		t.Fatalf("repaired metadata = %#v, want observed requirement and identity", metadata)
	}
	for _, suffix := range []string{".partial", ".previous"} {
		if _, err := os.Stat(correctedPath + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("repair transient %q = %v, want absent", suffix, err)
		}
	}
}

func assertLegacyRepairReuse(t *testing.T, result models.PrepareModelAssetsResult, err error, body []byte) {
	t.Helper()
	if err != nil || result.Outcome != models.AssetPreparationAlreadyAvailable ||
		result.Asset.Integrity != models.AssetIntegrityVerified || result.Asset.TotalBytes != int64(len(body)) {
		t.Fatalf("offline repaired reuse = %#v, %v", result, err)
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
