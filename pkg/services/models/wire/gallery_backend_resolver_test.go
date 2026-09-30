package wire

import (
	"context"
	"errors"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

func TestGalleryBackendArtifactResolverSelectsCPUOnWindows(t *testing.T) {
	t.Parallel()
	var installed string
	resolver, err := NewGalleryBackendArtifactResolver(func(_ context.Context, name string, offline bool) (string, error) {
		installed = name
		return "/cache/backends/" + name, nil
	})
	if err != nil {
		t.Fatalf("construct gallery resolver: %v", err)
	}
	selection, err := resolver(context.Background(), modelseffects.ResolvedHostConfiguration{
		Backend: "localai-llamacpp", ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Platform: models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64", Accelerator: "cpu"},
	}, false)
	if err != nil {
		t.Fatalf("resolve Windows CPU backend: %v", err)
	}
	if installed != "cpu-llama-cpp" {
		t.Fatalf("installed = %q, want cpu-llama-cpp", installed)
	}
	if selection.Accelerator != "cpu" {
		t.Fatalf("accelerator = %q, want cpu", selection.Accelerator)
	}
}

func TestGalleryBackendArtifactResolverRejectsCUDAOnWindows(t *testing.T) {
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
		Backend: "localai-llamacpp", ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Platform: models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64", Accelerator: "cuda"},
	}, false)
	if !errors.Is(err, artifacts.ErrUnsupportedPlatform) || called {
		t.Fatalf("error = %v, installer called = %t, want ErrUnsupportedPlatform", err, called)
	}
}

func TestGalleryBackendArtifactResolverRejectsUnsupportedPlatform(t *testing.T) {
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
		Backend: "localai-llamacpp", ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Platform: models.AssetHostPlatform{OperatingSystem: "darwin", Architecture: "arm64", Accelerator: "metal"},
	}, false)
	if !errors.Is(err, artifacts.ErrUnsupportedPlatform) || called {
		t.Fatalf("error = %v, installer called = %t, want ErrUnsupportedPlatform", err, called)
	}
}

func TestGalleryBackendArtifactResolverRejectsIncompatibleProtocol(t *testing.T) {
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
		Backend: "localai-llamacpp", ProtocolVersion: "localai-backend-v0",
		Platform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64", Accelerator: "cpu"},
	}, false)
	if !errors.Is(err, artifacts.ErrIncompatibleProtocol) || called {
		t.Fatalf("error = %v, installer called = %t, want ErrIncompatibleProtocol", err, called)
	}
}

func TestGalleryBackendArtifactResolverRejectsUnknownBackend(t *testing.T) {
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
		Backend: "localai-unknown", ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Platform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64", Accelerator: "cpu"},
	}, false)
	if !errors.Is(err, artifacts.ErrUnknownBackend) || called {
		t.Fatalf("error = %v, installer called = %t, want ErrUnknownBackend", err, called)
	}
}

func TestGalleryBackendNameUsesCUDAVariant(t *testing.T) {
	t.Parallel()
	name, err := galleryBackendName("localai-llamacpp", "cuda")
	if err != nil || name != "cuda12-llama-cpp" {
		t.Fatalf("gallery name = %q, error = %v, want cuda12-llama-cpp", name, err)
	}
}

func TestValidateGalleryPlatform(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		platform    models.AssetHostPlatform
		accelerator string
		wantErr     error
	}{
		{
			name:        "linux amd64 cpu",
			platform:    models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
			accelerator: "cpu",
		},
		{
			name:        "linux amd64 cuda",
			platform:    models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
			accelerator: "cuda",
		},
		{
			name:        "windows amd64 cpu",
			platform:    models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64"},
			accelerator: "cpu",
		},
		{
			name:        "windows amd64 cuda rejected",
			platform:    models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64"},
			accelerator: "cuda",
			wantErr:     artifacts.ErrUnsupportedPlatform,
		},
		{
			name:        "linux arm64 rejected",
			platform:    models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "arm64"},
			accelerator: "cpu",
			wantErr:     artifacts.ErrUnsupportedPlatform,
		},
		{
			name:        "windows arm64 rejected",
			platform:    models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "arm64"},
			accelerator: "cpu",
			wantErr:     artifacts.ErrUnsupportedPlatform,
		},
		{
			name:        "darwin rejected",
			platform:    models.AssetHostPlatform{OperatingSystem: "darwin", Architecture: "arm64"},
			accelerator: "metal",
			wantErr:     artifacts.ErrUnsupportedPlatform,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateGalleryPlatform(test.platform, test.accelerator)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("error = %v, want %v", err, test.wantErr)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
