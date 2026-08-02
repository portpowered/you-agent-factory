//go:build windows

package wire

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestProductionModelsRemovalPropagatesPartialCancellationAndRetry(t *testing.T) {
	t.Parallel()

	service := newProductionTestService(t)
	cacheDirectory := t.TempDir()
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{CacheDirectory: cacheDirectory},
	})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	target := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create model cache: %v", err)
	}
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(target, name), []byte(name), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	removalContext := &productionRemovalCancelContext{cancelOn: 12}
	_, err = service.RemoveModelAssets(removalContext, models.RemoveModelAssetsRequest{
		Scope: opened.Scope,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if !errors.Is(err, models.ErrAssetCancelled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("partial removal error = %v, want typed cancellation", err)
	}
	if _, err := os.Stat(filepath.Join(target, "a")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("first asset = %v, want removed before cancellation (context calls=%d)", err, removalContext.calls)
	}
	if _, err := os.Stat(filepath.Join(target, "b")); err != nil {
		t.Fatalf("second asset = %v, want remaining retry state", err)
	}

	result, err := service.RemoveModelAssets(context.Background(), models.RemoveModelAssetsRequest{
		Scope: opened.Scope,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil || result.Outcome != models.AssetRemovalRemoved {
		t.Fatalf("retry removal = %#v, err %v; want removed", result, err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target after retry = %v, want absent", err)
	}
}

func TestProductionModelsRemovalNormalizesDotRelativeCacheDirectory(t *testing.T) {
	t.Chdir(t.TempDir())

	service := newProductionTestService(t)
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{CacheDirectory: `.\managed-model-cache`},
	})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	target := filepath.Join("managed-model-cache", "OMNIVOICE_Q4_K_M")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create model cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "asset"), []byte("remove"), 0o644); err != nil {
		t.Fatalf("write model asset: %v", err)
	}

	result, err := service.RemoveModelAssets(context.Background(), models.RemoveModelAssetsRequest{
		Scope: opened.Scope,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil || result.Outcome != models.AssetRemovalRemoved {
		t.Fatalf("relative-cache removal = %#v, err %v; want removed", result, err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("relative-cache target = %v, want absent", err)
	}
}

func TestProductionModelsRemovalReportsLateCancellationAfterCommitAndRetries(t *testing.T) {
	t.Parallel()

	service := newProductionTestService(t)
	cacheDirectory := t.TempDir()
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{CacheDirectory: cacheDirectory},
	})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	target := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create model cache: %v", err)
	}

	removalContext := &productionRemovalCancelContext{cancelOn: 8}
	_, err = service.RemoveModelAssets(removalContext, models.RemoveModelAssetsRequest{
		Scope: opened.Scope,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if !errors.Is(err, models.ErrAssetCancelled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("late removal error = %v, want typed cancellation", err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("late-cancel target = %v, want absent (context calls=%d)", err, removalContext.calls)
	}

	result, err := service.RemoveModelAssets(context.Background(), models.RemoveModelAssetsRequest{
		Scope: opened.Scope,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil || result.Outcome != models.AssetRemovalAlreadyAbsent {
		t.Fatalf("late-cancel retry = %#v, err %v; want already absent", result, err)
	}
}

type productionRemovalCancelContext struct {
	calls    int
	cancelOn int
}

func (ctx *productionRemovalCancelContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (ctx *productionRemovalCancelContext) Done() <-chan struct{}       { return nil }
func (ctx *productionRemovalCancelContext) Value(any) any               { return nil }
func (ctx *productionRemovalCancelContext) Err() error {
	ctx.calls++
	if ctx.calls >= ctx.cancelOn {
		return context.Canceled
	}
	return nil
}
