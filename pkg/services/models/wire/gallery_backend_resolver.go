package wire

import (
	"context"
	"fmt"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

// GalleryBackendInstaller makes one named LocalAI gallery backend available
// and returns its installed directory. LocalAI owns OCI resolution and
// extraction; Models receives only the directory needed to launch it.
type GalleryBackendInstaller func(context.Context, string, bool) (string, error)

// NewGalleryBackendArtifactResolver selects LocalAI's current gallery build
// for Linux and Windows. The installer is an effect supplied by application composition.
//
// CUDA accelerators are only supported on Linux because upstream LocalAI detects
// CUDA toolkit installations via Linux-specific paths (/usr/local/cuda-12,
// /usr/local/cuda-13) and publishes no Windows CUDA backend gallery entries.
// CPU accelerators are supported on both linux/amd64 and windows/amd64.
func NewGalleryBackendArtifactResolver(install GalleryBackendInstaller) (BackendArtifactResolver, error) {
	if install == nil {
		return nil, fmt.Errorf("LocalAI gallery installer is required")
	}
	return func(ctx context.Context, request ResolvedHostConfiguration, offline bool) (BackendArtifactSelection, error) {
		if err := ctx.Err(); err != nil {
			return BackendArtifactSelection{}, err
		}
		if request.ProtocolVersion != modelseffects.PinnedHostProtocolVersion {
			return BackendArtifactSelection{}, fmt.Errorf("%w: requested protocol %q", artifacts.ErrIncompatibleProtocol, request.ProtocolVersion)
		}
		accelerator := defaultBackendAccelerator(request.Platform)
		if err := validateGalleryPlatform(request.Platform, accelerator); err != nil {
			return BackendArtifactSelection{}, err
		}
		name, err := galleryBackendName(request.Backend, accelerator)
		if err != nil {
			return BackendArtifactSelection{}, err
		}
		path, err := install(ctx, name, offline)
		if err != nil {
			return BackendArtifactSelection{}, fmt.Errorf("install LocalAI gallery backend %q: %w", name, err)
		}
		return BackendArtifactSelection{InstalledPath: path, Accelerator: accelerator}, nil
	}, nil
}

// validateGalleryPlatform enforces the platform and accelerator constraints of
// the LocalAI gallery backend. CUDA is Linux-only; CPU is supported on
// linux/amd64 and windows/amd64.
func validateGalleryPlatform(platform models.AssetHostPlatform, accelerator string) error {
	switch platform.OperatingSystem {
	case "linux":
		if platform.Architecture != "amd64" {
			return fmt.Errorf("%w: LocalAI gallery backend requires amd64 on linux", artifacts.ErrUnsupportedPlatform)
		}
	case "windows":
		if platform.Architecture != "amd64" {
			return fmt.Errorf("%w: LocalAI gallery backend requires amd64 on windows", artifacts.ErrUnsupportedPlatform)
		}
		if accelerator == "cuda" {
			return fmt.Errorf("%w: LocalAI gallery CUDA backend is not available on windows", artifacts.ErrUnsupportedPlatform)
		}
	default:
		return fmt.Errorf("%w: LocalAI gallery backend requires linux or windows", artifacts.ErrUnsupportedPlatform)
	}
	return nil
}

func galleryBackendName(backend, accelerator string) (string, error) {
	base := ""
	switch backend {
	case "localai-llamacpp":
		base = "llama-cpp"
	case "localai-whisper":
		base = "whisper"
	case "localai-vibevoice":
		base = "vibevoice-cpp"
	case "localai-qwen3-tts-cpp":
		base = "qwen3-tts-cpp"
	case "localai-qwen3-asr-cpp":
		base = "qwen3-asr-cpp"
	case "localai-audio-cpp":
		base = "audio-cpp"
	default:
		return "", fmt.Errorf("%w: backend %q", artifacts.ErrUnknownBackend, backend)
	}
	switch accelerator {
	case "cpu":
		return "cpu-" + base, nil
	case "cuda":
		return "cuda12-" + base, nil
	default:
		return "", fmt.Errorf("%w: accelerator %q", artifacts.ErrIncompatibleAccelerator, accelerator)
	}
}
