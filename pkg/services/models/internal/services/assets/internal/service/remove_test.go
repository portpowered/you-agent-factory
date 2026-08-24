package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
