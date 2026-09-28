package wire

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

type galleryInstallRunner struct {
	run func(platformprocess.CommandRequest) (platformprocess.CommandResult, error)
}

func (runner galleryInstallRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner.run(request)
}

func TestLocalAIGalleryInstallerUsesLocalAIAndInstalledDirectory(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "backends")
	var observed platformprocess.CommandRequest
	installer := localAIGalleryInstallerAt(galleryInstallRunner{run: func(request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		observed = request
		if err := os.Mkdir(filepath.Join(root, "cuda12-llama-cpp"), 0o700); err != nil {
			t.Fatalf("make installed backend: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "cuda12-llama-cpp", "run.sh"), []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatalf("make backend entrypoint: %v", err)
		}
		return platformprocess.CommandResult{}, nil
	}}, root, func(context.Context) (string, error) { return "/usr/bin/local-ai", nil })
	path, err := installer(context.Background(), "cuda12-llama-cpp", false)
	if err != nil {
		t.Fatalf("install gallery backend: %v", err)
	}
	if path != filepath.Join(root, "cuda12-llama-cpp") || observed.Command != "/usr/bin/local-ai" ||
		!reflect.DeepEqual(observed.Args, []string{"backends", "install", "--backends-path=" + root, "cuda12-llama-cpp"}) {
		t.Fatalf("path = %q, request = %#v", path, observed)
	}
}

func TestLocalAIGalleryInstallerRejectsMissingOutput(t *testing.T) {
	t.Parallel()
	installer := localAIGalleryInstallerAt(galleryInstallRunner{run: func(platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		return platformprocess.CommandResult{}, nil
	}}, filepath.Join(t.TempDir(), "backends"), func(context.Context) (string, error) { return "local-ai", nil })
	if _, err := installer(context.Background(), "cuda12-whisper", false); err == nil {
		t.Fatal("install succeeded without an installed backend directory")
	}
}

func TestLocalAIGalleryInstallerOfflineUsesOnlyInstalledBackend(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "backends")
	if err := os.MkdirAll(filepath.Join(root, "cuda12-whisper"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cuda12-whisper", "run.sh"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	installer := localAIGalleryInstallerAt(galleryInstallRunner{run: func(platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		t.Fatal("offline request launched LocalAI")
		return platformprocess.CommandResult{}, nil
	}}, root, func(context.Context) (string, error) {
		t.Fatal("offline request resolved LocalAI binary")
		return "", nil
	})
	if path, err := installer(context.Background(), "cuda12-whisper", true); err != nil || path != filepath.Join(root, "cuda12-whisper") {
		t.Fatalf("offline installed backend = %q, error = %v", path, err)
	}
}
