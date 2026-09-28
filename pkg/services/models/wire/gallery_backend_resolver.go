package wire

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

// GalleryBackendInstaller makes one named LocalAI gallery backend available
// and returns its installed directory. LocalAI owns OCI resolution and
// extraction; Models receives only the directory needed to launch it.
type GalleryBackendInstaller func(context.Context, string, bool) (string, error)

// NewGalleryBackendArtifactResolver selects LocalAI's current gallery build
// for Linux. The installer is an effect supplied by application composition.
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
		if request.Platform.OperatingSystem != "linux" || request.Platform.Architecture != "amd64" {
			return BackendArtifactSelection{}, fmt.Errorf("%w: LocalAI gallery backend requires linux/amd64", artifacts.ErrUnsupportedPlatform)
		}
		accelerator := defaultBackendAccelerator(request.Platform)
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
