package wire

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	managedchild "github.com/portpowered/infinite-you/pkg/platform/process/managedchild"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	"github.com/portpowered/infinite-you/pkg/wire/internal/managedbackend"
)

func TestModelsCompositionSelectsDisabledRevisionEffect(t *testing.T) {
	t.Parallel()
	resolve := provideModelAssetRevision(serviceedges.Edges{})
	if resolve == nil {
		t.Fatal("revision selection must supply an explicit disabled effect")
	}
	result, err := resolve(t.Context(), "hf://selected/repository@main")
	if result != "" || !errors.Is(err, models.ErrModelRevisionUnresolved) {
		t.Fatalf("disabled revision = %q/%v, want unresolved", result, err)
	}
}

func TestModelsCompositionSelectsRevisionEffectInertly(t *testing.T) {
	t.Parallel()
	for _, wantErr := range []error{nil, errors.New("selected revision failure")} {
		t.Run(fmt.Sprint(wantErr), func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			const source = "hf://selected/repository@main"
			calls := 0
			edges := serviceedges.Edges{
				ModelResolveHuggingFaceRevision: func(got context.Context, input string) (string, error) {
					calls++
					if got != ctx || input != source {
						t.Fatalf("revision request = %v/%q, want selected context/source", got, input)
					}
					return "selected-revision", wantErr
				},
			}
			resolve := provideModelAssetRevision(edges)
			if resolve == nil || calls != 0 {
				t.Fatal("revision selection must supply an inert effect")
			}
			result, err := resolve(ctx, source)
			if result != "selected-revision" || err != wantErr || calls != 1 {
				t.Fatalf("revision = %q/%v, calls=%d, want selected result/error once", result, err, calls)
			}
		})
	}
}

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

func TestWindowsHostSelectsPublishedCUDAAccelerator(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("native Windows host probe")
	}
	expected := windowsCUDAAvailable(func(ctx context.Context) ([]byte, error) {
		return exec.CommandContext(ctx, "nvidia-smi", "-L").Output()
	})
	// Keep the accelerator automatic so backend resolution can fall back to CPU
	// when no compatible Windows CUDA archive has been published.
	platform := provideModelAssetHostPlatform(serviceedges.Edges{})
	if platform.CUDAAvailable != expected || platform.Accelerator != "" {
		t.Fatalf("Windows host platform = %#v, want CUDA capability %t and automatic accelerator", platform, expected)
	}
}

func TestWindowsCUDAAvailableProbeDetectsDevice(t *testing.T) {
	t.Parallel()
	output := func(context.Context) ([]byte, error) {
		return []byte("GPU 0: NVIDIA GeForce RTX 4090"), nil
	}
	if !windowsCUDAAvailable(output) {
		t.Fatal("Windows nvidia-smi probe with device was not recognized")
	}
}

func TestWindowsCUDAAvailableProbeRejectsFailedProbe(t *testing.T) {
	t.Parallel()
	output := func(context.Context) ([]byte, error) {
		return nil, errors.New("nvidia-smi failed")
	}
	if windowsCUDAAvailable(output) {
		t.Fatal("failed Windows NVIDIA probe was accepted")
	}
}

func TestWindowsCUDAAvailableProbeRejectsNoDevice(t *testing.T) {
	t.Parallel()
	output := func(context.Context) ([]byte, error) {
		return nil, errors.New("no NVIDIA devices")
	}
	if windowsCUDAAvailable(output) {
		t.Fatal("Windows nvidia-smi probe without device was accepted")
	}
}

func TestModelAssetHostPlatformWindowsAcceleratorDetection(t *testing.T) {
	t.Parallel()
	platform := models.AssetHostPlatform{
		OperatingSystem: "windows",
		Architecture:    "amd64",
	}
	result := provideModelAssetHostPlatform(serviceedges.Edges{ModelAssetHostPlatform: platform})
	if result.Accelerator != "" {
		t.Fatalf("expected automatic Windows accelerator selection, got %q", result.Accelerator)
	}
}

func TestModelAssetHostPlatformWindowsNoAcceleratorWhenNoProbe(t *testing.T) {
	t.Parallel()
	// When windowsCUDAAvailable probe returns an error, Accelerator should remain empty
	if windowsCUDAAvailable(func(context.Context) ([]byte, error) {
		return nil, errors.New("nvidia-smi failed")
	}) {
		t.Fatal("windowsCUDAAvailable with failed probe was accepted")
	}
}

func TestModelAssetHostPlatformWindowsArm64NoAccelerator(t *testing.T) {
	t.Parallel()
	platform := models.AssetHostPlatform{
		OperatingSystem: "windows",
		Architecture:    "arm64",
	}
	result := provideModelAssetHostPlatform(serviceedges.Edges{ModelAssetHostPlatform: platform})
	if result.Accelerator != "" {
		t.Fatalf("expected empty Accelerator on arm64, got %q", result.Accelerator)
	}
}

func TestModelAssetHostPlatformLinuxNoAcceleratorWhenNoDevice(t *testing.T) {
	t.Parallel()
	platform := models.AssetHostPlatform{
		OperatingSystem: "linux",
		Architecture:    "amd64",
	}
	result := provideModelAssetHostPlatform(serviceedges.Edges{ModelAssetHostPlatform: platform})
	if result.Accelerator != "" {
		t.Fatalf("expected empty Accelerator on Linux without device, got %q", result.Accelerator)
	}
}
