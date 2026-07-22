package assets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestPullErrorPreservesResultAndCause(t *testing.T) {
	cause := errors.Join(context.DeadlineExceeded, ErrSourceFetchFailed)
	err := &PullError{
		Result: PullResult{
			ModelName:          "voice-model",
			ManagedPullOutcome: "TIMED_OUT",
			ReadinessState:     "FAILED",
		},
		Cause: cause,
	}

	var got *PullError
	if !errors.As(err, &got) || got != err {
		t.Fatalf("errors.As() = %v, want original error", got)
	}
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrSourceFetchFailed) {
		t.Fatalf("PullError does not preserve cause: %v", err)
	}
	want := `managed runtime pull for "voice-model" failed with outcome TIMED_OUT (readiness FAILED)`
	if err.Error() != want {
		t.Fatalf("PullError.Error() = %q, want %q", err.Error(), want)
	}
}

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

	puller := newModelAssetPullerForTest(t, t.TempDir())
	puller.SetEndpointsForTest(server.URL, server.URL+"/api", server.Client())

	runtimeCfg := mustLoadedFactoryConfigForModelPullTest(t, &testFactoryConfig{
		Resources: []modelRuntimeResource{{
			Name:       "omnivoice-cache",
			Type:       models.RuntimeResourceTypeModel,
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
	if result.Outcome != PullOutcomePulled || result.Revision != "rev-test" {
		t.Fatalf("result = %#v, want pulled rev-test", result)
	}
	for _, path := range []string{
		filepath.Join(result.CachePath, "omnivoice-base-Q4_K_M.gguf"),
		filepath.Join(result.CachePath, "omnivoice-tokenizer-Q4_K_M.gguf"),
	} {
		if _, err := testFileSHA256(path); err != nil {
			t.Fatalf("expected cached file %q: %v", path, err)
		}
	}
	if err := puller.EnsureModelAvailable(context.Background(), runtimeCfg, &modelRuntimeWorker{
		Model:         "OMNIVOICE_Q4_K_M",
		ModelLocality: models.RuntimeModelLocalityLocal,
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
	puller := newModelAssetPullerForTest(t, cacheDir)
	puller.SetEndpointsForTest(server.URL, server.URL+"/api", server.Client())

	runtimeCfg := mustLoadedFactoryConfigForModelPullTest(t, &testFactoryConfig{
		Resources: []modelRuntimeResource{{
			Name:       "omnivoice-cache",
			Type:       models.RuntimeResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
	})
	worker := &modelRuntimeWorker{
		Model:         "OMNIVOICE_Q4_K_M",
		ModelLocality: models.RuntimeModelLocalityLocal,
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

func TestPullModel_RetriesManifestLookupAfterDNSError(t *testing.T) {
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

	puller := newModelAssetPullerForTest(t, t.TempDir())
	puller.SetEndpointsForTest(server.URL, server.URL+"/api", &http.Client{
		Transport: &manifestRetryRoundTripper{
			base: server.Client().Transport,
		},
	})

	runtimeCfg := mustLoadedFactoryConfigForModelPullTest(t, &testFactoryConfig{
		Resources: []modelRuntimeResource{{
			Name:       "omnivoice-cache",
			Type:       models.RuntimeResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
	})

	result, err := puller.PullModel(context.Background(), runtimeCfg, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("PullModel with manifest retry: %v", err)
	}
	if result.Revision != "rev-test" || len(result.DownloadedFiles) != 2 {
		t.Fatalf("result = %#v, want rev-test with both managed files", result)
	}
}

func TestPullModel_ReturnsUnsupportedWhenRuntimeHasNoMatchingModelResource(t *testing.T) {
	puller := newModelAssetPullerForTest(t, t.TempDir())
	runtimeCfg := mustLoadedFactoryConfigForModelPullTest(t, &testFactoryConfig{})
	_, err := puller.PullModel(context.Background(), runtimeCfg, "OMNIVOICE_Q4_K_M")
	if err == nil || !strings.Contains(err.Error(), ErrPullUnsupported.Error()) {
		t.Fatalf("PullModel error = %v, want unsupported", err)
	}
}

func mustLoadedFactoryConfigForModelPullTest(t *testing.T, cfg *testFactoryConfig) *modelRuntimeConfig {
	t.Helper()
	return projectTestModelsRuntimeConfig("factory-dir", cfg)
}

func newModelAssetPullerForTest(t *testing.T, cacheDir string) *Puller {
	t.Helper()
	puller, err := NewPuller(
		cacheDir, "linux", "amd64", http.DefaultClient, DefaultEndpoints(),
		os.MkdirAll, os.Stat, os.UserHomeDir, os.WriteFile, os.Rename, os.Remove,
		os.ReadFile, os.ReadDir,
		func(path string) (io.WriteCloser, error) { return os.Create(path) },
		func(path string) (io.ReadCloser, error) { return os.Open(path) },
	)
	if err != nil {
		t.Fatalf("NewPuller: %v", err)
	}
	return puller
}

func sha256HexString(input []byte) string {
	sum := sha256.Sum256(input)
	return hex.EncodeToString(sum[:])
}

func testFileSHA256(path string) (string, error) {
	input, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer input.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, input); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

type manifestRetryRoundTripper struct {
	base http.RoundTripper
	mu   sync.Mutex
	hit  bool
}

func (rt *manifestRetryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	shouldFail := !rt.hit && req != nil && req.URL != nil && req.URL.Path == "/api/models/Serveurperso/OmniVoice-GGUF"
	if shouldFail {
		rt.hit = true
	}
	rt.mu.Unlock()
	if shouldFail {
		return nil, &net.DNSError{Err: "lookup huggingface.co: no such host", Name: "huggingface.co", IsNotFound: true}
	}
	return rt.base.RoundTrip(req)
}
