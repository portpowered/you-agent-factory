package wire

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	managedchild "github.com/portpowered/infinite-you/pkg/platform/process/managedchild"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	"github.com/portpowered/infinite-you/pkg/wire/internal/managedbackend"
)

func TestLinuxBackendPrefersPublishedCUDAAndKeepsGalleryFallback(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name           string
		accelerator    string
		cudaAvailable  bool
		offline        bool
		publishedCUDA  bool
		publicationErr bool
		wantPublished  bool
		wantCalls      int
	}{
		{name: "published CUDA", cudaAvailable: true, publishedCUDA: true, wantPublished: true, wantCalls: 1},
		{name: "missing CUDA archive", cudaAvailable: true, wantCalls: 1},
		{name: "release unavailable", cudaAvailable: true, publicationErr: true, wantCalls: 1},
		{name: "explicit CUDA without archive", accelerator: "cuda", wantCalls: 1},
		{name: "offline CUDA", cudaAvailable: true, offline: true, publishedCUDA: true},
		{name: "CPU host", publishedCUDA: true},
		{name: "explicit CPU", accelerator: "cpu", cudaAvailable: true, publishedCUDA: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			publishedCalls, galleryCalls := 0, 0
			published := func(_ context.Context, request modelswire.ResolvedHostConfiguration, offline bool) (modelswire.BackendArtifactSelection, error) {
				publishedCalls++
				if offline || request.Platform.Accelerator != "cuda" {
					t.Fatalf("published request = %#v, offline = %t", request, offline)
				}
				if test.publicationErr {
					return modelswire.BackendArtifactSelection{}, errors.New("release unavailable")
				}
				if !test.publishedCUDA {
					return modelswire.BackendArtifactSelection{Accelerator: "cpu", Name: "pinned-cpu"}, nil
				}
				return modelswire.BackendArtifactSelection{Accelerator: "cuda", Name: "published-cuda"}, nil
			}
			gallery := func(_ context.Context, request modelswire.ResolvedHostConfiguration, offline bool) (modelswire.BackendArtifactSelection, error) {
				galleryCalls++
				if offline != test.offline || request.Platform.Accelerator != test.accelerator {
					t.Fatalf("gallery request = %#v, offline = %t", request, offline)
				}
				return modelswire.BackendArtifactSelection{Accelerator: "cuda", InstalledPath: "cuda12-gallery"}, nil
			}
			request := modelswire.ResolvedHostConfiguration{Platform: models.AssetHostPlatform{
				OperatingSystem: "linux", Architecture: "amd64", CUDAAvailable: test.cudaAvailable, Accelerator: test.accelerator,
			}}
			selection, err := preferPublishedLinuxCUDA(published, gallery)(context.Background(), request, test.offline)
			if err != nil {
				t.Fatal(err)
			}
			assertLinuxBackendSelection(t, selection, publishedCalls, galleryCalls, test.wantPublished, test.wantCalls)
		})
	}
}

func assertLinuxBackendSelection(t *testing.T, selection modelswire.BackendArtifactSelection, publishedCalls, galleryCalls int, wantPublished bool, wantCalls int) {
	t.Helper()
	if wantPublished {
		if selection.Name != "published-cuda" || publishedCalls != 1 || galleryCalls != 0 {
			t.Fatalf("selection = %#v, published calls = %d, gallery calls = %d", selection, publishedCalls, galleryCalls)
		}
	} else if selection.InstalledPath != "cuda12-gallery" || galleryCalls != 1 || publishedCalls != wantCalls {
		t.Fatalf("selection = %#v, published calls = %d, gallery calls = %d", selection, publishedCalls, galleryCalls)
	}
}

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

func TestModelsProcessLauncherRemovesInheritedLlamaCPPLauncherSwitch(t *testing.T) {
	t.Setenv("LLAMACPP_GRPC_SERVERS", "1")
	var childEnvironment []string
	launcher := modelsProcessLauncher{
		resolveLaunch: func(context.Context, serviceedges.HostProcessStartSpec) (managedbackend.ManagedBackendLaunch, error) {
			return managedbackend.ManagedBackendLaunch{
				Command: "llama-cpp-grpc", Endpoint: "grpc://127.0.0.1:1",
				Env: []string{"LD_LIBRARY_PATH=/managed/lib"}, Cleanup: func() error { return nil },
			}, nil
		},
		startProcess: func(_ context.Context, spec managedchild.Spec) (*managedchild.Process, error) {
			childEnvironment = spec.Env
			return nil, errors.New("controlled stop before launch")
		},
	}
	_, _ = launcher.Start(context.Background(), serviceedges.HostProcessStartSpec{Backend: "localai-llamacpp"})
	if len(childEnvironment) == 0 {
		t.Fatal("managed child environment was not captured")
	}
	for _, entry := range childEnvironment {
		key, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, "LLAMACPP_GRPC_SERVERS") {
			t.Fatalf("managed llama-cpp child inherited launcher switch: %q", entry)
		}
	}
}

func TestLinuxCUDAAvailableRequiresDeviceAndWorkingNvidiaProbe(t *testing.T) {
	t.Parallel()
	device := func(path string) (os.FileInfo, error) {
		if path == "/dev/dxg" {
			return nil, nil
		}
		return nil, fs.ErrNotExist
	}
	if !linuxCUDAAvailable(device, func(context.Context) ([]byte, error) {
		return []byte("GPU 0: NVIDIA GeForce RTX 4090"), nil
	}) {
		t.Fatal("WSL NVIDIA device and probe were not recognized")
	}
	if linuxCUDAAvailable(device, func(context.Context) ([]byte, error) {
		return nil, errors.New("nvidia-smi failed")
	}) {
		t.Fatal("failed NVIDIA probe was accepted")
	}
	if linuxCUDAAvailable(func(string) (os.FileInfo, error) { return nil, fs.ErrNotExist },
		func(context.Context) ([]byte, error) { t.Fatal("probe called without a GPU device"); return nil, nil }) {
		t.Fatal("missing NVIDIA device was accepted")
	}
}

func TestWindowsCUDAAvailableRequiresWorkingNvidiaProbe(t *testing.T) {
	t.Parallel()
	if !windowsCUDAAvailable(func(context.Context) ([]byte, error) {
		return []byte("GPU 0: NVIDIA GeForce RTX 4090"), nil
	}) {
		t.Fatal("Windows NVIDIA device was not recognized")
	}
	if windowsCUDAAvailable(func(context.Context) ([]byte, error) {
		return nil, errors.New("nvidia-smi failed")
	}) {
		t.Fatal("failed NVIDIA probe was accepted")
	}
	if windowsCUDAAvailable(func(context.Context) ([]byte, error) {
		return []byte("  \n"), nil
	}) {
		t.Fatal("empty NVIDIA device list was accepted")
	}
	if windowsCUDAAvailable(func(context.Context) ([]byte, error) {
		return []byte("No devices were found"), nil
	}) {
		t.Fatal("non-GPU output was accepted")
	}
}

func TestWindowsHostRecordsCUDAWithoutSelectingUnpublishedArchive(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("native Windows host probe")
	}
	expected := windowsCUDAAvailable(func(ctx context.Context) ([]byte, error) {
		return exec.CommandContext(ctx, "nvidia-smi", "-L").Output()
	})
	platform := provideModelAssetHostPlatform(serviceedges.Edges{})
	if platform.CUDAAvailable != expected || platform.Accelerator != "" {
		t.Fatalf("Windows host platform = %#v, want CUDA capability %t and no selected archive", platform, expected)
	}
}
