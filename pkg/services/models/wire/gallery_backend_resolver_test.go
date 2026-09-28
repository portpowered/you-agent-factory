package wire

import (
	"context"
	"errors"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

func TestGalleryBackendArtifactResolverSelectsCUDAWithoutCPUFallback(t *testing.T) {
	t.Parallel()
	tests := []struct {
		backend        string
		expectedCUDA   string
		expectedPath   string
	}{
		{"localai-llamacpp", "cuda12-llama-cpp", "/cache/backends/cuda12-llama-cpp"},
		{"localai-whisper", "cuda12-whisper", "/cache/backends/cuda12-whisper"},
		{"localai-vibevoice", "cuda12-vibevoice-cpp", "/cache/backends/cuda12-vibevoice-cpp"},
	}
	for _, tt := range tests {
		t.Run(tt.backend, func(t *testing.T) {
			t.Parallel()
			var installed string
			resolver, err := NewGalleryBackendArtifactResolver(func(_ context.Context, name string, offline bool) (string, error) {
				installed = name
				if offline {
					t.Fatal("online request marked offline")
				}
				return "/cache/backends/" + name, nil
			})
			if err != nil {
				t.Fatalf("construct gallery resolver: %v", err)
			}
			selection, err := resolver(context.Background(), modelseffects.ResolvedHostConfiguration{
				Backend: tt.backend, ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
				Platform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64", Accelerator: "cuda"},
			}, false)
			if err != nil {
				t.Fatalf("resolve CUDA backend %s: %v", tt.backend, err)
			}
			if installed != tt.expectedCUDA || selection.InstalledPath != tt.expectedPath {
				t.Fatalf("installed = %q, selection = %#v, want installed = %q, path = %q", installed, selection, tt.expectedCUDA, tt.expectedPath)
			}
		})
	}
}

func TestGalleryBackendArtifactResolverRejectsUnsupportedAccelerator(t *testing.T) {
	t.Parallel()
	called := false
	resolver, err := NewGalleryBackendArtifactResolver(func(context.Context, string, bool) (string, error) {
		called = true
		return "", nil
	})
	if err != nil {
		t.Fatalf("construct gallery resolver: %v", err)
	}
	_, err = resolver(context.Background(), modelseffects.ResolvedHostConfiguration{
		Backend: "localai-whisper", ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Platform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64", Accelerator: "metal"},
	}, false)
	if !errors.Is(err, artifacts.ErrIncompatibleAccelerator) || called {
		t.Fatalf("error = %v, installer called = %t", err, called)
	}
}

func TestGalleryBackendNameUsesExplicitCPUVariant(t *testing.T) {
	t.Parallel()
	name, err := galleryBackendName("localai-vibevoice", "cpu")
	if err != nil || name != "cpu-vibevoice-cpp" {
		t.Fatalf("gallery name = %q, error = %v, want cpu-vibevoice-cpp", name, err)
	}
}
