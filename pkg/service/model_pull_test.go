package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestPullModel_DownloadsManagedCacheAssets(t *testing.T) {
	baseBytes := []byte("base-gguf")
	tokenizerBytes := []byte("tokenizer-gguf")
	baseSHA := sha256HexString(baseBytes)
	tokenizerSHA := sha256HexString(tokenizerBytes)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/models/Serveurperso/OmniVoice-GGUF":
			_, _ = io.WriteString(w, fmt.Sprintf(`{"sha":"rev-test","siblings":[{"rfilename":"omnivoice-base-Q4_K_M.gguf","size":%d,"lfs":{"oid":"%s","size":%d}},{"rfilename":"omnivoice-tokenizer-Q4_K_M.gguf","size":%d,"lfs":{"oid":"%s","size":%d}}]}`, len(baseBytes), baseSHA, len(baseBytes), len(tokenizerBytes), tokenizerSHA, len(tokenizerBytes)))
		case "/Serveurperso/OmniVoice-GGUF/resolve/rev-test/omnivoice-base-Q4_K_M.gguf":
			_, _ = w.Write(baseBytes)
		case "/Serveurperso/OmniVoice-GGUF/resolve/rev-test/omnivoice-tokenizer-Q4_K_M.gguf":
			_, _ = w.Write(tokenizerBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	puller := newHuggingFaceModelAssetPuller(t.TempDir())
	puller.baseURL = server.URL
	puller.apiBaseURL = server.URL + "/api"
	puller.client = server.Client()

	runtimeCfg := mustLoadedFactoryConfigForModelCatalogTest(t, &interfaces.FactoryConfig{
		Resources: []interfaces.ResourceConfig{{
			Name:       "omnivoice-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
	})

	result, err := puller.PullModel(context.Background(), runtimeCfg, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	if result.Outcome != modelPullOutcomePulled || result.Revision != "rev-test" {
		t.Fatalf("result = %#v, want pulled rev-test", result)
	}
	for _, path := range []string{
		filepath.Join(result.CachePath, "omnivoice-base-Q4_K_M.gguf"),
		filepath.Join(result.CachePath, "omnivoice-tokenizer-Q4_K_M.gguf"),
	} {
		if _, err := fileSHA256(path); err != nil {
			t.Fatalf("expected cached file %q: %v", path, err)
		}
	}
	if err := puller.EnsureModelAvailable(context.Background(), runtimeCfg, &interfaces.WorkerConfig{
		Model:         "OMNIVOICE_Q4_K_M",
		ModelLocality: interfaces.ModelLocalityLocal,
	}); err != nil {
		t.Fatalf("EnsureModelAvailable: %v", err)
	}
}

func TestPullModel_ResolveModelCacheUsesPersistedMetadataOffline(t *testing.T) {
	baseBytes := []byte("base-gguf")
	tokenizerBytes := []byte("tokenizer-gguf")
	baseSHA := sha256HexString(baseBytes)
	tokenizerSHA := sha256HexString(tokenizerBytes)
	manifestRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/models/Serveurperso/OmniVoice-GGUF":
			manifestRequests++
			_, _ = io.WriteString(w, fmt.Sprintf(`{"sha":"rev-test","siblings":[{"rfilename":"omnivoice-base-Q4_K_M.gguf","size":%d,"lfs":{"oid":"%s","size":%d}},{"rfilename":"omnivoice-tokenizer-Q4_K_M.gguf","size":%d,"lfs":{"oid":"%s","size":%d}}]}`, len(baseBytes), baseSHA, len(baseBytes), len(tokenizerBytes), tokenizerSHA, len(tokenizerBytes)))
		case "/Serveurperso/OmniVoice-GGUF/resolve/rev-test/omnivoice-base-Q4_K_M.gguf":
			_, _ = w.Write(baseBytes)
		case "/Serveurperso/OmniVoice-GGUF/resolve/rev-test/omnivoice-tokenizer-Q4_K_M.gguf":
			_, _ = w.Write(tokenizerBytes)
		default:
			http.NotFound(w, r)
		}
	}))

	cacheDir := t.TempDir()
	puller := newHuggingFaceModelAssetPuller(cacheDir)
	puller.baseURL = server.URL
	puller.apiBaseURL = server.URL + "/api"
	puller.client = server.Client()

	runtimeCfg := mustLoadedFactoryConfigForModelCatalogTest(t, &interfaces.FactoryConfig{
		Resources: []interfaces.ResourceConfig{{
			Name:       "omnivoice-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
	})
	worker := &interfaces.WorkerConfig{
		Model:         "OMNIVOICE_Q4_K_M",
		ModelLocality: interfaces.ModelLocalityLocal,
	}

	result, err := puller.PullModel(context.Background(), runtimeCfg, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	server.Close()

	layout, err := puller.ResolveModelCache(context.Background(), runtimeCfg, worker)
	if err != nil {
		t.Fatalf("ResolveModelCache after pull with offline manifest: %v", err)
	}
	if err := puller.EnsureModelAvailable(context.Background(), runtimeCfg, worker); err != nil {
		t.Fatalf("EnsureModelAvailable after pull with offline manifest: %v", err)
	}
	if manifestRequests != 1 {
		t.Fatalf("manifest requests = %d, want 1 during pull only", manifestRequests)
	}
	if layout.CachePath != result.CachePath || layout.Revision != "rev-test" || len(layout.Files) != 2 {
		t.Fatalf("layout = %#v, want pulled cache path and revision", layout)
	}
}

func TestPullModel_ReturnsUnsupportedWhenRuntimeHasNoMatchingModelResource(t *testing.T) {
	puller := newHuggingFaceModelAssetPuller(t.TempDir())
	runtimeCfg := mustLoadedFactoryConfigForModelCatalogTest(t, &interfaces.FactoryConfig{})
	_, err := puller.PullModel(context.Background(), runtimeCfg, "OMNIVOICE_Q4_K_M")
	if err == nil || !strings.Contains(err.Error(), apisurface.ErrModelPullUnsupported.Error()) {
		t.Fatalf("PullModel error = %v, want unsupported", err)
	}
}

func TestInvokeModel_ReturnsModelNotAvailableWhenManagedCacheIsMissing(t *testing.T) {
	puller := newHuggingFaceModelAssetPuller(t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/models/Serveurperso/OmniVoice-GGUF" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"sha":"rev-test","siblings":[{"rfilename":"omnivoice-base-Q4_K_M.gguf","size":10,"lfs":{"oid":"abc","size":10}},{"rfilename":"omnivoice-tokenizer-Q4_K_M.gguf","size":10,"lfs":{"oid":"def","size":10}}]}`)
	}))
	defer server.Close()
	puller.baseURL = server.URL
	puller.apiBaseURL = server.URL + "/api"
	puller.client = server.Client()

	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", &interfaces.FactoryConfig{
		Resources: []interfaces.ResourceConfig{{
			Name:       "omnivoice-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
		Workers: []interfaces.WorkerConfig{{Name: "tts-worker"}},
	}, map[string]*interfaces.WorkerConfig{
		"tts-worker": {
			Name:          "tts-worker",
			Type:          interfaces.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelProvider: interfaces.RunnerIDCodex,
			ModelLocality: interfaces.ModelLocalityLocal,
			Operations: []interfaces.ModelOperation{{
				Name: "TTS",
				Inputs: []interfaces.ModelOperationSlot{{
					Name:         "text",
					ContentTypes: []string{interfaces.ModelOperationContentTypeText},
					Required:     true,
				}},
				Outputs: []interfaces.ModelOperationSlot{{
					Name:         "audio",
					ContentTypes: []string{interfaces.ModelOperationContentTypeAudio},
				}},
			}},
		},
	}, nil)
	svc := &FactoryService{
		runtimeCfg:  runtimeCfg,
		cfg:         &FactoryServiceConfig{},
		modelAssets: puller,
	}
	_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content: &factoryapi.WorkContent{
			mustGeneratedServiceTextPart(t, "hello world"),
		},
	})
	if err == nil || !strings.Contains(err.Error(), apisurface.ErrModelNotAvailable.Error()) {
		t.Fatalf("InvokeModel error = %v, want model not available", err)
	}
}

func sha256HexString(input []byte) string {
	sum := sha256.Sum256(input)
	return hex.EncodeToString(sum[:])
}
