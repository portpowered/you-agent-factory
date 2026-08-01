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
	"strings"
	"sync/atomic"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

func TestPrepareModelAssetsRejectsPreCancelledRequestBeforeEffects(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "prepare-pre-cancelled")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	var sourceRequests atomic.Int32
	var mutations atomic.Int32
	service := newPreparationTestService(
		scopes,
		httpDoerFunc(func(*http.Request) (*http.Response, error) {
			sourceRequests.Add(1)
			return nil, errors.New("unexpected source request")
		}),
		models.RuntimeAssetEndpoints{
			BaseURL: "https://assets.example.test", APIBaseURL: "https://api.example.test",
		},
		&mutations,
	)
	cancelCause := errors.New("operator stopped asset preparation")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cancelCause)

	_, err := service.PrepareModelAssets(ctx, models.PrepareModelAssetsRequest{
		Scope: ref, Name: "OMNIVOICE_Q4_K_M",
	})
	if !errors.Is(err, models.ErrAssetCancelled) ||
		!errors.Is(err, context.Canceled) ||
		!errors.Is(err, cancelCause) {
		t.Fatalf("pre-cancelled error = %v, want typed cancellation and underlying causes", err)
	}
	if sourceRequests.Load() != 0 || mutations.Load() != 0 {
		t.Fatalf(
			"pre-cancelled preparation performed source=%d mutation=%d effects",
			sourceRequests.Load(),
			mutations.Load(),
		)
	}
}

func TestPrepareModelAssetsCancelsInFlightCleansUpAndRetries(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "prepare-in-flight-cancelled")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	baseBody := []byte("complete base")
	tokenizerBody := []byte("complete tokenizer")
	cancelCause := errors.New("runtime scope stopped")
	ctx, cancel := context.WithCancelCause(context.Background())
	service := newPreparationTestService(
		scopes,
		newAssetTestClient("cancelled-revision", baseBody, tokenizerBody, func() io.ReadCloser {
			return &cancellingReadCloser{cancel: cancel, cause: cancelCause}
		}),
		models.RuntimeAssetEndpoints{
			BaseURL: "https://assets.example.test", APIBaseURL: "https://api.example.test",
		},
		nil,
	)
	request := models.PrepareModelAssetsRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"}

	result, err := service.PrepareModelAssets(ctx, request)
	if !errors.Is(err, models.ErrAssetCancelled) ||
		!errors.Is(err, context.Canceled) ||
		!errors.Is(err, cancelCause) {
		t.Fatalf("in-flight cancellation error = %v, want typed cancellation and causes", err)
	}
	if result.Asset.Readiness != models.AssetReadinessFailed {
		t.Fatalf("cancelled result = %#v, want failed readiness", result)
	}
	assertAttemptAbsent(t, cacheDirectory, "cancelled-revision")

	service.client = newAssetTestClient("cancelled-revision", baseBody, tokenizerBody, nil)
	retried, err := service.PrepareModelAssets(context.Background(), request)
	if err != nil {
		t.Fatalf("retry after cancellation: %v", err)
	}
	if retried.Outcome != models.AssetPreparationPrepared ||
		retried.Asset.Integrity != models.AssetIntegrityVerified {
		t.Fatalf("retry result = %#v, want verified preparation", retried)
	}
}

func TestPrepareModelAssetsInterruptedReadCleansUpAndRetries(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "prepare-interrupted-read")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	baseBody := []byte("complete base")
	tokenizerBody := []byte("complete tokenizer")
	service := newPreparationTestService(
		scopes,
		newAssetTestClient("interrupted-revision", baseBody, tokenizerBody, func() io.ReadCloser {
			return &failingReadCloser{cause: io.ErrUnexpectedEOF}
		}),
		models.RuntimeAssetEndpoints{
			BaseURL: "https://assets.example.test", APIBaseURL: "https://api.example.test",
		},
		nil,
	)
	request := models.PrepareModelAssetsRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"}

	_, err := service.PrepareModelAssets(context.Background(), request)
	if !errors.Is(err, models.ErrAssetPreparationInterrupted) ||
		!errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("interrupted read error = %v, want interruption and read cause", err)
	}
	assertAttemptAbsent(t, cacheDirectory, "interrupted-revision")

	service.client = newAssetTestClient("interrupted-revision", baseBody, tokenizerBody, nil)
	if _, err := service.PrepareModelAssets(context.Background(), request); err != nil {
		t.Fatalf("retry after interrupted read: %v", err)
	}
}

func TestPrepareModelAssetsSurfacesCleanupFailureWithPrimaryFailure(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "prepare-cleanup-failure")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	primaryFailure := errors.New("response stream failed")
	cleanupFailure := errors.New("staged file removal failed")
	service := newPreparationTestService(
		scopes,
		newAssetTestClient("cleanup-failure", []byte("base"), []byte("tokenizer"), func() io.ReadCloser {
			return &failingReadCloser{cause: primaryFailure}
		}),
		models.RuntimeAssetEndpoints{
			BaseURL: "https://assets.example.test", APIBaseURL: "https://api.example.test",
		},
		nil,
	)
	removePath := service.removePath
	service.removePath = func(path string) error {
		if filepath.Base(path) == "omnivoice-base-Q4_K_M.gguf" {
			return cleanupFailure
		}
		return removePath(path)
	}

	_, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope: ref, Name: "OMNIVOICE_Q4_K_M",
	})
	if !errors.Is(err, models.ErrAssetPreparationInterrupted) ||
		!errors.Is(err, primaryFailure) ||
		!errors.Is(err, cleanupFailure) {
		t.Fatalf("cleanup error = %v, want primary classification, primary cause, and cleanup cause", err)
	}
}

func TestPrepareModelAssetsRenameFailurePreservesExistingRevisionAndRetries(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	existingPath := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M", "existing-revision", "marker")
	if err := os.MkdirAll(filepath.Dir(existingPath), 0o755); err != nil {
		t.Fatalf("create existing revision: %v", err)
	}
	if err := os.WriteFile(existingPath, []byte("preserve me"), 0o644); err != nil {
		t.Fatalf("write existing revision: %v", err)
	}
	scopes := newScopes(t, "prepare-rename-failure")
	ref := openScope(t, scopes, cacheDirectory, runtimeConfig(""))
	baseBody := []byte("base")
	tokenizerBody := []byte("tokenizer")
	service := newPreparationTestService(
		scopes,
		newAssetTestClient("new-revision", baseBody, tokenizerBody, nil),
		models.RuntimeAssetEndpoints{
			BaseURL: "https://assets.example.test", APIBaseURL: "https://api.example.test",
		},
		nil,
	)
	renameFailure := errors.New("cache rename interrupted")
	renamePath := service.renamePath
	service.renamePath = func(_, _ string) error { return renameFailure }
	request := models.PrepareModelAssetsRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"}

	_, err := service.PrepareModelAssets(context.Background(), request)
	if !errors.Is(err, models.ErrAssetPreparationInterrupted) ||
		!errors.Is(err, renameFailure) {
		t.Fatalf("rename error = %v, want typed interruption and rename cause", err)
	}
	assertFileBody(t, existingPath, []byte("preserve me"))
	assertAttemptAbsent(t, cacheDirectory, "new-revision")

	service.renamePath = renamePath
	if _, err := service.PrepareModelAssets(context.Background(), request); err != nil {
		t.Fatalf("retry after rename interruption: %v", err)
	}
	assertFileBody(t, existingPath, []byte("preserve me"))
}

func newAssetTestClient(
	revision string,
	baseBody []byte,
	tokenizerBody []byte,
	baseOverride func() io.ReadCloser,
) modelseffects.AssetHTTPDoer {
	manifest, _ := json.Marshal(map[string]any{
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
	return httpDoerFunc(func(request *http.Request) (*http.Response, error) {
		var body io.ReadCloser
		switch {
		case strings.HasPrefix(request.URL.Path, "/models/"):
			body = io.NopCloser(bytes.NewReader(manifest))
		case strings.Contains(request.URL.Path, "omnivoice-base"):
			if baseOverride != nil {
				body = baseOverride()
			} else {
				body = io.NopCloser(bytes.NewReader(baseBody))
			}
		default:
			body = io.NopCloser(bytes.NewReader(tokenizerBody))
		}
		return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
	})
}

func assertAttemptAbsent(t *testing.T, cacheDirectory, revision string) {
	t.Helper()
	root := filepath.Join(cacheDirectory, "OMNIVOICE_Q4_K_M")
	for _, path := range []string{
		filepath.Join(root, revision),
		filepath.Join(root, revision+".partial"),
		filepath.Join(root, metadataFileName),
		filepath.Join(root, metadataFileName+".partial"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("failed attempt left %q: %v", path, err)
		}
	}
}

type cancellingReadCloser struct {
	cancel context.CancelCauseFunc
	cause  error
}

func (reader *cancellingReadCloser) Read([]byte) (int, error) {
	reader.cancel(reader.cause)
	return 0, reader.cause
}

func (*cancellingReadCloser) Close() error { return nil }

type failingReadCloser struct {
	cause error
}

func (reader *failingReadCloser) Read([]byte) (int, error) { return 0, reader.cause }
func (*failingReadCloser) Close() error                    { return nil }
