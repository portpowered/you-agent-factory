package service

import (
	"context"
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
