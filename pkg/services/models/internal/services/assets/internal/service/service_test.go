package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
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
	valid := openScope(t, scopes, "cache", runtimeConfig(""))
	unsupportedModel := openScope(t, scopes, "cache", models.RuntimeConfig{
		Resources: []models.RuntimeResource{{
			Type: models.RuntimeResourceTypeModel, Model: "OTHER_MODEL",
		}},
	})
	var cacheReads int
	service := newTestService(scopes, &cacheReads)

	tests := []struct {
		name            string
		scope           models.RuntimeScopeRef
		model           string
		verifyIntegrity bool
		want            error
	}{
		{name: "missing", scope: missing, model: "OMNIVOICE_Q4_K_M", want: models.ErrAssetSourceMissing},
		{name: "provider", scope: unsupported, model: "OMNIVOICE_Q4_K_M", want: models.ErrAssetSourceUnsupported},
		{name: "model", scope: unsupportedModel, model: "OTHER_MODEL", want: models.ErrAssetSourceUnsupported},
		{
			name: "integrity verification", scope: valid, model: "OMNIVOICE_Q4_K_M",
			verifyIntegrity: true, want: models.ErrUnsupportedOperation,
		},
	}
	for _, test := range tests {
		_, err := service.InspectModelAssets(context.Background(), models.InspectModelAssetsRequest{
			Scope:           test.scope,
			Name:            test.model,
			VerifyIntegrity: test.verifyIntegrity,
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
	).(*service)
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
