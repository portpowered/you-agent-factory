package wire

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

func TestBackendArtifactResolverPrefersAvailableWindowsCUDAArchive(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "internal", "artifacts", "testdata", "windows-cuda-variant-manifest.json"))
	if err != nil {
		t.Fatalf("read CUDA manifest fixture: %v", err)
	}
	manifest, err := artifacts.Decode(data)
	if err != nil {
		t.Fatalf("decode CUDA manifest fixture: %v", err)
	}
	resolve := backendArtifactResolver(manifest)
	platform := models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64", CUDAAvailable: true}
	for _, testCase := range []struct {
		backend, accelerator, target string
	}{
		{"localai-llamacpp", "cuda", "windows-amd64-cuda"},
		{"localai-whisper", "cpu", "windows-amd64"},
		{"localai-vibevoice", "cpu", "windows-amd64"},
	} {
		selection, err := resolve(context.Background(), ResolvedHostConfiguration{
			Backend: testCase.backend, Platform: platform,
			ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		}, false)
		if err != nil {
			t.Fatalf("resolve %s: %v", testCase.backend, err)
		}
		if selection.Accelerator != testCase.accelerator || selection.Name == "" ||
			!strings.Contains(selection.Name, testCase.target) {
			t.Fatalf("selection for %s = %#v, want %s", testCase.backend, selection, testCase.target)
		}
	}
}

func TestDefaultBackendArtifactResolverFallsBackToPublishedWindowsCPU(t *testing.T) {
	t.Parallel()
	resolve, err := NewDefaultBackendArtifactResolver()
	if err != nil {
		t.Fatalf("construct default resolver: %v", err)
	}
	selection, err := resolve(context.Background(), ResolvedHostConfiguration{
		Backend: "localai-llamacpp",
		Platform: models.AssetHostPlatform{
			OperatingSystem: "windows", Architecture: "amd64", CUDAAvailable: true,
		},
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
	}, false)
	if err != nil {
		t.Fatalf("resolve Windows backend: %v", err)
	}
	if selection.Accelerator != "cpu" || strings.Contains(selection.Name, "windows-amd64-cuda") {
		t.Fatalf("published archive selection = %#v, want CPU fallback", selection)
	}
}

func TestNewDefaultBackendArtifactResolverSelectsPinnedMatrix(t *testing.T) {
	t.Parallel()

	resolver, err := NewDefaultBackendArtifactResolver()
	if err != nil {
		t.Fatalf("NewDefaultBackendArtifactResolver: %v", err)
	}
	for _, test := range []struct {
		name     string
		backend  string
		platform models.AssetHostPlatform
	}{
		{name: "llamacpp linux", backend: "localai-llamacpp", platform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"}},
		{name: "llamacpp mac", backend: "localai-llamacpp", platform: models.AssetHostPlatform{OperatingSystem: "darwin", Architecture: "arm64"}},
		{name: "llamacpp windows", backend: "localai-llamacpp", platform: models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64"}},
		{name: "whisper linux", backend: "localai-whisper", platform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"}},
		{name: "vibevoice linux", backend: "localai-vibevoice", platform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			selection, err := resolver(context.Background(), ResolvedHostConfiguration{
				Backend: test.backend, Platform: test.platform,
				ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
			}, false)
			if err != nil {
				t.Fatalf("resolve %s: %v", test.backend, err)
			}
			if selection.Name == "" || selection.Location == "" || selection.Bytes <= 0 || len(selection.SHA256) != 64 {
				t.Fatalf("selection = %#v, want detached pinned archive facts", selection)
			}
		})
	}
}

func TestNewDefaultBackendArtifactResolverPreservesAcceleratorRequest(t *testing.T) {
	t.Parallel()

	resolver, err := NewDefaultBackendArtifactResolver()
	if err != nil {
		t.Fatalf("NewDefaultBackendArtifactResolver: %v", err)
	}
	for _, operatingSystem := range []string{"windows", "linux"} {
		t.Run(operatingSystem, func(t *testing.T) {
			platform := models.AssetHostPlatform{
				OperatingSystem: operatingSystem, Architecture: "amd64", Accelerator: "cuda",
			}
			request := ResolvedHostConfiguration{
				Backend: "localai-llamacpp", Platform: platform,
				ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
			}
			_, err := resolver(context.Background(), request, false)
			if !errors.Is(err, artifacts.ErrIncompatibleAccelerator) {
				t.Fatalf("resolve explicit CUDA error = %v, want ErrIncompatibleAccelerator", err)
			}
			var failure *artifacts.Failure
			if !errors.As(err, &failure) || failure.Detail != "the matching artifact does not declare this accelerator" {
				t.Fatalf("resolve explicit CUDA error = %v, want selection against the CPU-only manifest", err)
			}

			request.Platform.Accelerator = ""
			selection, err := resolver(context.Background(), request, false)
			if err != nil {
				t.Fatalf("resolve omitted accelerator: %v", err)
			}
			request.Platform.Accelerator = "cpu"
			cpuSelection, err := resolver(context.Background(), request, false)
			if err != nil {
				t.Fatalf("resolve explicit CPU: %v", err)
			}
			if selection != cpuSelection || selection.Name == "" {
				t.Fatalf("omitted accelerator selection = %#v, want explicit CPU selection %#v", selection, cpuSelection)
			}
		})
	}
}

func TestNewDefaultBackendArtifactResolverRejectsIncompatibleRequests(t *testing.T) {
	t.Parallel()

	resolver, err := NewDefaultBackendArtifactResolver()
	if err != nil {
		t.Fatalf("NewDefaultBackendArtifactResolver: %v", err)
	}
	base := ResolvedHostConfiguration{
		Backend: "localai-vibevoice", Platform: models.AssetHostPlatform{
			OperatingSystem: "linux", Architecture: "amd64",
		}, ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
	}
	tests := []struct {
		name string
		edit func(*ResolvedHostConfiguration)
		want error
	}{
		{name: "protocol", edit: func(request *ResolvedHostConfiguration) { request.ProtocolVersion = "localai-backend-v0" }, want: artifacts.ErrIncompatibleProtocol},
		{name: "platform", edit: func(request *ResolvedHostConfiguration) { request.Platform.OperatingSystem = "freebsd" }, want: artifacts.ErrUnsupportedPlatform},
		{name: "backend", edit: func(request *ResolvedHostConfiguration) { request.Backend = "localai-unknown" }, want: artifacts.ErrUnknownBackend},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.edit(&request)
			_, err := resolver(context.Background(), request, false)
			if !errors.Is(err, test.want) {
				t.Fatalf("resolve error = %v, want %v", err, test.want)
			}
		})
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolver(cancelled, base, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled resolve error = %v, want context.Canceled", err)
	}
}

func TestNewDefaultHostCompatibilityCheckerUsesPinnedArtifactMatrix(t *testing.T) {
	t.Parallel()

	checker, err := NewDefaultHostCompatibilityChecker()
	if err != nil {
		t.Fatalf("NewDefaultHostCompatibilityChecker: %v", err)
	}
	if err := checker.Check(context.Background(), HostCompatibilityRequest{
		Configuration: ResolvedHostConfiguration{
			Backend:   "localai-llamacpp",
			ModelName: "llm",
			Platform:  models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		},
	}); err != nil {
		t.Fatalf("supported pinned host: %v", err)
	}
	if err := checker.Check(context.Background(), HostCompatibilityRequest{
		Configuration: ResolvedHostConfiguration{
			Backend:   "localai-llamacpp",
			ModelName: "llm",
			Platform:  models.AssetHostPlatform{OperatingSystem: "freebsd", Architecture: "amd64"},
		},
	}); err == nil {
		t.Fatal("unsupported pinned host unexpectedly passed compatibility")
	}
}
