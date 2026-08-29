package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	platformlocking "github.com/portpowered/infinite-you/pkg/platform/locking"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	pullsupport "github.com/portpowered/infinite-you/pkg/services/models/internal/pullsupport"
	assets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

func TestPrepareModelAssetsReusesVerifiedCacheWithoutSourceOrMutation(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeVerifiedCacheFixture(t, cacheDirectory)
	scopes := newScopes(t, "prepare-cache-hit")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	var sourceRequests atomic.Int32
	var mutations atomic.Int32
	service := newPreparationTestService(
		scopes,
		httpDoerFunc(func(*http.Request) (*http.Response, error) {
			sourceRequests.Add(1)
			return nil, fmt.Errorf("source must not be contacted for a verified cache hit")
		}),
		models.RuntimeAssetEndpoints{
			BaseURL: "https://assets.example.test", APIBaseURL: "https://api.example.test",
		},
		&mutations,
	)

	result, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: ref, Name: "omnivoice_q4_k_m",
	})
	if err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	if result.Outcome != models.AssetPreparationAlreadyAvailable ||
		result.Asset.Readiness != models.AssetReadinessAvailable ||
		result.Asset.Integrity != models.AssetIntegrityVerified {
		t.Fatalf("cache-hit result = %#v", result)
	}
	if sourceRequests.Load() != 0 || mutations.Load() != 0 {
		t.Fatalf("cache hit performed source=%d mutation=%d effects", sourceRequests.Load(), mutations.Load())
	}
}

func TestPrepareModelAssetsReconcilesCompleteLegacyCacheWithoutAssetDownload(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	baseBody := []byte("legacy cached base")
	tokenizerBody := []byte("legacy cached tokenizer")
	revision := "legacy-revision"
	root := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	revisionDirectory := filepath.Join(root, revision)
	if err := os.MkdirAll(revisionDirectory, 0o755); err != nil {
		t.Fatalf("create legacy cache: %v", err)
	}
	for name, body := range map[string][]byte{
		"omnivoice-base-Q4_K_M.gguf":      baseBody,
		"omnivoice-tokenizer-Q4_K_M.gguf": tokenizerBody,
	} {
		if err := os.WriteFile(filepath.Join(revisionDirectory, name), body, 0o644); err != nil {
			t.Fatalf("write legacy cache asset %s: %v", name, err)
		}
	}
	staleMetadata, err := json.Marshal(cacheMetadata{
		ModelName: "OMNIVOICE_Q4_K_M",
		Revision:  revision,
		Files: []metadataFile{{
			Path: "omnivoice-base-Q4_K_M.gguf",
		}},
	})
	if err != nil {
		t.Fatalf("marshal stale legacy metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, metadataFileName), staleMetadata, 0o644); err != nil {
		t.Fatalf("write stale legacy metadata: %v", err)
	}

	var manifestRequests atomic.Int32
	var assetRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/models/Serveurperso/OmniVoice-GGUF" {
			assetRequests.Add(1)
			http.Error(writer, "assets must not be downloaded", http.StatusInternalServerError)
			return
		}
		manifestRequests.Add(1)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"sha": revision,
			"siblings": []map[string]any{
				{
					"rfilename": "omnivoice-base-Q4_K_M.gguf",
					"lfs": map[string]any{
						"oid": sha256String(baseBody), "size": len(baseBody),
					},
				},
				{
					"rfilename": "omnivoice-tokenizer-Q4_K_M.gguf",
					"lfs": map[string]any{
						"oid": sha256String(tokenizerBody), "size": len(tokenizerBody),
					},
				},
			},
		})
	}))
	t.Cleanup(server.Close)

	scopes := newScopes(t, "prepare-legacy-cache")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	var mutations atomic.Int32
	service := newPreparationTestService(
		scopes,
		server.Client(),
		models.RuntimeAssetEndpoints{BaseURL: server.URL, APIBaseURL: server.URL},
		&mutations,
	)
	result, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: ref, Name: "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	if result.Outcome != models.AssetPreparationAlreadyAvailable ||
		result.Asset.Integrity != models.AssetIntegrityVerified ||
		result.Asset.Revision != revision ||
		len(result.Asset.Artifacts) != 2 {
		t.Fatalf("legacy cache result = %#v, want verified cache hit", result)
	}
	if manifestRequests.Load() != 1 || assetRequests.Load() != 0 || mutations.Load() != 0 {
		t.Fatalf(
			"legacy cache effects: manifest=%d asset=%d mutation=%d, want 1/0/0",
			manifestRequests.Load(), assetRequests.Load(), mutations.Load(),
		)
	}
}

func TestPrepareModelAssetsPublishesVerifiedPullThenReturnsCacheHit(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	baseBody := []byte("verified base model")
	tokenizerBody := []byte("verified tokenizer")
	baseSHA := sha256String(baseBody)
	tokenizerSHA := sha256String(tokenizerBody)
	var requests atomic.Int32
	server := newSuccessfulAssetServer(baseBody, tokenizerBody, baseSHA, tokenizerSHA, &requests)
	t.Cleanup(server.Close)

	scopes := newScopes(t, "prepare-pull")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	var mutations atomic.Int32
	service := newPreparationTestService(
		scopes,
		server.Client(),
		models.RuntimeAssetEndpoints{BaseURL: server.URL, APIBaseURL: server.URL},
		&mutations,
	)
	request := models.PrepareModelAssetsRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"}
	first, err := service.PrepareModelAssets(context.Background(), request)
	if err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	assertSuccessfulPreparation(t, cacheDirectory, first, baseBody, tokenizerBody, baseSHA)

	requestsAfterPull := requests.Load()
	mutationsAfterPull := mutations.Load()
	first.Asset.Artifacts[0].Name = "peer mutation"
	second, err := service.PrepareModelAssets(context.Background(), request)
	if err != nil {
		t.Fatalf("PrepareModelAssets repeated: %v", err)
	}
	if second.Outcome != models.AssetPreparationAlreadyAvailable ||
		second.Asset.Artifacts[0].Name != "omnivoice-base-Q4_K_M.gguf" {
		t.Fatalf("repeated result = %#v", second)
	}
	if requests.Load() != requestsAfterPull || mutations.Load() != mutationsAfterPull {
		t.Fatalf(
			"repeated preparation changed source/mutation counts: source %d->%d mutation %d->%d",
			requestsAfterPull, requests.Load(), mutationsAfterPull, mutations.Load(),
		)
	}
}

func newSuccessfulAssetServer(
	baseBody []byte,
	tokenizerBody []byte,
	baseSHA string,
	tokenizerSHA string,
	requests *atomic.Int32,
) *httptest.Server {
	var manifestRequests atomic.Int32
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		switch request.URL.Path {
		case "/models/Serveurperso/OmniVoice-GGUF":
			if manifestRequests.Add(1) == 1 {
				http.Error(writer, "retry", http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"sha": "revision-1",
				"siblings": []map[string]any{
					{
						"rfilename": "omnivoice-base-Q4_K_M.gguf",
						"lfs":       map[string]any{"oid": baseSHA, "size": len(baseBody)},
					},
					{
						"rfilename": "omnivoice-tokenizer-Q4_K_M.gguf",
						"lfs":       map[string]any{"oid": tokenizerSHA, "size": len(tokenizerBody)},
					},
				},
			})
		case "/Serveurperso/OmniVoice-GGUF/resolve/revision-1/omnivoice-base-Q4_K_M.gguf":
			_, _ = writer.Write(baseBody)
		case "/Serveurperso/OmniVoice-GGUF/resolve/revision-1/omnivoice-tokenizer-Q4_K_M.gguf":
			_, _ = writer.Write(tokenizerBody)
		default:
			http.NotFound(writer, request)
		}
	}))
}

func assertSuccessfulPreparation(
	t *testing.T,
	cacheDirectory string,
	result models.PrepareModelAssetsResult,
	baseBody []byte,
	tokenizerBody []byte,
	baseSHA string,
) {
	t.Helper()
	if result.Outcome != models.AssetPreparationPrepared ||
		result.Asset.Readiness != models.AssetReadinessAvailable ||
		result.Asset.Integrity != models.AssetIntegrityVerified ||
		result.Asset.Revision != "revision-1" ||
		result.Asset.Source.Revision != "revision-1" ||
		result.Asset.TotalBytes != int64(len(baseBody)+len(tokenizerBody)) {
		t.Fatalf("prepared result = %#v", result)
	}
	if len(result.Asset.Artifacts) != 2 ||
		result.Asset.Artifacts[0].Name != "omnivoice-base-Q4_K_M.gguf" ||
		result.Asset.Artifacts[0].SHA256 != baseSHA {
		t.Fatalf("prepared artifacts = %#v", result.Asset.Artifacts)
	}
	assertFileBody(
		t,
		filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M", "revision-1", result.Asset.Artifacts[0].Name),
		baseBody,
	)
	if _, err := os.Stat(filepath.Join(
		cacheDirectory, "OMNIVOICE_Q4_K_M", "revision-1.partial",
	)); !os.IsNotExist(err) {
		t.Fatalf("staging revision remains after publication: %v", err)
	}
}

func TestPrepareModelAssetsRejectsInvalidCachedChecksumWithoutMutation(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeVerifiedCacheFixture(t, cacheDirectory)
	corruptPath := filepath.Join(
		cacheDirectory, "OMNIVOICE_Q4_K_M", "verified-revision",
		"omnivoice-base-Q4_K_M.gguf",
	)
	if err := os.WriteFile(corruptPath, []byte("corrupt bas"), 0o644); err != nil {
		t.Fatalf("corrupt cached asset: %v", err)
	}
	scopes := newScopes(t, "prepare-corrupt-cache")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	var sourceRequests atomic.Int32
	var mutations atomic.Int32
	service := newPreparationTestService(
		scopes,
		httpDoerFunc(func(*http.Request) (*http.Response, error) {
			sourceRequests.Add(1)
			return nil, fmt.Errorf("corrupt metadata-backed cache must fail before source access")
		}),
		models.RuntimeAssetEndpoints{
			BaseURL: "https://assets.example.test", APIBaseURL: "https://api.example.test",
		},
		&mutations,
	)

	result, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: ref, Name: "OMNIVOICE_Q4_K_M",
	})
	if !errors.Is(err, models.ErrAssetIntegrityFailed) {
		t.Fatalf("PrepareModelAssets error = %v, want ErrAssetIntegrityFailed", err)
	}
	if result.Asset.Readiness != models.AssetReadinessFailed ||
		result.Asset.Integrity != models.AssetIntegrityFailed {
		t.Fatalf("corrupt cache result = %#v", result)
	}
	if sourceRequests.Load() != 0 || mutations.Load() != 0 {
		t.Fatalf("corrupt cache performed source=%d mutation=%d effects", sourceRequests.Load(), mutations.Load())
	}
}

func TestPrepareModelAssetsRejectsInvalidCachedSize(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeVerifiedCacheFixture(t, cacheDirectory)
	if err := os.Truncate(filepath.Join(
		cacheDirectory, "OMNIVOICE_Q4_K_M", "verified-revision",
		"omnivoice-tokenizer-Q4_K_M.gguf",
	), 1); err != nil {
		t.Fatalf("truncate cached asset: %v", err)
	}
	scopes := newScopes(t, "prepare-invalid-size")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newPreparationTestService(
		scopes,
		httpDoerFunc(func(*http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("invalid metadata-backed cache must fail before source access")
		}),
		models.RuntimeAssetEndpoints{
			BaseURL: "https://assets.example.test", APIBaseURL: "https://api.example.test",
		},
		nil,
	)

	_, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: ref, Name: "OMNIVOICE_Q4_K_M",
	})
	if !errors.Is(err, models.ErrAssetIntegrityFailed) {
		t.Fatalf("PrepareModelAssets error = %v, want ErrAssetIntegrityFailed", err)
	}
}

func TestPrepareModelAssetsDoesNotPublishFailedVerification(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	body := []byte("short")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/models/Serveurperso/OmniVoice-GGUF":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"sha": "invalid-revision",
				"siblings": []map[string]any{
					{
						"rfilename": "omnivoice-base-Q4_K_M.gguf",
						"lfs":       map[string]any{"oid": sha256String(body), "size": len(body) + 1},
					},
					{
						"rfilename": "omnivoice-tokenizer-Q4_K_M.gguf",
						"lfs":       map[string]any{"oid": sha256String(body), "size": len(body)},
					},
				},
			})
		default:
			_, _ = writer.Write(body)
		}
	}))
	t.Cleanup(server.Close)
	scopes := newScopes(t, "prepare-invalid-download")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newPreparationTestService(
		scopes,
		server.Client(),
		models.RuntimeAssetEndpoints{BaseURL: server.URL, APIBaseURL: server.URL},
		nil,
	)

	result, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: ref, Name: "OMNIVOICE_Q4_K_M",
	})
	if !errors.Is(err, models.ErrAssetIntegrityFailed) {
		t.Fatalf("PrepareModelAssets error = %v, want ErrAssetIntegrityFailed", err)
	}
	var stageErr *models.PullStageError
	if !errors.As(err, &stageErr) || stageErr.Stage != models.PullStageIntegrityVerification ||
		stageErr.Cause == nil {
		t.Fatalf("PrepareModelAssets stage error = %#v, want integrity stage with cause", stageErr)
	}
	if result.Asset.Readiness != models.AssetReadinessFailed {
		t.Fatalf("failed pull result = %#v", result)
	}
	root := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	for _, path := range []string{
		filepath.Join(root, "invalid-revision"),
		filepath.Join(root, "invalid-revision.partial"),
		filepath.Join(root, metadataFileName),
	} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("failed verification published %q: %v", path, statErr)
		}
	}
}

func TestPrepareModelAssetsDoesNotPublishChecksumMismatch(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	body := []byte("complete but corrupt")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/models/Serveurperso/OmniVoice-GGUF":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"sha": "checksum-mismatch",
				"siblings": []map[string]any{
					{
						"rfilename": "omnivoice-base-Q4_K_M.gguf",
						"lfs":       map[string]any{"oid": sha256String([]byte("expected base")), "size": len(body)},
					},
					{
						"rfilename": "omnivoice-tokenizer-Q4_K_M.gguf",
						"lfs":       map[string]any{"oid": sha256String(body), "size": len(body)},
					},
				},
			})
		default:
			_, _ = writer.Write(body)
		}
	}))
	t.Cleanup(server.Close)
	scopes := newScopes(t, "prepare-checksum-mismatch")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newPreparationTestService(
		scopes,
		server.Client(),
		models.RuntimeAssetEndpoints{BaseURL: server.URL, APIBaseURL: server.URL},
		nil,
	)

	result, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: ref, Name: "OMNIVOICE_Q4_K_M",
	})
	if !errors.Is(err, models.ErrAssetIntegrityFailed) {
		t.Fatalf("PrepareModelAssets error = %v, want ErrAssetIntegrityFailed", err)
	}
	if result.Asset.ModelName != "OMNIVOICE_Q4_K_M" ||
		result.Asset.Readiness != models.AssetReadinessFailed ||
		result.Asset.Integrity != models.AssetIntegrityFailed ||
		result.Asset.Revision != "checksum-mismatch" {
		t.Fatalf("checksum failure result = %#v", result.Asset)
	}
	root := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	for _, path := range []string{
		filepath.Join(root, "checksum-mismatch"),
		filepath.Join(root, "checksum-mismatch.partial"),
		filepath.Join(root, metadataFileName),
	} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("checksum mismatch published %q: %v", path, statErr)
		}
	}
}

func TestPrepareModelAssetsClassifiesManifestFailures(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		statusCode int
		body       string
		wantCause  error
		wantStage  models.PullStage
	}{
		{
			name: "source response", statusCode: http.StatusBadRequest, body: "bad source request",
			wantCause: models.ErrSourceFetchFailed, wantStage: models.PullStageSourceFetch,
		},
		{
			name:       "missing required artifact",
			statusCode: http.StatusOK,
			body: `{"sha":"revision","siblings":[` +
				`{"rfilename":"omnivoice-base-Q4_K_M.gguf","size":4}]}`,
			wantCause: models.ErrModelReferenceUnknown, wantStage: models.PullStageSourceResolution,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cacheDirectory := t.TempDir()
			scopes := newScopes(t, "manifest-failure-"+test.name)
			ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
			service := newPreparationTestService(
				scopes,
				httpDoerFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: test.statusCode,
						Body:       io.NopCloser(strings.NewReader(test.body)),
					}, nil
				}),
				models.RuntimeAssetEndpoints{
					BaseURL:    "https://assets.example.test",
					APIBaseURL: "https://api.example.test",
				},
				nil,
			)

			_, err := service.PrepareModelAssets(
				context.Background(),
				models.PrepareModelAssetsRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"},
			)
			if !errors.Is(err, test.wantCause) {
				t.Fatalf("PrepareModelAssets error = %v, want %v", err, test.wantCause)
			}
			var stageErr *models.PullStageError
			if !errors.As(err, &stageErr) || stageErr.Stage != test.wantStage ||
				stageErr.Cause == nil {
				t.Fatalf("PrepareModelAssets stage error = %#v, want %s stage with cause", stageErr, test.wantStage)
			}
			diagnostics := pullsupport.PullDiagnosticsFromError(err)
			if diagnostics.ModelName != "OMNIVOICE_Q4_K_M" || diagnostics.ResolvedRepository == "" ||
				diagnostics.Operation == "" || (test.statusCode != http.StatusOK && diagnostics.UpstreamStatusCode != test.statusCode) {
				t.Fatalf("PrepareModelAssets diagnostics = %#v, want safe source facts", diagnostics)
			}
			if test.statusCode == http.StatusOK && diagnostics.File == "" {
				t.Fatalf("PrepareModelAssets diagnostics = %#v, want missing logical artifact", diagnostics)
			}
			if strings.Contains(err.Error(), test.body) {
				t.Fatalf("PrepareModelAssets error leaked response body: %v", err)
			}
		})
	}
}

func TestPrepareModelAssetsClassifiesAssetDownloadFailure(t *testing.T) {
	t.Parallel()

	baseBody := []byte("downloaded base")
	tokenizerBody := []byte("downloaded tokenizer")
	baseSHA := sha256String(baseBody)
	tokenizerSHA := sha256String(tokenizerBody)
	var failedDownloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/models/Serveurperso/OmniVoice-GGUF":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"sha": "download-failure-revision",
				"siblings": []map[string]any{
					{"rfilename": "omnivoice-base-Q4_K_M.gguf", "lfs": map[string]any{
						"oid": baseSHA, "size": len(baseBody),
					}},
					{"rfilename": "omnivoice-tokenizer-Q4_K_M.gguf", "lfs": map[string]any{
						"oid": tokenizerSHA, "size": len(tokenizerBody),
					}},
				},
			})
		case "/Serveurperso/OmniVoice-GGUF/resolve/download-failure-revision/omnivoice-base-Q4_K_M.gguf":
			failedDownloads.Add(1)
			http.Error(writer, "upstream temporarily unavailable", http.StatusServiceUnavailable)
		case "/Serveurperso/OmniVoice-GGUF/resolve/download-failure-revision/omnivoice-tokenizer-Q4_K_M.gguf":
			_, _ = writer.Write(tokenizerBody)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	scopes := newScopes(t, "prepare-download-failure")
	cacheDirectory := t.TempDir()
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	service := newPreparationTestService(
		scopes,
		server.Client(),
		models.RuntimeAssetEndpoints{BaseURL: server.URL, APIBaseURL: server.URL},
		nil,
	)
	result, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: ref, Name: "OMNIVOICE_Q4_K_M",
	})
	if !errors.Is(err, models.ErrSourceFetchFailed) {
		t.Fatalf("PrepareModelAssets error = %v, want ErrSourceFetchFailed", err)
	}
	if failedDownloads.Load() != assetSourceMaxAttempts {
		t.Fatalf("failed download attempts = %d, want %d", failedDownloads.Load(), assetSourceMaxAttempts)
	}
	diagnostics := pullsupport.PullDiagnosticsFromError(err)
	if diagnostics.File != "omnivoice-base-Q4_K_M.gguf" ||
		diagnostics.UpstreamStatusCode != http.StatusServiceUnavailable {
		t.Fatalf("download failure diagnostics = %#v", diagnostics)
	}
	if result.Asset.Readiness != models.AssetReadinessFailed || result.Asset.Revision != "download-failure-revision" {
		t.Fatalf("download failure result = %#v", result.Asset)
	}
	assertAttemptAbsent(t, cacheDirectory, "download-failure-revision")
}

func TestPrepareModelAssetsRetriesTimeoutThenReturnsSourceFailure(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "manifest-timeout")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	var attempts atomic.Int32
	service := newPreparationTestService(
		scopes,
		httpDoerFunc(func(*http.Request) (*http.Response, error) {
			attempts.Add(1)
			return nil, timeoutTestError{}
		}),
		models.RuntimeAssetEndpoints{
			BaseURL: "https://assets.example.test", APIBaseURL: "https://api.example.test",
		},
		nil,
	)

	_, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: ref, Name: "OMNIVOICE_Q4_K_M",
	})
	if !errors.Is(err, models.ErrSourceFetchFailed) {
		t.Fatalf("PrepareModelAssets error = %v, want ErrSourceFetchFailed", err)
	}
	if attempts.Load() != assetSourceMaxAttempts {
		t.Fatalf("source attempts = %d, want %d", attempts.Load(), assetSourceMaxAttempts)
	}
}

func newPreparationTestService(
	scopes runtimescopes.Service,
	client modelseffects.AssetHTTPDoer,
	endpoints models.RuntimeAssetEndpoints,
	mutations *atomic.Int32,
) *service {
	record := func() {
		if mutations != nil {
			mutations.Add(1)
		}
	}
	return New(
		scopes,
		models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		client,
		endpoints,
		func(path string, mode os.FileMode) error {
			record()
			return os.MkdirAll(path, mode)
		},
		os.Stat,
		os.UserHomeDir,
		func(path string, body []byte, mode os.FileMode) error {
			record()
			return os.WriteFile(path, body, mode)
		},
		func(oldPath, newPath string) error {
			record()
			return os.Rename(oldPath, newPath)
		},
		func(path string) error {
			record()
			return os.Remove(path)
		},
		os.ReadFile,
		os.ReadDir,
		func(path string) (io.WriteCloser, error) {
			record()
			return os.Create(path)
		},
		func(path string) (io.ReadCloser, error) { return os.Open(path) },
		assets.ConstructionOptions{Coordination: mustPreparationLockingService()},
	).(*service)
}

func mustPreparationLockingService() platformlocking.Service {
	service, err := platformlocking.New(platformlocking.LocalFileSystem{})
	if err != nil {
		panic(err)
	}
	return service
}

func writeVerifiedCacheFixture(t *testing.T, cacheDirectory string) {
	t.Helper()
	baseBody := []byte("cached base")
	tokenizerBody := []byte("cached tokenizer")
	root := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	revision := filepath.Join(root, "verified-revision")
	if err := os.MkdirAll(revision, 0o755); err != nil {
		t.Fatalf("create verified cache: %v", err)
	}
	files := []struct {
		name string
		body []byte
	}{
		{name: "omnivoice-base-Q4_K_M.gguf", body: baseBody},
		{name: "omnivoice-tokenizer-Q4_K_M.gguf", body: tokenizerBody},
	}
	metadata := cacheMetadata{ModelName: "OMNIVOICE_Q4_K_M", Revision: "verified-revision"}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(revision, file.name), file.body, 0o644); err != nil {
			t.Fatalf("write verified asset: %v", err)
		}
		metadata.Files = append(metadata.Files, metadataFile{
			Path: file.name, Bytes: int64(len(file.body)), SHA256: sha256String(file.body),
		})
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal verified metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, metadataFileName), body, 0o644); err != nil {
		t.Fatalf("write verified metadata: %v", err)
	}
}

func sha256String(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func assertFileBody(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	if string(got) != string(want) {
		t.Fatalf("file %q = %q, want %q", path, got, want)
	}
}

type httpDoerFunc func(*http.Request) (*http.Response, error)

func (do httpDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return do(request)
}

type timeoutTestError struct{}

func (timeoutTestError) Error() string   { return "timeout" }
func (timeoutTestError) Timeout() bool   { return true }
func (timeoutTestError) Temporary() bool { return true }

func TestPrepareGenericAssetsFetchesManifestWhenRequirementsAreOmitted(t *testing.T) {
	t.Parallel()
	body := []byte("manifest-discovered model")
	scopes := newScopes(t, "generic-manifest-discovery")
	scope := openScope(t, scopes, t.TempDir(), models.RuntimeConfig{})
	service := newGenericService(t, scopes, genericManifestClient("weights.bin", body, func() []byte { return body }), func(string) string { return "" })
	result, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{Scope: scope, Reference: models.ModelReference{NameOrURI: "hf://owner/repo@" + genericTestRevision}})
	if err != nil || result.Outcome != models.AssetPreparationPrepared || len(result.Asset.Artifacts) != 1 || result.Asset.Artifacts[0].Name != "weights.bin" || result.Asset.Artifacts[0].SHA256 != sha256Hex(body) {
		t.Fatalf("manifest-discovered result = %#v", result)
	}
}

func TestPreflightGenericAssetsReportsMissingBytesWithoutReadingHEADBody(t *testing.T) {
	t.Parallel()

	modelBody := []byte("model weights")
	backendBody := []byte("backend archive")
	modelPath := filepath.Join(t.TempDir(), "model.bin")
	if err := os.WriteFile(modelPath, modelBody, 0o644); err != nil {
		t.Fatalf("write model fixture: %v", err)
	}
	backendURL := "https://github.com/owner/backend/releases/download/v1/backend.bin"
	var methods []string
	var headBody preflightTrackingReadCloser
	client := httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		methods = append(methods, request.Method)
		headBody = preflightTrackingReadCloser{reader: bytes.NewReader(backendBody)}
		return &http.Response{
			StatusCode:    http.StatusOK,
			Body:          &headBody,
			ContentLength: int64(len(backendBody)),
		}, nil
	})
	scopes := newScopes(t, "preflight-estimate")
	scope := openScope(t, scopes, t.TempDir(), models.RuntimeConfig{})
	service := newGenericService(t, scopes, client, func(string) string { return "" })
	request := models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      "model",
		Reference: models.ModelReference{NameOrURI: modelPath},
		Artifacts: []models.AssetRequirement{{Name: "model.bin", Bytes: int64(len(modelBody)), SHA256: sha256Hex(modelBody)}},
		Backend:   "fixture-backend",
		BackendReference: models.ModelReference{
			NameOrURI: backendURL,
		},
		BackendArtifacts: []models.AssetRequirement{{
			Name: "backend.bin", Bytes: int64(len(backendBody)), SHA256: sha256Hex(backendBody),
		}},
	}

	result, err := service.PreflightModelAssets(context.Background(), request)
	if err != nil {
		t.Fatalf("PreflightModelAssets: %v", err)
	}
	if result.ModelName != "model" || result.BackendBytes != int64(len(backendBody)) ||
		result.ModelBytes != int64(len(modelBody)) || result.TotalBytes != int64(len(modelBody)+len(backendBody)) ||
		!result.BackendDownloadRequired || !result.ModelDownloadRequired {
		t.Fatalf("preflight result = %#v, want exact missing byte totals", result)
	}
	if !reflect.DeepEqual(methods, []string{http.MethodHead}) {
		t.Fatalf("preflight methods = %#v, want backend HEAD only", methods)
	}
	if headBody.reads.Load() != 0 || headBody.closed.Load() != 1 {
		t.Fatalf("HEAD body reads/closes = %d/%d, want 0/1", headBody.reads.Load(), headBody.closed.Load())
	}
}

func TestPreflightGenericAssetsStopsBeforeModelMetadataWhenBackendHEADFails(t *testing.T) {
	t.Parallel()

	var methods []string
	client := httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		methods = append(methods, request.Method+" "+request.URL.Path)
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("private backend response")),
		}, nil
	})
	scopes := newScopes(t, "preflight-backend-failure")
	scope := openScope(t, scopes, t.TempDir(), models.RuntimeConfig{})
	service := newGenericService(t, scopes, client, func(string) string { return "" })
	_, err := service.PreflightModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      "model",
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo/model.bin@" + genericTestRevision},
		Artifacts: []models.AssetRequirement{{Name: "model.bin"}},
		Backend:   "fixture-backend",
		BackendReference: models.ModelReference{
			NameOrURI: "https://github.com/owner/backend/releases/download/v1/backend.bin",
		},
		BackendArtifacts: []models.AssetRequirement{{Name: "backend.bin", Bytes: 4, SHA256: strings.Repeat("a", 64)}},
	})
	if !errors.Is(err, models.ErrAssetBackendNotReady) {
		t.Fatalf("preflight error = %v, want ErrAssetBackendNotReady", err)
	}
	if !reflect.DeepEqual(methods, []string{http.MethodHead + " /owner/backend/releases/download/v1/backend.bin"}) {
		t.Fatalf("backend failure methods = %#v, want one HEAD before model metadata", methods)
	}
	if strings.Contains(err.Error(), "private backend response") {
		t.Fatalf("preflight error leaked backend response body: %v", err)
	}
}

func TestPreflightGenericAssetsRejectsOverflowBeforeRemoteEffects(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	client := httpDoerFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("overflow preflight must not use network")
	})
	scopes := newScopes(t, "preflight-overflow")
	scope := openScope(t, scopes, t.TempDir(), models.RuntimeConfig{})
	service := newGenericService(t, scopes, client, func(string) string { return "" })
	_, err := service.PreflightModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      "model",
		Reference: models.ModelReference{NameOrURI: "hf://owner/repo/model.bin@" + genericTestRevision},
		Artifacts: []models.AssetRequirement{{Name: "model.bin", Bytes: math.MaxInt64, SHA256: strings.Repeat("a", 64)}},
		Backend:   "fixture-backend",
		BackendReference: models.ModelReference{
			NameOrURI: "https://github.com/owner/backend/releases/download/v1/backend.bin",
		},
		BackendArtifacts: []models.AssetRequirement{{Name: "backend.bin", Bytes: 1, SHA256: strings.Repeat("b", 64)}},
	})
	if !errors.Is(err, models.ErrAssetEstimateOverflow) {
		t.Fatalf("overflow error = %v, want ErrAssetEstimateOverflow", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("overflow preflight HTTP requests = %d, want 0", requests.Load())
	}
}

type preflightTrackingReadCloser struct {
	reader *bytes.Reader
	reads  atomic.Int32
	closed atomic.Int32
}

func (body *preflightTrackingReadCloser) Read(buffer []byte) (int, error) {
	body.reads.Add(1)
	return body.reader.Read(buffer)
}

func (body *preflightTrackingReadCloser) Close() error {
	body.closed.Add(1)
	return nil
}
