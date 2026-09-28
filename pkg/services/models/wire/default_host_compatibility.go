package wire

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backendregistry"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

// NewDefaultHostCompatibilityChecker constructs the production host policy.
// Windows capability checks leave archive availability to the published resolver;
// other platforms retain selection against the pinned artifact matrix.
func NewDefaultHostCompatibilityChecker() (HostCompatibilityChecker, error) {
	resolver, err := NewDefaultBackendArtifactResolver()
	if err != nil {
		return nil, fmt.Errorf("construct default host compatibility selector: %w", err)
	}
	return defaultHostCompatibilityChecker{resolve: resolver}, nil
}

// NewGalleryHostCompatibilityChecker validates a Linux gallery backend choice
// without starting a download during capability checks.
func NewGalleryHostCompatibilityChecker() HostCompatibilityChecker {
	return galleryHostCompatibilityChecker{}
}

type galleryHostCompatibilityChecker struct{}

func (galleryHostCompatibilityChecker) Check(ctx context.Context, request HostCompatibilityRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	configuration := request.Configuration
	if configuration.Platform.OperatingSystem != "linux" || configuration.Platform.Architecture != "amd64" {
		return fmt.Errorf("LocalAI gallery backend requires linux/amd64")
	}
	if configuration.ProtocolVersion != "" && configuration.ProtocolVersion != modelseffects.PinnedHostProtocolVersion {
		return fmt.Errorf("LocalAI backend protocol %q is unsupported", configuration.ProtocolVersion)
	}
	_, err := galleryBackendName(configuration.Backend, defaultBackendAccelerator(configuration.Platform))
	return err
}

type defaultHostCompatibilityChecker struct {
	resolve BackendArtifactResolver
}

func (checker defaultHostCompatibilityChecker) Check(
	ctx context.Context,
	request HostCompatibilityRequest,
) error {
	configuration := request.Configuration.Clone()
	if configuration.ProtocolVersion == "" {
		configuration.ProtocolVersion = modelseffects.PinnedHostProtocolVersion
	}
	if configuration.Platform.OperatingSystem == "windows" && configuration.Platform.Architecture == "amd64" {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, supported := backendregistry.LookupArtifact(configuration.Backend); !supported {
			return fmt.Errorf("%w: backend %q", artifacts.ErrUnknownBackend, configuration.Backend)
		}
		if configuration.ProtocolVersion != modelseffects.PinnedHostProtocolVersion {
			return fmt.Errorf("%w: requested protocol %q", artifacts.ErrIncompatibleProtocol, configuration.ProtocolVersion)
		}
		accelerator := defaultBackendAccelerator(configuration.Platform)
		if accelerator != "cpu" && accelerator != "cuda" {
			return fmt.Errorf("%w: accelerator %q", artifacts.ErrIncompatibleAccelerator, accelerator)
		}
		return nil
	}
	_, err := checker.resolve(ctx, configuration, false)
	if err != nil {
		return fmt.Errorf(
			"select pinned backend %q for model %q: %w",
			configuration.Backend, configuration.ModelName, err,
		)
	}
	return nil
}

var _ HostCompatibilityChecker = defaultHostCompatibilityChecker{}
