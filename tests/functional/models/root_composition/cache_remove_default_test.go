package root_composition_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestModelsPublicRemoveWorkflowDefaultsToRevisionOnlyForASR(t *testing.T) {
	t.Parallel()

	home := functionalTempDir(t)
	cacheRoot := filepath.Join(home, "managed-model-cache")
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	fixture := writeDefaultASRRemovalFixture(t, cacheRoot)
	process := functionalSharedDefaultProcess(t)
	environment := isolatedModelEnvironment(home, cacheRoot)
	inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "models", "remove", models.BuiltInModelNameASR})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(default ASR remove) error = %v\nstdout=%s\nstderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var removed factoryapi.ModelRemoveResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &removed); err != nil {
		t.Fatalf("decode default ASR remove output: %v\nstdout=%s", err, inputs.Stdout())
	}
	if removed.ModelName != "ASR" || removed.Revision != fixture.revision ||
		removed.CachePath != fixture.revisionPath || removed.BytesRemoved != fixture.revisionBytes ||
		removed.Outcome != factoryapi.REMOVED {
		t.Fatalf("default ASR remove response = %#v, want revision-only removal of %d bytes", removed, fixture.revisionBytes)
	}
	assertDefaultASRRemovalEffects(t, fixture)

	missingInputs := support.FakeInputs(
		t.Context(), []string{"you", "--json", "models", "remove", models.BuiltInModelNameASR},
	)
	missingInputs.Input.Env = environment
	missingInputs.Input.WorkingDirectory = factoryDir
	if err := process.Execute(missingInputs.Input); err == nil || !errors.Is(err, modelscli.ErrModelCacheNotFound) {
		t.Fatalf("repeated default ASR remove error = %v, want ErrModelCacheNotFound", err)
	}
	diagnostic := decodeFirstDiagnostic(t, missingInputs.Stderr())
	if diagnostic.Code != factoryapi.ErrorResponseCodeMODELCACHENOTFOUND ||
		diagnostic.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("repeated default ASR remove diagnostic = %#v, want MODEL_CACHE_NOT_FOUND/NOT_FOUND", diagnostic)
	}
	assertDefaultASRRemovalEffects(t, fixture)
}

type defaultASRRemovalFixture struct {
	revision      string
	revisionPath  string
	revisionBytes int64
	siblingPath   string
	modelCASPath  string
	backendPath   string
}

func writeDefaultASRRemovalFixture(t *testing.T, cacheRoot string) defaultASRRemovalFixture {
	t.Helper()
	definition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameASR)
	if !ok {
		t.Fatal("built-in catalog did not publish ASR")
	}
	sourceParts := strings.SplitN(definition.Source, "@", 2)
	if len(sourceParts) != 2 || strings.TrimSpace(sourceParts[1]) == "" {
		t.Fatalf("built-in ASR source %q has no pinned revision", definition.Source)
	}
	assetName := path.Base(sourceParts[0])
	revision := sourceParts[1]
	body := []byte("controlled installed ASR model bytes")
	modelRoot := filepath.Join(cacheRoot, "ASR")
	revisionPath := filepath.Join(modelRoot, revision)
	writeDefaultASRFile(t, filepath.Join(revisionPath, assetName), body)
	siblingPath := filepath.Join(modelRoot, "sibling-revision", "sibling.bin")
	writeDefaultASRFile(t, siblingPath, []byte("another managed revision"))
	modelCASPath := writeDefaultASRModelSnapshot(t, cacheRoot, definition.Source, assetName, body)
	selection := pinnedASRBackendSelection()
	backendBody := []byte("controlled ASR backend snapshot")
	selection.Bytes = int64(len(backendBody))
	selection.SHA256 = fmt.Sprintf("%x", sha256.Sum256(backendBody))
	writeCacheSelectionBackendFixture(t, cacheRoot, definition.Backend, selection, backendBody)
	backendRoot := defaultASRBackendCachePath(cacheRoot, definition.Backend, selection)
	backendPath := filepath.Join(backendRoot, selection.Name)
	backendRelative, err := filepath.Rel(cacheRoot, backendRoot)
	if err != nil {
		t.Fatalf("relative ASR backend cache path: %v", err)
	}
	files := []map[string]any{{
		"path": assetName, "bytes": len(body), "sha256": fmt.Sprintf("%x", sha256.Sum256(body)),
	}}
	backendFiles := []map[string]any{{
		"path": selection.Name, "bytes": len(backendBody), "sha256": selection.SHA256,
	}}
	metadata := map[string]any{
		"modelName": definition.Name,
		"revision":  revision,
		"files":     files,
		"backend": map[string]any{
			"cachePath": filepath.ToSlash(backendRelative), "revision": "fixture-backend", "files": backendFiles,
		},
	}
	writeCacheSelectionMetadata(t, filepath.Join(modelRoot, ".managed-cache.json"), metadata)
	return defaultASRRemovalFixture{
		revision: revision, revisionPath: revisionPath,
		revisionBytes: story003RegularFileBytes(t, revisionPath),
		siblingPath:   siblingPath, modelCASPath: modelCASPath, backendPath: backendPath,
	}
}

func writeDefaultASRModelSnapshot(t *testing.T, cacheRoot, source, name string, body []byte) string {
	t.Helper()
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	identity := fmt.Sprintf("model|%s|%s:%d:%s", source, name, len(body), digest)
	identityHash := sha256.Sum256([]byte(identity))
	snapshot := filepath.Join(cacheRoot, ".you-content-addressed", "model", hex.EncodeToString(identityHash[:]))
	filePath := filepath.Join(snapshot, name)
	writeDefaultASRFile(t, filePath, body)
	writeCacheSelectionMetadata(t, filepath.Join(snapshot, ".you-assets.json"), map[string]any{
		"kind": "model", "identity": identity, "source": source, "sourceKey": source,
		"artifacts": []map[string]any{{"Name": name, "Bytes": len(body), "SHA256": digest}},
	})
	return filePath
}

func defaultASRBackendCachePath(
	cacheRoot, backend string,
	selection serviceedges.ModelBackendArtifactSelection,
) string {
	urlHash := fmt.Sprintf("%x", sha256.Sum256([]byte(selection.Location)))
	source := fmt.Sprintf("backend://%s/release://%s", backend, urlHash)
	identity := fmt.Sprintf("backend|%s|%s:%d:%s", source, selection.Name, selection.Bytes, selection.SHA256)
	identityHash := sha256.Sum256([]byte(identity))
	return filepath.Join(cacheRoot, "backend-artifacts", ".you-content-addressed", "backend", hex.EncodeToString(identityHash[:]))
}

func writeDefaultASRFile(t *testing.T, filePath string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("create ASR cache fixture directory: %v", err)
	}
	if err := os.WriteFile(filePath, body, 0o644); err != nil {
		t.Fatalf("write ASR cache fixture %q: %v", filePath, err)
	}
}

func assertDefaultASRRemovalEffects(t *testing.T, fixture defaultASRRemovalFixture) {
	t.Helper()
	if _, err := os.Stat(fixture.revisionPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed ASR revision stat error = %v, want not-exist", err)
	}
	if body, err := os.ReadFile(fixture.siblingPath); err != nil || string(body) != "another managed revision" {
		t.Fatalf("sibling ASR revision changed: body=%q error=%v", body, err)
	}
	assertDefaultASRRemovalSharedAssets(t, fixture)
}

func assertDefaultASRRemovalSharedAssets(t *testing.T, fixture defaultASRRemovalFixture) {
	t.Helper()
	for _, asset := range []struct{ path, body string }{
		{path: fixture.modelCASPath, body: "controlled installed ASR model bytes"},
		{path: fixture.backendPath, body: "controlled ASR backend snapshot"},
	} {
		body, err := os.ReadFile(asset.path)
		if err != nil || string(body) != asset.body {
			t.Fatalf("ASR shared snapshot changed: path=%q body=%q error=%v", asset.path, body, err)
		}
	}
}
