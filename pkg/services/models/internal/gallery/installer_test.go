package gallery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	models "github.com/portpowered/infinite-you/pkg/services/models"
)

type galleryContextRunner struct {
	run func(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error)
}

func (runner galleryContextRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner.run(ctx, request)
}
func TestLocalAIGalleryInstallerInstallationAndOffline(t *testing.T) {
	t.Parallel()
	cache := t.TempDir()
	installer := fixtureInstaller()
	installer.CacheDirectory = func() (string, error) { return cache, nil }
	installer.Environment = func(string) string { return "selected-local-ai" }
	root := filepath.Join(cache, "you", "localai-backends")
	selected := filepath.Join(root, "cpu-whisper")
	calls := 0
	installer.Runner = galleryContextRunner{run: func(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		calls++
		if request.Command != "selected-local-ai" || !reflect.DeepEqual(request.Args, []string{"backends", "install", "--backends-path=" + root, "cpu-whisper"}) {
			t.Fatalf("gallery command = %#v", request)
		}
		if err := os.MkdirAll(selected, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(selected, "run.sh"), []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		return platformprocess.CommandResult{}, nil
	}}
	if path, err := installer.Install(t.Context(), "cpu-whisper", false); err != nil || path != selected {
		t.Fatalf("install = %q %v", path, err)
	}
	installer.Environment = func(string) string { t.Fatal("offline resolved binary"); return "" }
	if path, err := installer.Install(t.Context(), "cpu-whisper", true); err != nil || path != selected {
		t.Fatalf("offline installed = %q %v", path, err)
	}
	if path, err := installer.Install(t.Context(), "cpu-missing", true); path != "" || !errors.Is(err, models.ErrAssetOffline) {
		t.Fatalf("offline missing = %q %v", path, err)
	}
	if calls != 1 {
		t.Fatalf("offline launched command; calls=%d", calls)
	}
}
func TestLocalAIGalleryInstallerRejectsFailureAndRetries(t *testing.T) {
	t.Parallel()
	for _, fault := range []string{"cache", "command", "exit", "script"} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			cache := t.TempDir()
			installer := fixtureInstaller()
			failed := true
			cause := errors.New("selected installation failure")
			installer.CacheDirectory = func() (string, error) {
				if failed && fault == "cache" {
					return "", cause
				}
				return cache, nil
			}
			installer.Environment = func(string) string { return "selected-local-ai" }
			installer.Runner = galleryInstallationFaultRunner(cache, fault, &failed, cause)
			if path, err := installer.Install(t.Context(), "cpu-whisper", false); path != "" || err == nil {
				t.Fatalf("failed install = %q %v", path, err)
			} else if (fault == "cache" || fault == "command") && !errors.Is(err, cause) {
				t.Fatalf("lost cause: %v", err)
			}
			failed = false
			if path, err := installer.Install(t.Context(), "cpu-whisper", false); err != nil || path == "" {
				t.Fatalf("repaired install = %q %v", path, err)
			}
		})
	}
}
func TestLocalAIGalleryInstallerCancelledAndUnsafeDoNotExecute(t *testing.T) {
	t.Parallel()
	installer := fixtureInstaller()
	installer.CacheDirectory = func() (string, error) { t.Fatal("invalid request resolved cache"); return "", nil }
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := installer.Install(ctx, "cpu-whisper", false); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled install = %v", err)
	}
	for _, name := range []string{"", "../outside", "a/b", "a\\b"} {
		if path, err := installer.Install(t.Context(), name, false); path != "" || err == nil {
			t.Fatalf("unsafe name = %q %v", path, err)
		}
	}
}

func galleryInstallationFaultRunner(cache, fault string, failed *bool, cause error) galleryContextRunner {
	return galleryContextRunner{run: func(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		if *failed {
			switch fault {
			case "command":
				return platformprocess.CommandResult{}, cause
			case "exit":
				return platformprocess.CommandResult{ExitCode: 17}, nil
			case "script":
				return platformprocess.CommandResult{}, nil
			}
		}
		directory := filepath.Join(cache, "you", "localai-backends", "cpu-whisper")
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return platformprocess.CommandResult{}, err
		}
		return platformprocess.CommandResult{}, os.WriteFile(filepath.Join(directory, "run.sh"), []byte("#!/bin/sh\n"), 0o700)
	}}
}
