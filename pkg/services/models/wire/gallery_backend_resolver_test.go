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
		Backend: "localai-llamacpp", ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Platform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64", Accelerator: "cuda"},
	}, false)
	if err != nil {
		t.Fatalf("resolve CUDA backend: %v", err)
	}
	if installed != "cuda12-llama-cpp" || selection.InstalledPath != "/cache/backends/cuda12-llama-cpp" {
		t.Fatalf("installed = %q, selection = %#v", installed, selection)
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
	if err != nil || name != "cpu-vibevoice" {
		t.Fatalf("gallery name = %q, error = %v, want cpu-vibevoice", name, err)
	}
}
