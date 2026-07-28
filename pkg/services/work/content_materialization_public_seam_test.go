package work_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workservice "github.com/portpowered/infinite-you/pkg/services/work/service"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
)

func newPublicWorkRootMaterializer(t *testing.T) work.ContentMaterializer {
	t.Helper()
	materializer, err := workwire.NewContentMaterializationService(
		work.ContentHostPlatform(runtime.GOOS),
		&http.Client{
			Timeout:       workwire.DefaultContentMaterializationHTTPTimeout,
			CheckRedirect: workwire.ContentMaterializationRedirectPolicy(0, false),
		},
		os.Stat,
		func(dir, pattern string) (work.ContentTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.WriteFile,
		func(path string) (io.WriteCloser, error) {
			return os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
		},
	)
	if err != nil {
		t.Fatalf("work wire.NewContentMaterializationService: %v", err)
	}
	return materializer
}

func newPublicWorkRootWithMaterialization(t *testing.T) work.Service {
	t.Helper()
	return workservice.NewService(nil, nil, nil, newPublicWorkRootMaterializer(t))
}

func TestPublicWorkRootMaterializationSeamSuccessLocalFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "seam.png")
	if err := os.WriteFile(path, []byte("png-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	rawURL, err := work.FilesystemPathToContentURL(path)
	if err != nil {
		t.Fatalf("FilesystemPathToContentURL: %v", err)
	}

	materializer := newPublicWorkRootMaterializer(t)
	got, cleanup, err := materializer.MaterializeContentURL(ctx, rawURL)
	if err != nil {
		t.Fatalf("MaterializeContentURL: %v", err)
	}
	if got != path || cleanup == nil {
		t.Fatalf("materialize = (%q, %v), want local path and cleanup handle", got, cleanup)
	}
	cleanup()
	if _, statErr := os.Stat(got); statErr != nil {
		t.Fatalf("local file should remain after cleanup: %v", statErr)
	}
}

func TestPublicWorkRootMaterializationSeamSuccessThroughRootService(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "root-seam.png")
	if err := os.WriteFile(path, []byte("root-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	rawURL, err := work.FilesystemPathToContentURL(path)
	if err != nil {
		t.Fatalf("FilesystemPathToContentURL: %v", err)
	}

	var root work.Service = newPublicWorkRootWithMaterialization(t)
	got, cleanup, err := root.MaterializeContentURL(ctx, rawURL)
	if err != nil {
		t.Fatalf("MaterializeContentURL: %v", err)
	}
	if got != path || cleanup == nil {
		t.Fatalf("materialize = (%q, %v), want local path and cleanup handle", got, cleanup)
	}
	cleanup()
}

func TestPublicWorkRootMaterializationSeamTypedFailuresThroughRootService(t *testing.T) {
	ctx := context.Background()
	var root work.Service = newPublicWorkRootWithMaterialization(t)

	t.Run("unsafe content URL", func(t *testing.T) {
		_, cleanup, err := root.MaterializeContentURL(ctx, "http://127.0.0.1/secret")
		defer cleanup()
		if !errors.Is(err, work.ErrUnsafeContentURL) {
			t.Fatalf("error = %v, want errors.Is(..., ErrUnsafeContentURL)", err)
		}
	})

	t.Run("inaccessible content URL", func(t *testing.T) {
		_, cleanup, err := root.MaterializeContentURL(ctx, "https://example.invalid/missing.png")
		defer cleanup()
		if !errors.Is(err, work.ErrContentURLInaccessible) {
			t.Fatalf("error = %v, want errors.Is(..., ErrContentURLInaccessible)", err)
		}
	})
}

// TestPublicWorkRootMaterializationSeamSealsMaterializationCutWithoutStagingOrStateAccess
// seals IMP-WORK-03: the nested content_materialization capability remains a
// materialization-only ContentMaterializer behind CTR-WORK, not the full Work
// root that owns staging and state-access. It also re-proves success and typed
// failures through the published Work root materialization seam.
func TestPublicWorkRootMaterializationSeamSealsMaterializationCutWithoutStagingOrStateAccess(t *testing.T) {
	ctx := context.Background()
	materializer := newPublicWorkRootMaterializer(t)

	if _, ok := any(materializer).(work.Service); ok {
		t.Fatal("content_materialization must not satisfy work.Service (staging/state-access stay out of this cut)")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "seal.png")
	if err := os.WriteFile(path, []byte("seal-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	rawURL, err := work.FilesystemPathToContentURL(path)
	if err != nil {
		t.Fatalf("FilesystemPathToContentURL: %v", err)
	}

	var root work.Service = newPublicWorkRootWithMaterialization(t)
	got, cleanup, err := root.MaterializeContentURL(ctx, rawURL)
	if err != nil || got != path || cleanup == nil {
		t.Fatalf("success materialize = (%q, %v, %v), want local path and cleanup", got, cleanup, err)
	}
	cleanup()

	if _, _, err := root.MaterializeContentURL(ctx, "http://127.0.0.1/secret"); !errors.Is(err, work.ErrUnsafeContentURL) {
		t.Fatalf("unsafe materialize error = %v, want ErrUnsafeContentURL", err)
	}
	if _, _, err := root.MaterializeContentURL(ctx, "https://example.invalid/missing.png"); !errors.Is(err, work.ErrContentURLInaccessible) {
		t.Fatalf("inaccessible materialize error = %v, want ErrContentURLInaccessible", err)
	}
}
